# deploy/

Ishga tushirish bilan bog'liq fayllar. Ilova kodi bu yerda emas — u `apps/` da.

| Fayl | Vazifasi |
|------|----------|
| `docker-compose.dev.yml` | Lokal dev overlay (APP_ENV=dev, OTP_DEV_RETURN, localhost CORS) |
| `server-setup.sh` | Yangi Hetzner Cloud serverni noldan tayyorlaydi (Docker, Caddy, ufw, swap, deploy user, cron) |
| `Caddyfile` | Xostdagi Caddy konfiguratsiyasi — TLS, uchta xost (ommaviy sayt, mobil API, boshqaruv paneli) |
| `backup-mongo.sh` | Konteyner Mongo'ning kunlik gzip zaxirasi (7 kun saqlanadi) |

## Production — Hetzner Cloud server

```
Internet → :443 Caddy (xost, avtomatik Let's Encrypt)
  │
  ├── ishchibormi.uz
  │     ├── /admin*, /api/admin/*  → 404  (panel bu domenda YO'Q)
  │     ├── /api/*, /uploads/*     → 127.0.0.1:8080  (Go API)
  │     └── qolgan hamma narsa     → 127.0.0.1:3000  (Next.js)
  │
  ├── api.ishchibormi.uz            (mobil ilova)
  │     ├── /api/admin/*           → 404
  │     └── qolgani                → 127.0.0.1:8080
  │
  └── <boshqaruv subdomeni>         (nomi repoda yo'q — pastga qarang)
        ├── /api/*                 → 127.0.0.1:8080  (basic-auth'siz)
        └── qolgani (panel)        → 127.0.0.1:3000  (basic-auth ortida)

docker compose: mongo + backend + bot + frontend
                (barchasi faqat 127.0.0.1 ga bog'langan)
```

## Boshqaruv (admin) paneli — alohida subdomen

Panel ilgari `ishchibormi.uz/admin` da edi, ya'ni login formasi saytga kirgan
har kimga ochiq turardi. Endi u alohida xostda, uch qatlam ortida:

1. **Boshqa xost** — ommaviy domenlarning ikkalasida ham `/admin` va
   `/api/admin/*` 404 (Caddy), Next.js middleware'ida ham xuddi shu qoida
   takrorlangan (`apps/web/middleware.ts`).
2. **Nomi repoda yo'q** — bu repo ochiq, shuning uchun xost nomi faqat
   serverda: Caddy uni `/etc/caddy/panel.env` dan, Next.js esa `.env` dagi
   `ADMIN_PANEL_HOST` dan oladi. **Ikkalasi bir xil bo'lishi shart.**
3. **Basic-auth** — panel sahifalariga yetib borish uchun ham alohida parol
   kerak. `/api/*` ATAYLAB basic-auth'siz: mobil admin ilovasi
   `Authorization` sarlavhasini Bearer token uchun band qiladi.

### Serverda bir martalik sozlash

```bash
# 1) Caddy uchun maxfiy qiymatlar
sudo install -m 640 -o root -g caddy /dev/null /etc/caddy/panel.env
sudo tee /etc/caddy/panel.env >/dev/null <<'EOF'
IB_PANEL_HOST=boshqaruv-xxxxxx.ishchibormi.uz
IB_PANEL_USER=ib-panel
# Xeshni `caddy hash-password` chiqaradi. Bir tirnoq ichida yozilishi SHART:
# bcrypt xeshi `$` belgilarini o'z ichiga oladi.
IB_PANEL_HASH='$2a$10$...'
EOF

# 2) systemd Caddy'ga shu faylni ko'rsatsin
sudo mkdir -p /etc/systemd/system/caddy.service.d
sudo tee /etc/systemd/system/caddy.service.d/panel-env.conf >/dev/null <<'EOF'
[Service]
EnvironmentFile=/etc/caddy/panel.env
EOF
sudo systemctl daemon-reload

# 3) Konfiguratsiyani o'rnatish
sudo cp deploy/Caddyfile /etc/caddy/Caddyfile
sudo caddy validate --config /etc/caddy/Caddyfile
sudo systemctl restart caddy   # EnvironmentFile o'zgarganda reload yetmaydi
```

DNS'da subdomen uchun A-yozuv server IP'siga yo'naltirilgan bo'lishi kerak —
Caddy sertifikatni o'zi oladi.

Sozlama berilmasa blok **inert**: xost `panel.localhost` ga tushadi va parol
xeshi hech kim bilmaydigan tasodifiy qiymatniki. Ya'ni env fayl yo'qolsa ham
Caddy ishga tushaveradi va asosiy sayt o'chib qolmaydi.

> **Nomi o'zgarsa** uchta joyni birga yangilash kerak:
> 1. `/etc/caddy/panel.env` → `sudo systemctl restart caddy`
> 2. `ADMIN_PANEL_HOST` (GitHub secret + serverdagi `.env`) → `docker compose up -d frontend`
>    (qiymat runtime'da o'qiladi, qayta build shart emas)
> 3. admin mobil ilovasidagi `ApiConfig.prodBaseUrl` → APK qayta yig'iladi

