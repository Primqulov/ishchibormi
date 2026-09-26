# Ishchi Bormi — server tomoni (API / Web / botlar)

O'zbekiston uchun kunlik ish (mardikor) bozori platformasi. Telegram orqali parolsiz
(OTP) kirish, MongoDB'ga asoslangan Go API, Next.js (App Router + TypeScript +
Tailwind) veb-panel va Go Telegram botlari.

---

## 1. Stack (texnologiyalar)

| Qatlam    | Texnologiya |
|-----------|-------------|
| Frontend  | Next.js (App Router) + TypeScript + Tailwind CSS |
| Backend   | Go (chi router, mongo-go-driver, JWT) |
| Bot       | Go (`go-telegram-bot-api/v5`) |
| DB        | MongoDB 7 (OTP uchun TTL kolleksiya — Redis kerak emas) |
| Fayllar   | AWS S3 **yoki** lokal disk (S3 sozlanmasa avtomatik lokal) |
| Infra     | Docker + docker-compose, GitHub Actions CI/CD → Contabo VPS (Caddy + TLS) |

---

## 2. Papka tuzilmasi

Har bir ishga tushiriladigan birlik `apps/` ostida alohida papkada; ildizda
faqat butun tizimga tegishli fayllar (compose, `.env`, CI) turadi.

```
.
├── apps/
│   ├── api/                  # Go API server + seed
│   │   ├── cmd/api/main.go   # kirish nuqtasi: router, middleware, route'lar
│   │   ├── config/           # env'dan konfiguratsiya (Config.Load)
│   │   ├── internal/         # domenlar (har biri handler.go)
│   │   ├── pkg/              # umumiy yordamchi paketlar
│   │   └── seed/main.go      # demo ma'lumot to'ldirish
│   ├── web/                  # Next.js veb-panel
│   │   ├── app/              # sahifalar (App Router)
│   │   ├── components/       # UI komponentlar
│   │   └── lib/              # API klient, i18n, format, xarita
│   └── bots/
│       └── otp/              # OTP yetkazuvchi bot (Mongo'ga yozadi)
├── deploy/                   # Server setup, Caddyfile, backup, dev overlay
├── scripts/                  # play-preflight.sh
├── .github/workflows/        # CI/CD pipeline (test + prod deploy)
├── docker-compose.yml        # mongo + backend + bot + frontend
├── Makefile
├── .env.example              # BITTA .env — barcha servislar shundan o'qiydi
└── README.md
```

**Nega bitta repo.** Bu servislar bitta serverda, bitta `docker compose`
bilan, bitta Mongo ustida ishlaydi va birga deploy qilinadi — ya'ni bitta
yetkazish birligi. Mustaqil deploy kerak bo'lganda yechim repo bo'lish emas,
CI'da yo'l filtri (`paths:`) — har servis faqat o'zi o'zgarganda quriladi.
Repo'ga bo'lish faqat **alohida jamoalar** paydo bo'lganda mantiqiy
(`git subtree split -P apps/api` bilan tarixi saqlanib ajratiladi).

Mobil ilova (Flutter) bu repoda emas — u alohida, chunki toolchain'i
(Xcode/Gradle), CI runner'i va reliz tezligi (store review) butunlay boshqa
va u serverga deploy qilinmaydi.

---

## 3. Backend — `apps/api/`

### 3.1 Kirish nuqtasi
- **`cmd/api/main.go`** — server ishga tushishi. Mongo ulanadi, indekslar
  yaratiladi, storage (S3/lokal) tayyorlanadi, barcha handler'lar ulanadi,
  middleware'lar (RequestID, Logger, Recover, SecurityHeaders, CORS, rate-limit)
  o'rnatiladi va route'lar ro'yxatga olinadi. Graceful shutdown bilan.
- **`seed/main.go`** — har bir kolleksiyaga realistik demo yozuvlar qo'yadi.

### 3.2 Domen paketlari — `internal/`
Har bir paketda `handler.go` bor; funksiyalari HTTP endpoint'larga bog'langan.

