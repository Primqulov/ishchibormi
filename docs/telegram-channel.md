# Yangi ish e'lonlarini Telegram kanallariga chiqarish

Web, iOS, Android yoki Telegram bot orqali muvaffaqiyatli joylangan har bir
yangi ish, bot **administrator bo'lgan barcha kanallarga** avtomatik
yuboriladi. Bu e'lon foydalanuvchilarning shaxsiy bot chatlariga
tarqatilmaydi. Ishga ariza berish, qabul qilish va bekor qilishga oid mavjud
shaxsiy bildirishnomalar bundan mustaqil ishlaydi.

Kanal posti botdagi suhbatdan forward qilinmaydi. Alohida `sendMessage`
orqali kanal posti yaratiladi; ovozli bildirishnoma va havola preview si
o'chirilgan.

Koordinatasi bor e'lon **ikkita xabar** bo'lib chiqadi: avval Telegramning
o'z xarita kartasi (`sendLocation`), so'ng uning ostida to'liq matn va tugma.
Kartaga bosilganda Telegram O'ZINING xaritasini ochadi — brauzer yoki Google
Maps'ga chiqib ketmaydi, foydalanuvchi xohlasa o'zi tashqi xaritaga o'tadi.
Oddiy havola tugmasi buni qila olmaydi, karta esa caption qabul qilmaydi —
shuning uchun tafsilotlar alohida xabarda.

Karta ATAYLAB yalang'och — `sendVenue` emas. Venue sarlavha va manzilni
MAJBURIY talab qiladi va ularni kartaning ostiga yozib qo'yadi; natijada ish
nomi bilan hudud ikki marta — kartada ham, ostidagi matnda ham — ko'rinardi.

Karta matndan OLDIN yuboriladi va uning message ID si darhol saqlanadi
(`channel_posts.mapMessageId`). Shunda matn yuborishda xato bo'lsa, qayta
urinish kartani ikkinchi marta chiqarmaydi. Karta yuborilmasa matn ham
ketmaydi: yarim post kartasiz tafsilot yoki tafsilotsiz karta bo'lib
qolardi.

Koordinatasi yo'q yoki buzuq e'lon (eski yozuv) faqat matn posti bo'ladi.

Aloqa telefoni, to'liq tavsif, aniq manzil matni va ish beruvchi haqidagi
ma'lumot kanalga CHIQMAYDI — ularni ko'rish uchun odam botga o'tadi.

## Kanalni ulash

1. O'z kanalingizga ishlatilayotgan botni **administrator** qiling.
2. Unga **Post Messages / Xabar joylash** huquqini bering.

Tamom. Bot ulanganini kanalga bitta xabar bilan tasdiqlaydi va o'sha
paytdan boshlab **yangi** e'lonlar shu kanalga chiqadi. Hech qanday sozlama,
so'rov yoki tasdiqlash kerak emas — botni kim o'z kanaliga qo'shsa, o'sha
kanal e'lonlarni oladi.

**To'xtatish:** botni kanal administratorlaridan chiqaring. Bot buni darhol
qayd etadi va navbatdagi eski postlar ham yuborilmaydi.

Huquqsiz administrator e'lon yubora olmaydi, shuning uchun "Post Messages"
berilmagan kanal faol hisoblanmaydi. Guruh va superguruhlar ataylab qabul
qilinmaydi: e'lonlar oqimi suhbatni bosib ketardi.

## Kanal posti namunasi

> ┌──────────────────────────┐
> │   XARITA (Telegram)   📍 │
> └──────────────────────────┘
>
> 💼 **Yuk tushirish uchun ishchilar kerak**
>
> 👥 Kerak: **4 kishi**
>
> 💰 Ish haqi: **250 000 so'm / kishi**
>
> 📅 18.09.2026 · 09:00 (Toshkent)
>
> 📍 Toshkent, Chilonzor
>
> Ish haqida batafsil ma'lumot olish uchun pastdagi tugmani bosing.
>
> **Ish haqida batafsil**

Tugma `https://t.me/<bot_username>?start=job_<elon_id>` manzilini ochadi.
Kartaning o'ziga bosilsa Telegramning xaritasi ochiladi.
Telegram botni birinchi marta ochayotgan foydalanuvchidan **Start / Boshlash**ni
bosishni talab qilishi mumkin. Shundan keyin bot shu e'lonni darhol ochadi;
qidiruv yoki kirish kodi talab qilinmaydi.

