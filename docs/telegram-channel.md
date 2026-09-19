# Yangi ish e'lonlarini Telegram kanaliga chiqarish

Web, iOS, Android yoki Telegram bot orqali muvaffaqiyatli joylangan har bir
yangi ish avtomatik ravishda bitta sozlangan kanalga yuboriladi. Bu e'lon foydalanuvchilarning shaxsiy bot
chatlariga tarqatilmaydi. Ishga ariza berish, qabul qilish va bekor qilishga
oid mavjud shaxsiy bildirishnomalar bundan mustaqil ishlaydi.

Kanal posti botdagi suhbatdan forward qilinmaydi. Alohida `sendMessage` orqali
kanal posti yaratiladi; ovozli bildirishnoma va havola preview si o'chirilgan.
Kanalda to'liq tavsif, aloqa telefoni va aniq xarita chiqarilmaydi.

## Kanal posti namunasi

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
Telegram botni birinchi marta ochayotgan foydalanuvchidan **Start / Boshlash**ni
bosishni talab qilishi mumkin. Shundan keyin bot shu e'lonni darhol ochadi;
qidiruv yoki kirish kodi talab qilinmaydi.

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

## Ulash

1. E'lonlar chiqadigan kanalga ishlatilayotgan botni administrator qiling.
2. Botga **Post Messages / Xabar joylash** huquqini bering.
3. Backend muhitida quyidagini sozlang:

   ```dotenv
   TELEGRAM_JOBS_CHANNEL_ID=@kanal_username
   ```

   Yopiq kanal uchun `-100…` ID ishlatiladi. `TELEGRAM_BOT_TOKEN` va
   `TELEGRAM_BOT_USERNAME` aynan bir botga tegishli bo'lishi kerak.
4. Backendni qayta ishga tushiring. Keyin web, iOS, Android yoki botdan yangi e'lon bering.

Sozlama bo'sh bo'lsa kanalga yuborish o'chiq. Avvalgi e'lonlar orqadan
tarqatilmaydi. Kanal sozlangandan keyin `POST /api/elons` orqali yaratilgan
yangi e'lonlar manbasidan qat'i nazar navbatga olinadi. Alohida platforma
headeri yoki bot imzosi kanalga yuborish uchun talab qilinmaydi; barcha
so'rovlarda foydalanuvchi autentifikatsiyasi va e'lon tekshiruvlari saqlanadi.
App Store / Google Play tekshiruvi uchun yaratilgan demo hisob e'lonlari
kanalga chiqarilmaydi. E'lonni tahrirlash yangi kanal posti yaratmaydi.

Local test bot: `@ishchibormiauthtestbot`. Local ishga tushirish skriptlari
`%TEMP%/ishchibormi-telegram-local/.env.telegram-test` faylini asosiy `.env`
ustidan yuklaydi; test kanal sozlamasi shu override faylida saqlanishi mumkin.

## Yetkazish va qayta urinish

Web, iOS, Android va bot bir xil backend e'lon yaratish jarayonidan foydalanadi.
Botning so'rovida qo'shimcha HMAC imzo ham tekshiriladi, lekin kanalga yuborish
bu imzoga bog'liq emas. Kanal navbati e'lonning o'z Mongo hujjatida
`telegramChannel` sifatida atomar saqlanadi; oddiy foydalanuvchi bu ichki
maydonni JSON orqali belgilay olmaydi. Bir xil `Idempotency-Key` bilan takroriy
yaratish so'rovi ayni e'lonni qaytaradi va kanalga qaytadan navbatga qo'ymaydi.

Yuborishdan oldin e'lonning faol holati, bo'sh o'rin, kanal turi, botning haqiqiy
username'i va kanalga yozish huquqi tekshiriladi. Foydalanuvchilar ro'yxati
o'qilmaydi va yangi e'lon uchun shaxsiy chatga xabar yuborilmaydi.

`sent` holatida kanal ID va Telegram message ID saqlanadi. Telegramning aniq
429 javobidan keyin belgilangan muddat kutib qayta urinish mumkin. `sendMessage`
uchun Telegram idempotency kaliti bermaydi: timeout yoki uzilishda xabar yetib
borgan-bormagani noma'lum bo'lsa, takroriy post chiqarmaslik uchun `uncertain`
holatiga o'tiladi. Operator `elons.telegramChannel` va backend jurnalidan e'lon
ID si bo'yicha natijani tekshiradi; bunday xabar avtomatik qayta yuborilmaydi.
`failed` — aniq rad etilgan, `skipped` — yuborilguncha ish yopilgan/to'lgan.
Kanal manzili almashtirilsa eski navbat yangi kanalga yo'naltirilmaydi.