| Paket | Fayl | Vazifasi |
|-------|------|----------|
| `auth` | `handler.go`, `otp.go` | OTP so'rash/tekshirish, JWT (access/refresh) chiqarish, refresh, foydalanuvchini upsert qilish. `otp.go` — OTP kodlarini Mongo TTL kolleksiyada saqlaydi. |
| `admin` (sessiya) | `refresh.go`, `session.go` | Admin sessiyasi: qisqa umrli access token + aylanuvchi refresh token (veb — HttpOnly cookie, mobil — javob tanasi). 3 kun foydalanilmasa hisobning barcha sessiyalari yopiladi; oyna veb va ilova uchun BIRGA yuritiladi. |
| `user` | `handler.go` | `Me`/`UpdateMe` (o'z profili), `GetPublic` (ochiq profil), `Search` (qidirish), `Block`/`Unblock`. |
| `category` | `handler.go` | `List` (turkumlar). Turkumlarni faqat admin belgilaydi. |
| `elon` | `handler.go`, `price_test.go` | E'lon CRUD + `Feed` + `MyElons`. Narx hisoblash (`computePrice`), muddat tekshiruvi (`isExpired`/`notExpiredExpr`), **ish sanasi 3 kun ichida ekanini tekshirish (`validateStartDate`)**. Geokodlash orqali viloyat/tuman aniqlanadi. |
| `application` | `handler.go` | Ariza berish (`Apply`), qabul/rad (`Accept`/`Reject`), bekor (`Cancel`), yakunlash (`ConfirmDone`), o'z arizalarim/e'lonimga kelgan arizalar, tarix (`History`). |
| `review` | `handler.go` | Baho/izoh qoldirish (`Create`), foydalanuvchi baholari (`ListForUser`). |
| `report` | `handler.go` | Shikoyat (`Create`), admin ro'yxati + hal qilish. |
| `feedback` | `handler.go` | Taklif/shikoyat (`Create`/`Mine`), admin ro'yxati + hal qilish. |
| `notification` | `service.go` | Bildirishnoma yaratish (`Push`), ro'yxat (`List`), hammasini o'qilgan qilish (`ReadAll`). |
| `upload` | `handler.go` | Fayl yuklash (`Upload`) / o'chirish (`Delete`) — S3 yoki lokal. |
| `admin` | `handler.go` | Admin login + dashboard, foydalanuvchi/e'lon/turkum boshqaruvi, shikoyat/feedback hal qilish, ommaviy xabar (`Broadcast`), audit jurnali. |
| `models` | `models.go` | MongoDB hujjat tuzilmalari (quyida). |

### 3.3 Ma'lumot modellari — `internal/models/models.go`
`User`, `PublicUser`, `Category`, `Elon`, `Application`, `Review`,
`Notification` (+`RelatedEntity`), `Report`, `Feedback`, `Admin`,
`AdminAudit`, `OTPCode`.

### 3.4 Umumiy paketlar — `pkg/`
| Paket | Vazifasi |
|-------|----------|
| `db` | Mongo ulanishi va indekslarni yaratish (`EnsureIndexes`). |
| `envfile` | `.env` faylini o'qish. |
| `geocode` | Koordinatadan viloyat/tuman aniqlash (reverse geocoding). |
| `httpx` | JSON javob/xato, JWT auth middleware (`UserAuth`/`AdminAuth`), rate limiter, xavfsizlik header'lari, xavfsiz URL tekshiruvi (XSS'ga qarshi). |
| `logger` | Strukturaviy log. |
| `storage` | Fayl saqlash — AWS S3 (`New`) yoki lokal disk (`NewLocal`). |
| `validator` | So'rov maydonlarini tekshirish. |

---

## 4. API — to'liq ro'yxat

Bazaviy prefiks: `/api`. Autentifikatsiya: `Authorization: Bearer <accessToken>`.

