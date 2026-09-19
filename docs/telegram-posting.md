# Telegram bot: ish topish va Mini App orqali e'lon berish

Yangi e'lonlarni kanalga chiqarish va aniq ishni botda ochish:
[Telegram kanalini ulash](telegram-channel.md).

## E'lon berish

Bot menyusidagi **E'lon berish** Telegram Mini App'ni ochadi. `/post` ham
shu ilovani ochadigan tugmani yuboradi. Chatdagi eski savol-javob formasi
olib tashlangan; eski `/preview`, `/back` va qoralama tugmalari endi Mini
App'ga yo'naltiradi, eski qoralamani chop etmaydi.

1. Telegram imzolagan ma'lumot bilan hisob avtomatik aniqlanadi.
2. Telefon oldin tasdiqlanmagan bo'lsa, foydalanish shartlariga rozilik va
   Telegram orqali o'z raqamini ulashish so'raladi. Oddiy yozilgan raqam
   yoki boshqa odamning kontakti hisobga kirish uchun qabul qilinmaydi.
3. Yetishmayotgan ism, yashash viloyati va tumani to'ldiriladi. Sayt, mobil
   ilova va botdagi mavjud hisob bir xil qoladi.
4. Saytdagi ayni `CreateElonForm` komponenti ochiladi: sarlavha, kategoriya,
   tavsif, xarita, kim kerakligi, ishchilar soni, to'lov turi va miqdori,
   sana-vaqt, aloqa telefoni va oltitagacha rasm.
5. **E'lonni joylashtirish** mavjud API tekshiruvlari, rasm egaligi va
   moderatsiyasi orqali e'lonni saqlaydi. Kanalga chiqarish ham API'dagi
   umumiy navbat orqali ishlaydi; bot foydalanuvchilariga ommaviy xabar ketmaydi.
6. Muvaffaqiyat ekranida **Botda e'lonni ko'rish**, **Yana e'lon berish** va
   **Yopish** bor. Botdagi havola aynan yangi ishning tafsilotlarini ochadi.

Aloqa telefoni ataylab oldindan to'ldirilmaydi: e'londagi telefon profil
telefonidan farq qilishi mumkin. Ish haqi bo'sh yoki 0 bo'lsa kelishiladi;
har bir ishchiga yoki umumiy summaga narx hisoblanadi. Sana bugun, ertaga
yoki indin; vaqt kamida bir soatdan keyin. JPG/PNG/WebP rasmlar 8 MB gacha.

Mini App ochiq turganda xato sababini ko'rsatib, kiritilgan ma'lumotlarni
saqlab qoladi. Yuborishni takrorlash bir xil 24 belgilik hex
`Idempotency-Key` bilan amalga oshadi va bir e'lon ikkinchi marta yaratilmaydi.
Qisqa tarmoq/HTTPS uzilishlarida Mini App sessiya, o'qish va kaliti bor e'lon
yaratish so'rovlarini cheklangan oraliqda uch martagacha qayta urinadi.
Har safar ayni mazmun va kalit yuboriladi; rasmlar yuklash kabi boshqa yozuv
amallari avtomatik takrorlanmaydi. Uzilish davom etsa forma ochiq qoladi va
foydalanuvchi ma'lumotni qayta kiritmasdan yuborishni takrorlashi mumkin.
Forma sahifani yopgandan keyin tiklanadigan qoralama sifatida saqlanmaydi;
Telegram yopishdan oldin tasdiq so'raydi.

## Ish qidirish va arizalar

- `/start` yoki `/menu`: yaqin ishlar, arizalar, qabul qilingan ishlar,
  kelgan arizalar, e'lon berish va yordam.
- `/jobs`: joylashuv yuborish, eng yaqinidan saralash, bugun/ertaga/barcha
  kunlar va kategoriya filtrlari. Qidirish ro'yxatdan o'tishni talab qilmaydi.
  Telegram lokatsiyasi, venue yoki `lat, lng` matni qabul qilinadi.
- Natijalar 5 tadan sahifalanadi. Masofa to'g'ri chiziq bo'yicha taxminiy,
  yo'l uzunligi emas. Bo'sh o'rinli faol ishlar ko'rsatiladi.
- E'lonning batafsil kartasi joriy bo'sh o'rinlar, telefon va xaritani beradi.
  Bitta rasm tavsifi bilan, bir nechta rasm bitta albomda umumiy tavsif bilan
  yuboriladi. Uzun matn **To'liq tavsif** tugmasidan ochiladi.
- **Ariza berish**: shartlar, o'z kontakti, yetishmagan profil, kishi soni,
  ko'rib chiqish va tasdiqlash. Ish beruvchiga yuboriladigan ma'lumotlar aytiladi.
- `/applications`: kutilayotgan, qabul qilingan va tarixdagi arizalar.
  `/myjobs`: qabul qilingan ishning telefoni, joyi va vaqti.
- Arizani bekor qilish va **Ishni tugatdim** tasdiq bilan bajariladi.
  Yakunlash ikki tomon tasdig'iga tayanadi, pul to'langanini tasdiqlamaydi.
- `/candidates`: ish beruvchi kelgan arizalarni qabul/rad qiladi, bekor
  qiladi yoki ish yakunini tasdiqlaydi.

Qidiruv lokatsiyasi bot xotirasida bir soat saqlanadi; `/cancel`, `/post`
yoki restart qidiruvni tugatadi. Ariza suhbatlari Mongo'da saqlanadi,
eski tugmalar va takroriy update'lar tekshiriladi. OTP login oqimi saqlangan.

