package posting

import (
	"strings"

	tg "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

// Rasmiy Telegram kanallari.
//
// news — loyiha yangiliklari kanali (qo'lda yuritiladi).
// jobs — har bir yangi ish e'loni AVTOMATIK tushadigan kanal; unga
// apps/api/internal/channelpost yozadi va har bir postga
// «t.me/<bot>?start=job_<id>» havolasi qo'shiladi, ya'ni kanaldan kelgan odam
// aynan o'sha e'lonni ochadi (internal/posting/channel_link_test.go).
const (
	newsChannelURL = "https://t.me/Ishchibormi"
	jobsChannelURL = "https://t.me/Ishchibormi_elonlar"
)

// Taklif bo'limi matni.
//
// NEGA KERAK: bot bilan tanishgan odam loyihaning qolgan uch yo'lini —
// ilova, sayt va ikkita kanalni — umuman bilmay qolardi. Ayniqsa
// @Ishchibormi_elonlar: unga obuna bo'lgan odam ishni o'zi qidirmasa ham
// yangi e'lonni ko'radi, ya'ni botga qaytish ehtimoli ancha yuqori.
//
// Matn ATAYLAB «reklama» emas, foyda tilida yozilgan: har bir band
// «bu sizga nima beradi» degan savolga javob beradi.
const offerText = `💡 Ishchi Bormi — faqat shu bot emas

Ish izlash ham, ishchi topish ham qulay bo'lsin deb biz bir necha joydamiz. Qaysi biri sizga qulay bo'lsa — o'shanisidan foydalaning. Hisobingiz hammasida BITTA: bot, ilova va saytga o'sha bir raqam bilan kirasiz.

📱 Android ilova — eng tezkor yo'l
Ish endi cho'ntagingizda. Yaqiningizdagi e'lonlar xaritada ko'rinadi, ariza ikki bosishda yuboriladi, yangi ish chiqsa telefoningizning o'zi xabar beradi. Play Store'dan mutlaqo bepul.

🌐 ishchibormi.uz — katta ekranning qulayligi
Barcha e'lonlar, qidiruv filtrlari, o'z e'lonlaringiz va arizalar tarixi bitta sahifada. Ish beruvchilar uchun ayniqsa qulay: e'lon berish ham, nomzodlarni ko'rish ham bir necha soniya.

📢 @Ishchibormi — yangiliklar kanali
Loyihaning yuragi shu yerda uradi: yangi imkoniyatlar, muhim e'lonlar, foydali maslahatlar va yangilanishlardan birinchi bo'lib xabardor bo'lasiz.

🔔 @Ishchibormi_elonlar — yangi ish e'lonlari kanali
Botga, saytga yoki ilovaga joylangan HAR BIR yangi ish e'loni bu kanalga o'zi tushadi — kechikmasdan, tanlamasdan. Obuna bo'ling-u, ishni qidirmang: ish o'zi sizga kelsin. E'lon ostidagi havola sizni to'g'ri o'sha ishga olib kiradi, ariza esa shu yerdan yuboriladi.

Yoqqan bo'lsa, do'stlaringizga ham ulashing — kimdir aynan bugun ish topib ketishi mumkin.`

// showOffer taklif bo'limini yuboradi.
//
// Havolalar tugma sifatida beriladi: matndagi @nom ni qidirib o'tirishdan
// ko'ra bir bosish qulayroq, qolaversa sayt manzili WebURL dan olinadi —
// test/stend muhitida ham to'g'ri joyga olib boradi.
func (e *Engine) showOffer(chat int64) error {
	return e.workerMessage(chat, offerText,
		tg.NewInlineKeyboardRow(
			tg.NewInlineKeyboardButtonURL("📱 Android ilova", androidAppURL),
			tg.NewInlineKeyboardButtonURL("🌐 Sayt", strings.TrimRight(e.WebURL, "/"))),
		tg.NewInlineKeyboardRow(tg.NewInlineKeyboardButtonURL("📢 Yangiliklar kanali", newsChannelURL)),
		tg.NewInlineKeyboardRow(tg.NewInlineKeyboardButtonURL("🔔 Ish e'lonlari kanali", jobsChannelURL)),
		tg.NewInlineKeyboardRow(tg.NewInlineKeyboardButtonData("📍 Ish topish", "jobs:start")),
		tg.NewInlineKeyboardRow(workerButton("⬅️ Bosh menyu", "home", "")))
}
