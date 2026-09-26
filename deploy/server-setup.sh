#!/usr/bin/env bash
# Yangi prod serverni (Contabo VPS yoki boshqa) (Ubuntu 24.04/26.04 LTS) noldan ishga tayyorlaydi.
#
# ISHLATISH — serverga root bo'lib kirib:
#   scp deploy/server-setup.sh root@<SERVER_IP>:~/
#   ssh root@<SERVER_IP> 'bash server-setup.sh'
#
# Repo OCHIQ (public) bo'lsa to'g'ridan-to'g'ri ham mumkin:
#   curl -fsSL https://raw.githubusercontent.com/IshchiBormi/web-application/main/deploy/server-setup.sh | bash
#
# Skript IDEMPOTENT: qayta ishga tushirilsa hech narsani buzmaydi, faqat
# yetishmagan qismini to'ldiradi. `.env` ga TEGMAYDI (mavjud sirlar saqlanadi).
set -euo pipefail

DEPLOY_USER="${DEPLOY_USER:-deploy}"
PROJECT_DIR="${PROJECT_DIR:-/opt/ishchibormi}"
REPO_URL="${REPO_URL:-https://github.com/IshchiBormi/web-application.git}"
DOMAIN="${DOMAIN:-ishchibormi.uz}"

log() { printf '\n\033[1;36m==> %s\033[0m\n' "$*"; }

if [ "$(id -u)" -ne 0 ]; then
	echo "XATO: root bo'lib ishga tushiring (sudo bash $0)" >&2
	exit 1
fi

# ── 1. Tizim paketlari ──────────────────────────────────────────────────────
log "Tizim paketlarini yangilash"
export DEBIAN_FRONTEND=noninteractive
apt-get update -qq
apt-get upgrade -y -qq
apt-get install -y -qq ca-certificates curl gnupg git ufw fail2ban apt-transport-https

# ── 2. Swap ─────────────────────────────────────────────────────────────────
# Next.js `npm run build` xotira talab qiladi. Swap bo'lmasa build OOM-killer
# tomonidan "Killed" qilinadi va deploy yarim yo'lda to'xtaydi — aynan shu
# muammo eski serverda ham bo'lgan (deploy/README.md dagi izohga qarang).
if ! swapon --show | grep -q .; then
	log "2GB swap yaratish (Next.js build OOM bo'lmasligi uchun)"
	fallocate -l 2G /swapfile
	chmod 600 /swapfile
	mkswap /swapfile >/dev/null
	swapon /swapfile
	grep -q '^/swapfile' /etc/fstab || echo '/swapfile none swap sw 0 0' >>/etc/fstab
	# Swap faqat zaxira bo'lsin: RAM to'lmaguncha diskka yozilmasin.
	sysctl -qw vm.swappiness=10
	grep -q '^vm.swappiness' /etc/sysctl.conf || echo 'vm.swappiness=10' >>/etc/sysctl.conf
else
	log "Swap allaqachon bor — o'tkazib yuborildi"
fi

# ── 3. Docker Engine + Compose plugin ───────────────────────────────────────
if ! command -v docker >/dev/null 2>&1; then
	log "Docker o'rnatish (rasmiy repodan)"
	install -m 0755 -d /etc/apt/keyrings
	curl -fsSL https://download.docker.com/linux/ubuntu/gpg -o /etc/apt/keyrings/docker.asc
	chmod a+r /etc/apt/keyrings/docker.asc
	CODENAME="$(. /etc/os-release && echo "$VERSION_CODENAME")"
	ARCH="$(dpkg --print-architecture)"
	echo "deb [arch=$ARCH signed-by=/etc/apt/keyrings/docker.asc] https://download.docker.com/linux/ubuntu $CODENAME stable" \
		>/etc/apt/sources.list.d/docker.list
	apt-get update -qq
	apt-get install -y -qq docker-ce docker-ce-cli containerd.io docker-buildx-plugin docker-compose-plugin
	systemctl enable --now docker
else
	log "Docker allaqachon bor: $(docker --version)"
fi