Havola bot nomiga quriladi, shuning uchun nom ishga tushishda BIR MARTA
`getMe` bilan tekshiriladi: `TELEGRAM_BOT_USERNAME` token egasiga mos
kelmasa, kanalga chiqarish umuman yoqilmaydi (noto'g'ri nom butun postni
foydasiz qilardi). Har xabar oldidan tekshirish Telegram chegarasini
bekorga yeb qo'yardi.

## Bot ichida

E'lon har safar API'dan qayta olinadi. Joriy sarlavha, haq, sana va vaqt,
kerakli/qabul qilingan ishchilar soni, qolgan o'rinlar, ish beruvchi telefoni,
tavsif va Telegram xaritasi ko'rsatiladi. **Joylarni yangilash** tugmasi
so'nggi holatni ochadi. **Ariza berish** mavjud ariza oqimiga o'tkazadi:
o'z kontaktini tasdiqlash, profil, kishi soni va alohida yuborish tasdig'i.
Havola o'zi ariza yubormaydi.

Rasmli e'londa bitta rasm `sendPhoto`, 2–6 ta rasm esa bitta `sendMediaGroup`
albomi orqali yuboriladi. Ish tafsilotlari albomning birinchi rasmiga caption
sifatida biriktiriladi va butun albom ostida bir marta chiqadi; rasmlar ham,
shu tafsilotlar ham alohida-alohida xabar qilib takrorlanmaydi. Xarita va amal
tugmalari albomdan keyin keladi. Rasmsiz e'lon odatdagi matn bilan ochiladi.

Telegram caption chegarasi 1024 UTF-16 birlik. Uzoq tavsifda asosiy ma'lumot,
joriy bo'sh o'rinlar va telefon caption'da qoladi; **To'liq tavsif** tugmasi
matnni botning o'zida to'liq ochadi. Bu tugma ham ayni e'lonni API'dan qayta
tekshiradi. Rasmni yuklashda xato bo'lsa qisman albom yuborilmaydi: foydalanuvchi
ish tafsilotlarini va rasmlarni qayta yuklash ko'rsatmasini oladi.

Bot rasmlarni sozlangan storage'dan olib Telegram'ga fayl sifatida yuklaydi:
lokal URL'larni Telegram serveri ochishi talab qilinmaydi. `BOT_API_BASE_URL`
ostidagi `/uploads`, `UPLOAD_PUBLIC_BASE` va `AWS_S3_PUBLIC_BASE_URL` (yoki
bucket/region asosidagi S3 URL) qo'llanadi. Faqat shu manbalardagi `elons/`
rasmlari olinadi, redirectlar kuzatilmaydi, har bir fayl 8 MB bilan cheklanadi.
JPEG, PNG va WEBP Telegram uchun JPEG'ga aylantiriladi; katta o'lchamlar
kichraytiriladi, juda uzun/tor rasm qirqilmaydi.

To'lgan yoki muddati tugagan ishga ariza tugmasi chiqarilmaydi. O'chirilgan,
yashirilgan yoki ommaga yopilgan e'lon havolasi boshqa ish topish taklifini beradi.
Kanal posti e'lon yaratilgan paytdagi qisqa ma'lumot; joriy joylar botda ko'rinadi.

## Qanday ishlaydi

Uchta hujjat turi bor. Bitta e'lon endi N ta kanalga ketadi, ya'ni har
birining natijasi alohida saqlanishi kerak.

**`telegram_channels` — kanallar reyestri.** Uni **bot** yuritadi:
Telegram kanalga qo'shilish/chiqarilish haqidagi `my_chat_member` update'ini
faqat so'rab turgan jarayonga yuboradi. `_id` — kanalning Telegram chat ID si,
ya'ni qayta qo'shilganda ayni yozuv tiklanadi va ikkinchi hujjat paydo
bo'lmaydi. Holatlar: `active`, `inactive` (bot chiqarilgan yoki huquqi
olingan), `blocked` (operator qo'lda to'xtatgan).

**`elons.telegramBroadcast` — e'lon hujjatidagi belgi.** E'lon bilan ayni
`InsertOne` ichida yoziladi, ya'ni «e'lon yaratildi, lekin navbatga
qo'yilmadi» holati bo'lmaydi. Kanallar ro'yxati bu yerda O'QILMAYDI —
shuning uchun e'lon yaratish tezligi kanallar soniga bog'liq emas.