### 4.1 Ochiq (auth talab qilmaydi)
| Metod | Yo'l | Handler | Vazifasi |
|-------|------|---------|----------|
| GET | `/healthz` | — | Sog'liqni tekshirish. |
| GET/HEAD | `/uploads/*` | — | Lokal saqlangan fayllar (S3 yo'q paytda). |
| POST | `/api/auth/otp/request` | `auth.RequestOTP` | OTP so'rash → `{ tgToken }` (+ dev'da `{ code }`). |
| POST | `/api/auth/otp/verify` | `auth.VerifyOTP` | Kodni tekshirish → `{ accessToken, refreshToken, user }`. |
| GET | `/api/auth/otp/peek` | `auth.DevPeekOTP` | (Dev) kodni ko'rish. |
| POST | `/api/auth/refresh` | `auth.Refresh` | Access token'ni yangilash. |
| GET | `/api/elons` | `elon.Feed` | Ommaviy e'lonlar (qidiruv/filtr/sahifalash). |
| GET | `/api/elons/nearby?lat=&lng=&page=&limit=` | `elon.Nearby` | Joylashuvga eng yaqin faol ishlar, masofa bo'yicha tartiblangan. |
| GET | `/api/elons/{id}` | `elon.Get` | Bitta e'lon (ko'rishlar +1). |
| GET | `/api/users/{id}` | `user.GetPublic` | Ochiq profil. |
| GET | `/api/users?q=` | `user.Search` | Foydalanuvchi qidirish. |
| GET | `/api/categories` | `category.List` | Turkumlar. |
| GET | `/api/users/{id}/reviews` | `review.ListForUser` | Foydalanuvchi baholari. |

### 4.2 Auth-himoyalangan (JWT + faol foydalanuvchi)
| Metod | Yo'l | Handler | Vazifasi |
|-------|------|---------|----------|
| GET | `/api/me` | `user.Me` | O'z profilim. |
| PATCH | `/api/me` | `user.UpdateMe` | Profilni yangilash. |
| POST | `/api/users/{id}/block` | `user.Block` | Bloklash. |
| DELETE | `/api/users/{id}/block` | `user.Unblock` | Blokni yechish. |
| POST | `/api/elons` | `elon.Create` | E'lon yaratish (**sana 3 kun ichida**). |
| PATCH | `/api/elons/{id}` | `elon.Update` | Tahrirlash. |
| DELETE | `/api/elons/{id}` | `elon.Delete` | O'chirish. |
| GET | `/api/my/elons` | `elon.MyElons` | Mening e'lonlarim (faol/arxiv). |
| POST | `/api/elons/{id}/apply` | `application.Apply` | Ishga ariza (rate-limit). |
| POST | `/api/applications/{id}/accept` | `application.Accept` | Qabul qilish. |
| POST | `/api/applications/{id}/reject` | `application.Reject` | Rad etish. |
| POST | `/api/applications/{id}/cancel` | `application.Cancel` | Bekor qilish. |
| POST | `/api/applications/{id}/confirm-done` | `application.ConfirmDone` | Ish bajarilganini tasdiqlash. |
| POST | `/api/applications/{id}/review` | `review.Create` | Baho/izoh. |
| GET | `/api/my/applications` | `application.MyApplications` | Mening arizalarim. |
| GET | `/api/my/elons/applications` | `application.MyElonsApplications` | E'lonlarimga kelgan arizalar. |
| GET | `/api/me/history` | `application.History` | Ish tarixim. |
| GET | `/api/notifications` | `notification.List` | Bildirishnomalar. |
| POST | `/api/notifications/read-all` | `notification.ReadAll` | Hammasini o'qilgan qilish. |
| POST | `/api/reports` | `report.Create` | Shikoyat. |
| POST | `/api/feedback` | `feedback.Create` | Taklif/shikoyat. |
| GET | `/api/feedback` | `feedback.Mine` | Mening murojaatlarim. |
| POST | `/api/uploads` | `upload.Upload` | Fayl yuklash. |
| DELETE | `/api/uploads` | `upload.Delete` | Faylni o'chirish. |

### 4.3 Admin (`/api/admin`, alohida admin JWT)

> **O'chirishning ikki xili.** Admin panelidan o'chirish `?mode=` bilan
> boshqariladi (`internal/admin/deletemode.go`):
> `hidden` (standart) — ma'lumot foydalanuvchilardan butunlay ketadi, lekin
> admin panelida ko'rinib turadi va bazada saqlanadi; `purge` — bazadan
> butunlay o'chiriladi (faqat superadmin, `account.Purger` orqali).
> Ikkalasi ham **qaytarilmaydi**: `isDeleted` ni bekor qiladigan yo'l
> kodda umuman yo'q.