## Texnik ulanish

- `POST /api/auth/miniapp/session` xom `initData` va kerak bo'lsa imzolangan
  `contactData` qabul qiladi. Telegram HMAC tekshiriladi, ma'lumot 10 daqiqadan
  eski yoki kelajakdan kelgan bo'lsa rad etiladi. Kontakt egasi Telegram
  foydalanuvchisiga mos bo'lishi va rozilik berilgan bo'lishi kerak.
- JWT faqat Mini App'ning `sessionStorage` joyida saqlanadi, oddiy saytdagi
  hisob sessiyasiga aralashmaydi. Bot tokeni brauzerga berilmaydi.
- Next.js `/miniapp/api/*` proksisi faqat sessiya, kategoriyalar, profil,
  e'lon yaratish, rasmlar yuklash/o'chirish va lokal rasmni ko'rish yo'llarini
  uzatadi. Admin, OTP, debug va ixtiyoriy URL'lar proksi qilinmaydi.
- Ish qidirish va ariza amallari botning mavjud imzolangan
  `POST /api/auth/bot/session` autentifikatsiyasidan foydalanadi.
- Bitta Telegram poller ishlaydi. Eski chat qoralamalari bazadan o'chirilmaydi,
  lekin ularni nashr qiluvchi oqim endi yo'q.

## Sozlash

API va bot bir xil `TELEGRAM_BOT_TOKEN`, `TELEGRAM_BOT_USERNAME`,
`BOT_SHARED_SECRET` va bazadan foydalanadi. Qo'shimcha sozlamalar:

```dotenv
TELEGRAM_MINIAPP_URL=https://ishchibormi.uz/miniapp/post
MINIAPP_API_BASE_URL=http://127.0.0.1:8080
BOT_API_BASE_URL=http://127.0.0.1:8080
```

`TELEGRAM_MINIAPP_URL` HTTPS bo'lishi majburiy. Bot ishga tushganda Telegram
chat menyusidagi **E'lon berish** Web App tugmasini ham o'rnatadi.
`MINIAPP_API_BASE_URL` faqat Next.js serverida o'qiladi; Docker Compose
uni `http://backend:8080` qiladi. Telefon brauzeri localhost API'ga ulanmaydi.

Production uchun backend, frontend va bot birga yangilanadi:

```sh
docker compose up -d --build backend frontend bot
```

Proksi `/miniapp/api/*` da, ya'ni `/api/*` dan tashqarida: reverse-proxy
`/api/*` ni Go backendga, qolganini Next.js'ga yuborsa, Mini App uchun
qo'shimcha marshrut qoidasi KERAK EMAS. Bu ataylab shunday — proksi yo'lini
`/api/` ostiga qo'yish xostdagi konfigni qo'lda yangilashni talab qilardi va
bir marta aynan shu sababdan prod'da Mini App ishlamay qolgan edi.

Telegram **Web** (web.telegram.org) Mini App'ni iframe'da ochadi, shuning
uchun u yerda ishlashi uchun xostdagi Caddy `/miniapp/*` ga
`X-Frame-Options` qo'shmasligi kerak — Next.js `frame-ancestors` bilan faqat
Telegram originlariga ruxsat beradi. Telegram'ning telefon ilovasi va
Desktop versiyasi webview ishlatadi, ularga bu ta'sir qilmaydi. Caddyfile
xostda turadi, ya'ni deploy uni yangilamaydi:

```sh
sudo cp deploy/Caddyfile /etc/caddy/Caddyfile
sudo caddy validate --config /etc/caddy/Caddyfile
sudo systemctl reload caddy
```

## Lokal HTTPS testi

API 8080 va Next dev 3000 portida ishlab turganda Web katalogidan:

```sh
node scripts/miniapp-local-proxy.mjs
cloudflared tunnel --url http://127.0.0.1:3108 --no-autoupdate --protocol http2
```

Tunnel bergan `https://...trycloudflare.com` manzilga `/miniapp/post` qo'shib,
**lokal test botining** `TELEGRAM_MINIAPP_URL` qiymatiga yozing va botni
qayta ishga tushiring. 3108 proksi faqat Mini App va uning kerakli fayllarini
uzatadi; butun saytni yoki 8080 portini internetga ochmang.

Telegram'da botga `/start` yuboring va **E'lon berish**ni bosing. Oddiy
brauzerda imzolangan Telegram sessiyasi bo'lmasa ochish yo'riqnomasi chiqadi.
Kompyuter, frontend, API, proksi va tunnel ishlashi kerak. Quick tunnel
qayta ochilganda manzili o'zgaradi; bot sozlamasini yangilash lozim.

## Tekshirish

```sh
node --test scripts/miniapp-local-proxy.test.mjs
cd apps/bots/otp
go test ./...
cd ../../api
go test ./internal/auth ./internal/elon ./internal/channelpost
cd ../web
npx tsc --noEmit --incremental false
```

Testlar Telegram imzosi, eskirgan yoki o'zgartirilgan ma'lumot, o'zga kontakt,
rozilik, mavjud hisob, bloklangan hisob, Mini App tugmalari, eski qoralamani
chop etmaslik, ish qidirish/arizalar, rasmlar albomi va kanal navbatini qamraydi.
Mongo integratsiya testlari alohida test bazalarini yaratib tozalaydi.