Caddy ATAYLAB compose ichida emas, xostda: `docker compose down` qilinganda
ham 80/443 va sertifikatlar joyida qoladi, ya'ni deploy paytida domen o'lmaydi.
Shu sababli `Caddyfile` o'zgarsa CI uni yangilamaydi — qo'lda:

```bash
sudo cp deploy/Caddyfile /etc/caddy/Caddyfile
sudo caddy validate --config /etc/caddy/Caddyfile
sudo systemctl reload caddy
```

Birinchi o'rnatish: `deploy/server-setup.sh` (skript ichidagi izohga qarang).
Undan keyingi har bir deploy — `main`'ga push (`.github/workflows/ci-cd.yml`).

Kerakli GitHub secrets: `DEPLOY_HOST`, `DEPLOY_USER`, `DEPLOY_SSH_KEY`,
`PROJECT_DIR`, `TELEGRAM_BOT_TOKEN`, `JWT_ACCESS_SECRET`, `JWT_REFRESH_SECRET`,
`BOT_SHARED_SECRET`, `ADMIN_SEED_PASS`, `ADMIN_PANEL_HOST`
(S3, `MONGO_URI` va `GEMINI_API_KEY` — ixtiyoriy).

OTP boti — `@Ishchibormibot`. `TELEGRAM_BOT_TOKEN` secretida shu botning
tokeni bo'lishi kerak; deploy `TELEGRAM_BOT_USERNAME=Ishchibormibot` ni
serverdagi `.env` ga yozadi. Frontend bot nomini build vaqtida oladi,
shuning uchun bot o'zgarganda backend, bot va frontend birga qayta deploy
qilinadi. Lokal ishga tushirishda ildizdagi `.env` hamda
`apps/web/.env.local` dagi `NEXT_PUBLIC_BOT_USERNAME` yangilanadi.

Eski bot bilan suhbat yangi botga ko'chmaydi: foydalanuvchi yangi botda
`Start` bosishi kerak. `ERROR_ALERT_CHAT_ID` ishlatilsa, yangi botga o'sha
guruh yoki kanalga xabar yuborish huquqini ham bering.

## Nega asosiy `docker-compose.yml` ildizda qoldi

Uni ham shu papkaga ko'chirish tabiiy ko'rinadi, lekin bu **prod ma'lumotini
yo'qotadi**. Sababi — Compose "loyiha nomi" (project name):

- Compose loyiha nomini **birinchi `-f` fayli turgan papka nomidan** oladi
- Nomlangan volume'lar shu prefiks bilan yaratiladi: `<loyiha>_uploads`,
  `<loyiha>_avatars`, `<loyiha>_mongo_data`
- Compose fayli `deploy/` ga ko'chsa, loyiha nomi `deploy` bo'lib qoladi va
  Compose eski volume'larni **topa olmay, yangi bo'shlarini yaratadi**

Serverdagi `uploads` va `avatars` volume'larida foydalanuvchilar yuklagan
rasmlar turadi. Ya'ni bu shunchaki noqulaylik emas — jonli ma'lumot
ko'rinmay qoladi (o'chmaydi, lekin orfan bo'lib qoladi).

Xuddi shu sabab `.env` ga ham tegishli: Compose uni loyiha papkasidan
o'qiydi, ya'ni `${MONGO_URI:-...}` kabi barcha interpolatsiyalar ildizdagi
`.env` ni ko'rishi kerak.

Shuning uchun qoida:

```bash
# TO'G'RI — ildizdan, prod compose birinchi
docker compose up -d --build
docker compose -f docker-compose.yml -f deploy/docker-compose.dev.yml up -d --build

# NOTO'G'RI — loyiha nomi "deploy" bo'lib ketadi
cd deploy && docker compose -f docker-compose.dev.yml up -d
```

`make up` / `make dev` allaqachon to'g'ri shaklda chaqiradi — shulardan
foydalangan ma'qul.

## Keyingi qadam (hali qilinmagan)

Hozir image'lar **serverning o'zida** build qilinadi (`docker compose
build`), ya'ni prod mashinaning RAM va CPU'sida. Next.js build'i RAM yetmay
"Killed" bo'lishi mumkin va deploy paytida sayt sekinlashadi. (`server-setup.sh`
shu sabab 2GB swap yaratadi — bu yamoq, yechim emas.)

Rejalashtirilgan o'zgarish: image'lar GitHub Actions'da build qilinib
registry'ga (GHCR) push qilinadi, server esa faqat `docker compose pull` qiladi.
Bu deploy'ni soniyalarga tushiradi va teglangan image orqali **rollback**
imkonini beradi (hozir buning imkoni yo'q).