> Bu yo'llar **faqat boshqaruv subdomenida** javob beradi. `ishchibormi.uz` va
> `api.ishchibormi.uz` da ular 404 — panel va uning API'si oddiy foydalanuvchi
> biladigan domenlarda umuman ko'rinmaydi (`deploy/Caddyfile`).

| Metod | Yo'l | Handler | Vazifasi |
|-------|------|---------|----------|
| POST | `/api/admin/login` | `admin.Login` | Admin kirishi (rate-limit). |
| POST | `/api/admin/refresh` | `admin.Refresh` | Sessiyani yangilash — access token + aylanuvchi refresh token. |
| GET | `/api/admin/dashboard` | `admin.Dashboard` | Statistika. |
| GET | `/api/admin/users` | `admin.ListUsers` | Foydalanuvchilar (`?deleted=hide|only` — o'chirilganlar standart holatda ham ko'rinadi). |
| POST | `/api/admin/users/{id}/block` | `admin.BlockUser` | Bloklash. |
| DELETE | `/api/admin/users/{id}` | `admin.DeleteUser` | O'chirish — `?mode=hidden` (standart) yoki `?mode=purge` (faqat superadmin). |
| GET | `/api/admin/elons` | `admin.ListElons` | E'lonlar (`?deleted=hide|only`). |
| DELETE | `/api/admin/elons/{id}` | `admin.DeleteElon` | O'chirish — `?mode=hidden` (standart) yoki `?mode=purge` (faqat superadmin). |
| GET | `/api/admin/categories` | `admin.ListCategories` | Turkumlar. |
| PATCH | `/api/admin/categories/{id}/active` | `admin.SetCategoryActive` | Yoqish/o'chirish. |
| GET | `/api/admin/reports` | `report.ListAdmin` | Shikoyatlar. |
| PATCH | `/api/admin/reports/{id}/resolve` | `report.Resolve` | Hal qilish. |
| GET | `/api/admin/feedback` | `feedback.ListAdmin` | Murojaatlar. |
| PATCH | `/api/admin/feedback/{id}/resolve` | `feedback.Resolve` | Hal qilish. |
| POST | `/api/admin/broadcast` | `admin.Broadcast` | Ommaviy bildirishnoma. |
| GET | `/api/admin/audit` | `admin.Audit` | Admin amallari jurnali. |

---

## 5. Botlar — `apps/bots/`

- **`otp/cmd/bot/main.go` (OTP boti)** — foydalanuvchi `t.me/<bot>?start=<token>`
  orqali `/start` bosadi, kontaktini ulashadi. Bot 6 xonali kodni yaratib
  Mongo'ning OTP kolleksiyasiga (TTL) yozadi; shu kod bilan
  `/api/auth/otp/verify` orqali kirish yakunlanadi.
- **`otp/internal/envfile/`** — `.env` yuklovchi.

