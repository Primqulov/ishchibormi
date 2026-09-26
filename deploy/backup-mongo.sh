#!/usr/bin/env bash
# Konteynerdagi MongoDB'ning kunlik zaxira nusxasi (gzip arxiv).
#
# Serverda baza konteyner ichida (`mongo_data` volume) turadi — ya'ni server
# o'chsa yoki volume yo'qolsa ma'lumot ham ketadi. Bu skript har kuni arxiv
# yaratadi va eskilarini tozalaydi. server-setup.sh uni /etc/cron.d ga yozadi.
#
# Qo'lda ishga tushirish:
#   PROJECT_DIR=/opt/ishchibormi deploy/backup-mongo.sh
#
# Off-site nusxadan tiklash: avval arxivni bucket'dan yuklab oling (R2/S3
# panelidan yoki `aws s3 cp s3://<bucket>/mongo/<fayl> .`), keyin quyidagicha.
# Tavsiya: oyiga bir marta sinov bazasiga tiklab ko'ring — tekshirilmagan
# zaxira zaxira emas.
#
# Tiklash (DIQQAT: mavjud ma'lumot ustiga yozadi):
#   gunzip -c /var/backups/ishchibormi/mongo-20260820-0330.gz \
#     | docker compose exec -T mongo mongorestore --archive --gzip --drop
set -euo pipefail

PROJECT_DIR="${PROJECT_DIR:-/opt/ishchibormi}"
BACKUP_DIR="${BACKUP_DIR:-/var/backups/ishchibormi}"
KEEP_DAYS="${KEEP_DAYS:-7}"
MONGO_DB="${MONGO_DB:-ishchibormi}"

cd "$PROJECT_DIR"

# MONGO_URI tashqi klasterga (masalan Atlas) qarab tursa, konteyner Mongo'si
# bo'sh bo'ladi va bu zaxira MA'NOSIZ. Shu holatni jimgina o'tkazib yubormaymiz.
if [ -f .env ] && grep -qE '^MONGO_URI=mongodb\+srv://' .env; then
	echo "[backup] MONGO_URI tashqi klasterga qaragan — konteyner zaxirasi o'tkazib yuborildi."
	echo "[backup] Tashqi baza zaxirasini provayder panelidan sozlang (Atlas: Backup)."
	exit 0
fi

mkdir -p "$BACKUP_DIR"
STAMP="$(date +%Y%m%d-%H%M)"
OUT="$BACKUP_DIR/mongo-$STAMP.gz"

echo "[backup] $(date -Is) — $MONGO_DB -> $OUT"
# --archive stdout'ga yozadi; `exec -T` TTY ajratmaydi, aks holda arxiv buziladi.
docker compose exec -T mongo mongodump --db "$MONGO_DB" --archive --gzip >"$OUT"

# Bo'sh/yarim yozilgan fayl "zaxira bor" degan yolg'on tuyg'u bermasin.
if [ ! -s "$OUT" ]; then
	echo "[backup] XATO: arxiv bo'sh — o'chirildi." >&2
	rm -f "$OUT"
	exit 1
fi

echo "[backup] Tayyor: $(du -h "$OUT" | cut -f1)"
find "$BACKUP_DIR" -name 'mongo-*.gz' -mtime "+$KEEP_DAYS" -delete
echo "[backup] $KEEP_DAYS kundan eski arxivlar tozalandi."

# ── Off-site nusxa (ixtiyoriy, lekin TAVSIYA ETILADI) ──────────────────────
# Yuqoridagi arxiv o'sha serverning diskida turadi: VPS yo'qolsa, baza ham,
# uning zaxirasi ham birga ketadi. Shuning uchun har bir arxiv S3-mos
# xotiraga (Cloudflare R2, Backblaze B2, AWS S3, ...) ham yuklanadi.
#
# Sozlash — server .env ida (qiymatlar hech qachon log'ga chiqmaydi):
#   BACKUP_S3_BUCKET=ishchibormi-backups
#   BACKUP_S3_ENDPOINT=https://<account>.r2.cloudflarestorage.com  # AWS'da bo'sh
#   BACKUP_S3_ACCESS_KEY_ID=...
#   BACKUP_S3_SECRET_ACCESS_KEY=...
#   BACKUP_S3_PREFIX=mongo/        # ixtiyoriy
# Xostga hech narsa o'rnatish shart emas: yuklash rasmiy aws-cli konteynerida.
# Bucket'dagi eski nusxalarni tozalash — bucket'ning lifecycle qoidasi bilan
# (masalan 30 kun).
env_val() {
	[ -f .env ] || return 0
	# `|| true`: kalit yo'q bo'lsa grep 1 qaytaradi va `set -eo pipefail`
	# skriptni ogohlantirishgacha yetmay to'xtatib qo'yardi.
	grep -E "^$1=" .env | tail -n 1 | cut -d= -f2- | sed -e "s/^['\"]//" -e "s/['\"]\$//" || true
}
S3_BUCKET="${BACKUP_S3_BUCKET:-$(env_val BACKUP_S3_BUCKET)}"
if [ -z "$S3_BUCKET" ]; then
	echo "[backup] OGOHLANTIRISH: off-site nusxa sozlanmagan (BACKUP_S3_BUCKET yo'q) — arxiv faqat shu serverda."
	exit 0
fi
S3_ENDPOINT="${BACKUP_S3_ENDPOINT:-$(env_val BACKUP_S3_ENDPOINT)}"
S3_PREFIX="${BACKUP_S3_PREFIX:-$(env_val BACKUP_S3_PREFIX)}"
AWS_ACCESS_KEY_ID="${BACKUP_S3_ACCESS_KEY_ID:-$(env_val BACKUP_S3_ACCESS_KEY_ID)}"
AWS_SECRET_ACCESS_KEY="${BACKUP_S3_SECRET_ACCESS_KEY:-$(env_val BACKUP_S3_SECRET_ACCESS_KEY)}"
AWS_DEFAULT_REGION="${BACKUP_S3_REGION:-$(env_val BACKUP_S3_REGION)}"
AWS_DEFAULT_REGION="${AWS_DEFAULT_REGION:-auto}"
# `-e NOM` (qiymatsiz) — qiymat buyruq qatoriga (ps) emas, muhitdan o'tadi.
export AWS_ACCESS_KEY_ID AWS_SECRET_ACCESS_KEY AWS_DEFAULT_REGION

NAME="$(basename "$OUT")"
echo "[backup] Off-site: s3://$S3_BUCKET/$S3_PREFIX$NAME"
if docker run --rm 	-e AWS_ACCESS_KEY_ID -e AWS_SECRET_ACCESS_KEY -e AWS_DEFAULT_REGION 	-v "$BACKUP_DIR:/backup:ro" 	amazon/aws-cli:2.17.0 	s3 cp "/backup/$NAME" "s3://$S3_BUCKET/$S3_PREFIX$NAME" --only-show-errors 	${S3_ENDPOINT:+--endpoint-url "$S3_ENDPOINT"}; then
	echo "[backup] Off-site nusxa yuklandi."
else
	# Lokal arxiv joyida — lekin cron log'i xatoni ko'rsatsin.
	echo "[backup] XATO: off-site nusxa yuklanmadi (arxiv lokal saqlangan)." >&2
	exit 1
fi