# Docker loglari diskni to'ldirib qo'ymasin (server diski kichik).
if [ ! -f /etc/docker/daemon.json ]; then
	log "Docker log rotatsiyasini yoqish"
	mkdir -p /etc/docker
	printf '%s\n' \
		'{' \
		'  "log-driver": "json-file",' \
		'  "log-opts": { "max-size": "10m", "max-file": "3" }' \
		'}' >/etc/docker/daemon.json
	systemctl restart docker
fi

# ── 4. Deploy foydalanuvchisi ───────────────────────────────────────────────
# Hamma narsa root ostida ishlamasin: CI aynan shu foydalanuvchi bilan SSH qiladi.
if ! id -u "$DEPLOY_USER" >/dev/null 2>&1; then
	log "'$DEPLOY_USER' foydalanuvchisini yaratish"
	adduser --disabled-password --gecos "" "$DEPLOY_USER"
fi
usermod -aG docker "$DEPLOY_USER"

# root'ning SSH kalitlarini deploy user'ga ham beramiz — aks holda skriptdan
# keyin serverga kira olmay qolishingiz mumkin.
if [ -f /root/.ssh/authorized_keys ]; then
	log "SSH kalitlarini '$DEPLOY_USER' ga ko'chirish"
	install -d -m 700 -o "$DEPLOY_USER" -g "$DEPLOY_USER" "/home/$DEPLOY_USER/.ssh"
	touch "/home/$DEPLOY_USER/.ssh/authorized_keys"
	# Takrorlanmasin: mavjud kalitlar bilan birlashtirib, unikal qilamiz.
	sort -u /root/.ssh/authorized_keys "/home/$DEPLOY_USER/.ssh/authorized_keys" \
		| grep -v '^[[:space:]]*$' >"/tmp/authorized_keys.$$"
	mv "/tmp/authorized_keys.$$" "/home/$DEPLOY_USER/.ssh/authorized_keys"
	chmod 600 "/home/$DEPLOY_USER/.ssh/authorized_keys"
	chown "$DEPLOY_USER:$DEPLOY_USER" "/home/$DEPLOY_USER/.ssh/authorized_keys"
fi

# ── 5. Firewall ─────────────────────────────────────────────────────────────
# MUHIM: Mongo (27017/27018) va API (8080) ATAYLAB ochilmaydi — ular faqat
# 127.0.0.1 ga bog'langan va tashqaridan faqat Caddy orqali kiriladi.
log "Firewall (ufw): 22, 80, 443"
ufw allow OpenSSH >/dev/null
ufw allow 80/tcp >/dev/null
ufw allow 443/tcp >/dev/null
ufw --force enable >/dev/null
systemctl enable --now fail2ban >/dev/null 2>&1 || true

# ── 6. Caddy (TLS + reverse proxy) ──────────────────────────────────────────
if ! command -v caddy >/dev/null 2>&1; then
	log "Caddy o'rnatish"
	curl -1sLf 'https://dl.cloudsmith.io/public/caddy/stable/gpg.key' \
		| gpg --dearmor -o /usr/share/keyrings/caddy-stable-archive-keyring.gpg
	curl -1sLf 'https://dl.cloudsmith.io/public/caddy/stable/debian.deb.txt' \
		>/etc/apt/sources.list.d/caddy-stable.list
	apt-get update -qq
	apt-get install -y -qq caddy
else
	log "Caddy allaqachon bor: $(caddy version)"
fi

# ── 7. Repo ─────────────────────────────────────────────────────────────────
if [ ! -d "$PROJECT_DIR/.git" ]; then
	log "Reponi $PROJECT_DIR ga klonlash"
	mkdir -p "$(dirname "$PROJECT_DIR")"
	git clone "$REPO_URL" "$PROJECT_DIR"
else
	log "Repo allaqachon bor: $PROJECT_DIR"
fi
chown -R "$DEPLOY_USER:$DEPLOY_USER" "$PROJECT_DIR"
# CI serverda `git reset --hard` qiladi; "dubious ownership" xatosi chiqmasin.
sudo -u "$DEPLOY_USER" git config --global --add safe.directory "$PROJECT_DIR"