**`channel_posts` — har (e'lon, kanal) juftligi uchun bitta navbat yozuvi.**
`(elonId, chatId)` bo'yicha **unikal** indeks bor: takroriy postning oldini
olish aynan shunga tayanadi.

Fan-out worker belgisi `pending` bo'lgan e'lonni oladi, o'sha paytdagi faol
kanallarni o'qiydi va har biriga navbat yozuvi yaratadi. **Keyin qo'shilgan
kanal eski e'lonlarni olmaydi** — bu ataylab: yangi kanalga yuzlab eski
e'lon to'kilib ketmasligi kerak. Fan-out faqat idempotent yozuv qiladi,
shuning uchun uzilib qolgan urinish xavfsiz qaytariladi.

Yetkazish worker'i navbatdan sekundiga 5 tagacha post yuboradi. Telegram
umumiy chegarasi ~30 xabar/sekund, ya'ni zaxira bilan pastda turamiz.

## Yetkazish va qayta urinish

Web, iOS, Android va bot bir xil backend e'lon yaratish jarayonidan foydalanadi.
Botning so'rovida qo'shimcha HMAC imzo ham tekshiriladi, lekin kanalga yuborish
bu imzoga bog'liq emas. Bir xil `Idempotency-Key` bilan takroriy yaratish
so'rovi ayni e'lonni qaytaradi va kanallarga qaytadan navbatga qo'ymaydi.

Yuborishdan oldin e'lonning faol holati, bo'sh o'rin va kanalning hali
faolligi tekshiriladi. Foydalanuvchilar ro'yxati o'qilmaydi va yangi e'lon
uchun shaxsiy chatga xabar yuborilmaydi.

`sent` holatida Telegram message ID saqlanadi. Telegramning aniq 429
javobidan keyin belgilangan muddat kutib qayta urinish mumkin. `sendMessage`
uchun Telegram idempotency kaliti bermaydi: timeout yoki uzilishda xabar
yetib borgan-bormagani noma'lum bo'lsa, takroriy post chiqarmaslik uchun
`uncertain` holatiga o'tiladi. Operator `channel_posts` va backend
jurnalidan e'lon ID si bo'yicha natijani tekshiradi; bunday xabar avtomatik
qayta yuborilmaydi. `failed` — aniq rad etilgan, `skipped` — yuborilguncha
ish yopilgan/to'lgan yoki kanal uzilgan.

Aniq rad javobi (4xx) kelgan kanal darhol `inactive` qilinadi: bot
chiqarilgan yoki kanal o'chirilgan bo'lsa, har yangi e'lon shu xatoni
qaytarib navbatni bekorga band qilib turardi.

## Suiiste'mol va nazorat

Botni istagan odam o'z kanaliga qo'sha oladi — bu ataylab shunday, chunki
e'lonlar ommaviy ma'lumot va har bir kanal qo'shimcha tarqatish kanali.
Kanal faqat e'lonning qisqa ma'lumotini oladi: telefon, to'liq tavsif va
aniq manzil kanalga chiqmaydi.

Muayyan kanalni butunlay uzish kerak bo'lsa, `telegram_channels` dagi
yozuvga `status: "blocked"` qo'ying. Kod bu holatni hech qachon o'zgartirmaydi:
bot o'sha kanalga qayta qo'shilsa ham yozuv `blocked` bo'lib qoladi.

## Eski sozlama

`TELEGRAM_JOBS_CHANNEL_ID` endi ixtiyoriy va faqat bitta holat uchun kerak:
bot **allaqachon a'zo** bo'lgan kanal. Telegram mavjud a'zolik uchun
`my_chat_member` yubormaydi, ya'ni bunday kanal o'z-o'zidan reyestrga
tushmaydi. Kalit qo'yilgan bo'lsa API ishga tushganda uni bir marta qayd
etadi. Yangi kanallar uchun bu kalit kerak emas.

App Store / Google Play tekshiruvi uchun yaratilgan demo hisob e'lonlari
kanallarga chiqarilmaydi. E'lonni tahrirlash yangi kanal posti yaratmaydi.

Local test bot: `@ishchibormiauthtestbot`.

## Tekshirish

```sh
cd apps/api
go test ./internal/channelpost ./internal/elon ./internal/auth
cd ../bots/otp
go test ./internal/channels ./cmd/bot
```

Testlar bir nechta kanalga fan-out, takroriy postning oldini olish, faqat
faol kanallarga yuborish, yangi kanalga eski e'lonlarni yubormaslik,
noaniq yetkazishni qayta urinmaslik, rad etgan kanalni uzish, bloklangan
kanalni tiklamaslik va faqat kanal a'zoligini qabul qilishni qamraydi.
Mongo integratsiya testlari alohida test bazalarini yaratib tozalaydi.