Bot alohida Go moduli. OTP oqimi Mongo orqali ishlaydi; `/post` orqali
e'lon berish esa mavjud API endpointlaridan foydalanadi. To'liq oqim,
platformadagi maydonlar bilan moslik va sozlamalar:
[Telegram orqali e'lon berish](docs/telegram-posting.md).

**Ish bildirishnomalari** ham shu botdan keladi: ariza yuborilgani tasdig'i,
ish beruvchiga yangi ariza, qabul/rad,
bekor qilish (jumladan joy to'lishi), ish shartlari o'zgarishi, yakunlash
so'rovi va ish yakunlanishi. API `TELEGRAM_BOT_TOKEN` orqali foydalanuvchining
kirishda bog'langan `telegramId` manziliga yuboradi; bot qaytadan polling
qilinmaydi. Admin/dasturchi xabarlari, broadcast, xavfsizlik va noma'lum
turdagi bildirishnomalar botga uzatilmaydi.

Telegram navbati bildirishnoma bilan bitta Mongo hujjatida saqlanadi va API
xabar ostiga `WEB_BASE_URL` asosida arizani yoki e'lonni ochadigan **inline
tugma** qo'yadi (`/applications/{id}`, `/elon/{id}`, aks holda
`/notifications`). Standart sayt `https://ishchibormi.uz`; lokal test uchun
`WEB_BASE_URL=http://127.0.0.1:3000` belgilanadi va shu origin
`CORS_ORIGINS` ro'yxatiga qo'shiladi. Telegram `localhost` tugmasini rad etib,
butun xabarni yubormaydi; numeric loopback havolasi faqat shu kompyuterda
ishlaydi. Tugma URL'i Telegram tomonidan rad etilsa, xabarning o'zi
tugmasiz qayta yuboriladi. Ariza tafsilotlarini faqat
ariza beruvchi va e'lon egasi ko'radi; login va profilni to'ldirishdan keyin
foydalanuvchi shu havolaga qaytadi.

Ariza qabul qilinganda ishchiga yuboriladigan xabarda ish nomi, sana va
hafta kuni, ish vaqti (Toshkent vaqti), ish beruvchining aloqa raqami,
manzil va «Xaritada ochish» havolasi ham bo'ladi. E'londagi aloqa raqami
bo'sh bo'lsa, ish beruvchining profil raqami olinadi. Tafsilotlar qabul
paytida navbatga saqlanadi; kiritilmagan sana, vaqt yoki lokatsiya xabarda
ochiq ko'rsatiladi. «Arizani ko'rish» tugmasi ham qoladi.

Tugmadagi manzil oddiy `https://` havola — ataylab. `ishchibormi.uz` tasdiqlangan
Android App Link (`/.well-known/assetlinks.json` + ilova manifesti), shuning
uchun ilova o'rnatilgan telefonda Telegram havolani **ilovaga** beradi, qolgan
hamma joyda (desktop, iOS, ilovasiz telefon) **veb sahifa** ochiladi. `tg://`
yoki maxsus sxema ikkalasini ham qila olmaydi va Telegram uni tugmada qabul
qilmaydi.

> Tugmaning har bir yo'li ikki joyda ro'yxatdan o'tishi SHART:
> `flutter-app/android/app/src/main/AndroidManifest.xml` dagi intent-filter va
> `DeepLinkService.parse`. Faqat birinchisida bo'lsa — ilova ochiladi-yu, ekran
> topilmaydi; faqat ikkinchisida bo'lsa — ilova bor bo'lsa ham brauzer ochiladi.
> iOS'da Associated Domains hali sozlanmagan, ya'ni iPhone'da tugma doim veb
> sahifani ochadi.

Telegram navbati API
qayta ishga tushganda davom etadi. Vaqtinchalik xatolar 12 urinishgacha
qayta tekshiriladi, Telegram `retry_after` muddati hisobga olinadi. Botni
bloklagan, Telegram'i bog'lanmagan, o'chirilgan yoki demo hisoblarga xabar
yuborilmaydi. Oldingi bildirishnomalar qayta tarqatilmaydi. FCM sozlamalari
bu kanalga ta'sir qilmaydi. Telegram yuborishni tasdiqlagan zahoti jarayon
uzilsa, keyingi urinishda takroriy xabar ehtimoli bor (at-least-once).

> Eslatma: qo'llab-quvvatlash endi shaxsiy Telegram akkaunti orqali — avvalgi
> taklif/shikoyat botlari (`cmd/feedbackbot`, `bot_feedback`/`support_admins`
> kolleksiyalari) 2026-07-23 da butunlay olib tashlandi.

---

## 6. Frontend — `apps/web/`

Next.js App Router. Uch guruh: ochiq `(public)`, kabinet `(cabinet)`, admin `admin/`.

### 6.1 Sahifalar — `app/`
- **`(public)/`** — `page.tsx` (bosh sahifa/e'lonlar), `login`, `elon/[id]`,
  `u/[id]` (ochiq profil), `biz-haqimizda`, `yordam`,
  `foydalanish-shartlari`, `maxfiylik-siyosati`.
- **`(cabinet)/`** — `dashboard`, `elon/create`, `elon/[id]/edit`, `my-elons`,
  `history`, `notifications`, `feedback`, `process`, `profile`, `settings`,
  `onboarding` (+ `layout.tsx`).
- **`admin/`** — `login`, `page` (dashboard), `users`, `elons`, `categories`,
  `reports`, `feedback`, `notifications`, `audit`. Bu guruh faqat boshqaruv
  subdomenida ochiladi; ommaviy domenda `middleware.ts` uni 404 qiladi.

### 6.2 Yordamchilar — `lib/`
`api.ts` (API klient), `i18n.ts` (uz/ru/en + lotin/kirill), `format.ts`,
`leaflet.ts` + `url.ts` (xarita/URL), `upload.ts`.

### 6.3 Komponentlar — `components/`
`Shell`, `Sidebar`, `CabinetNavbar`, `JobCard`, `Modal`, `ShareModal`,
`StatusBadge`, `ThemeToggle`, `ScriptToggle`, `T`, `Providers` va `ui/`
(Avatar, Button, Card, EmptyState, ImageUpload, Input, MapPicker, MapView,
Skeleton, Tabs).

---

## 7. Ishga tushirish

### Docker bilan
```bash
cp .env.example .env
# .env'da TELEGRAM_BOT_TOKEN, JWT sirlari, AWS S3 (ixtiyoriy) to'ldiring
make dev                                # yoki: docker compose up --build -d
docker compose exec backend /app/seed   # demo ma'lumot
# frontend: http://localhost:3000 | API: http://localhost:8080
```

> Barcha compose buyruqlari **repo ildizidan** ishlatilishi kerak. Sabab va
> `deploy/docker-compose.dev.yml` ning to'g'ri chaqirilishi:
> [deploy/README.md](deploy/README.md).

### Portlar (docker-compose)
| Xizmat | Port |
|--------|------|
| frontend | 3000 |
| backend | 8080 |
| mongo | 27018 → 27017 |
| bot | (tashqi port yo'q) |

### Kirish oqimi
`/login` → "Telegram botga o'tish" → botda `/start` + kontakt → 6 xonali kod →
`/login`da kiritish. `OTP_DEV_RETURN=true` bo'lsa kod so'rov javobida ham
qaytadi (botsiz test uchun).

---

## 8. CI/CD → prod server (Contabo VPS)

`.github/workflows/ci-cd.yml`:
- **test** (har push/PR): `apps/api` va OTP boti uchun `go vet` + `go test`,
  `apps/web` uchun `npm ci` + `lint` + `build`.
- **deploy** (faqat `main`'ga push, test o'tgach): prod serverga SSH
  orqali `git reset --hard origin/main` + `docker compose up --build`.
  ⚠️ `main`'ga push = **production deploy**.

Kerakli GitHub secrets: `DEPLOY_HOST`, `DEPLOY_USER`, `DEPLOY_SSH_KEY`,
`PROJECT_DIR` + ilova sirlari (`TELEGRAM_BOT_TOKEN`, `JWT_*`,
`BOT_SHARED_SECRET`, `ADMIN_SEED_PASS`).

Serverni birinchi marta tayyorlash — `deploy/server-setup.sh` (Docker, Caddy,
ufw, swap, deploy user, kunlik Mongo zaxirasi). TLS'ni xostdagi Caddy beradi
(`deploy/Caddyfile`), sertifikat avtomatik yangilanadi. Batafsil arxitektura va
`Caddyfile` ni yangilash tartibi: [deploy/README.md](deploy/README.md).

### Rejalashtirilgan yaxshilanishlar
1. **Image'larni CI'da qurish** (GHCR) — hozir build serverning o'zida ketadi,
   ya'ni prod mashinaning RAM/CPU'sida (Next.js build "Killed" bo'lishi
   mumkin). Registry'ga o'tilsa deploy `pull` ga aylanadi va **rollback**
   paydo bo'ladi. Batafsil: [deploy/README.md](deploy/README.md).
2. **Yo'l filtri** (`paths:`) — har servis faqat o'zi o'zgarganda qurilishi.
   Buni (1) dan keyin qilgan ma'qul, chunki mustaqil deploy uchun baribir
   servis-bo'yicha image kerak.