# FCM service-account fayli uchun papka (bo'sh bo'lsa mobil push jimgina o'chiq).
#
# Guruh ATAYLAB 10001 — backend konteyneri appuser (uid/gid 10001,
# apps/api/Dockerfile) sifatida ishlaydi, faylni esa CI `deploy` (1001)
# nomidan yozadi. Setgid biti (2750) bo'lmasa yangi fayllar `deploy` guruhida
# yaratiladi va konteyner ularni ocha olmaydi ("permission denied" -> push
# jimgina o'chadi). `deploy` o'zi a'zo bo'lmagan guruhga chgrp qila olmaydi,
# shuning uchun buni root shu yerda, bir marta o'rnatib qo'yadi.
# Dockerfile'dagi UID o'zgarsa, bu raqam ham o'zgarishi shart.
install -d -m 2750 -o "$DEPLOY_USER" -g 10001 "$PROJECT_DIR/secrets"

# ── 8. Caddyfile ────────────────────────────────────────────────────────────
if [ -f "$PROJECT_DIR/deploy/Caddyfile" ]; then
	log "Caddyfile o'rnatish"
	cp "$PROJECT_DIR/deploy/Caddyfile" /etc/caddy/Caddyfile
	mkdir -p /var/log/caddy && chown caddy:caddy /var/log/caddy
	if caddy validate --config /etc/caddy/Caddyfile >/dev/null 2>&1; then
		systemctl reload caddy || systemctl restart caddy
	else
		echo "OGOHLANTIRISH: Caddyfile validatsiyadan o'tmadi — qo'lda tekshiring." >&2
	fi
fi

# ── 9. Mongo zaxira nusxasi (kunlik cron) ───────────────────────────────────
if [ -f "$PROJECT_DIR/deploy/backup-mongo.sh" ]; then
	log "Kunlik Mongo backup cron'i (har kuni 03:30)"
	chmod +x "$PROJECT_DIR/deploy/backup-mongo.sh"
	{
		echo "SHELL=/bin/bash"
		echo "PATH=/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin"
		echo "30 3 * * * $DEPLOY_USER PROJECT_DIR=$PROJECT_DIR $PROJECT_DIR/deploy/backup-mongo.sh >>/var/log/ishchibormi-backup.log 2>&1"
	} >/etc/cron.d/ishchibormi-backup
	chmod 644 /etc/cron.d/ishchibormi-backup
fi

IP="$(curl -s4 --max-time 5 ifconfig.me 2>/dev/null || echo '<SERVER_IP>')"

cat <<INFO

================================================================
 Server tayyor. Qolgan qadamlar (QO'LDA bajariladi):

 1) DNS — $DOMAIN va www.$DOMAIN uchun A-yozuv: $IP
    Tarqalishini kuting: dig +short $DOMAIN
    (DNS tayyor bo'lmasa Caddy TLS sertifikat OLA OLMAYDI.)

 2) .env yaratish:
      sudo -u $DEPLOY_USER -i
      cd $PROJECT_DIR && cp .env.example .env && nano .env

    Majburiy o'zgartiriladigan kalitlar:
      APP_ENV=production
      JWT_ACCESS_SECRET   <- openssl rand -hex 32
      JWT_REFRESH_SECRET  <- openssl rand -hex 32  (ACCESS'dan FARQLI!)
      BOT_SHARED_SECRET   <- openssl rand -hex 32
      ADMIN_SEED_PASS     <- kamida 12 belgi
      TELEGRAM_BOT_TOKEN  <- @BotFather
      CORS_ORIGINS=https://$DOMAIN
      UPLOAD_PUBLIC_BASE=https://$DOMAIN/uploads
      NEXT_PUBLIC_API_BASE=https://$DOMAIN

 3) Birinchi ishga tushirish:
      cd $PROJECT_DIR && docker compose up -d --build
      docker compose ps
      curl -s localhost:8080/healthz

 4) GitHub secrets (Settings -> Secrets and variables -> Actions):
      DEPLOY_HOST=$IP
      DEPLOY_USER=$DEPLOY_USER
      DEPLOY_SSH_KEY=<private kalit, to'liq matn>
      PROJECT_DIR=$PROJECT_DIR
================================================================
INFO
