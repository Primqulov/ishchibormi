package notification

import (
	"strings"

	"github.com/ishchibormi/backend/internal/models"
	"github.com/ishchibormi/backend/pkg/tgsend"
)

// Use the saved event text: looking up the application's current state could
// describe a later decision when a queued notification is retried.
func telegramMessage(n models.Notification) string {
	if n.Type == "application_accepted" && n.AcceptedJob != nil {
		return telegramAcceptedMessage(n)
	}
	emoji, explanation, next := "", "", ""
	details := strings.TrimSpace(n.Body)
	switch n.Type {
	case "new_application":
		emoji = "📩 "
		details = "Ish: " + strings.TrimPrefix(details, "Sizning e'loningizga ariza tushdi: ")
		explanation = "E'loningizga yangi nomzod ariza yubordi."
		next = "Nomzod bilan tanishish uchun quyidagi tugmani bosing. Qarorni shu yerdan ham qabul qilishingiz mumkin: ✅ yoki ❌ tugmasini bossangiz, bot tasdiq so'raydi."
	case "job_nearby":
		emoji = "📍 "
		details = "Ish: " + details
		explanation = "Siz belgilagan hududda yangi ish e'loni chiqdi."
		next = "E'lonni ochib, ariza yuborishingiz mumkin. Kunlik ishlar tez to'ladi. Signalni o'chirish: botdagi /alerts."
	case "application_submitted":
		emoji = "📨 "
		details = "Ish: " + strings.TrimPrefix(details, "Ish beruvchining javobini kuting: ")
		explanation = "Arizangiz ish beruvchiga yuborildi. Endi uning javobini kuting."
		next = "Arizangiz holatini quyidagi tugma orqali ko'rishingiz mumkin."
	case "application_accepted":
		emoji = "✅ "
		details = "Ish: " + details
		explanation = "Ish beruvchi arizangizni qabul qildi. Siz ushbu ishga tanlandingiz!"
		next = "Ish boshlanishidan oldin sana, vaqt va manzilni tekshirib, ish beruvchi bilan kelishib oling. Tafsilotlar quyidagi tugmada."
	case "application_rejected":
		emoji = "ℹ️ "
		explanation = "Ish beruvchi ushbu ishga yuborgan arizangizni qabul qilmadi."
		if n.Title == "Joy to'ldi" {
			emoji = "👥 "
			details = strings.TrimSuffix(details, " — ish o'rinlari to'ldi, arizangiz qabul qilinmadi")
			explanation = "Ushbu ishdagi barcha o'rinlar band bo'ldi. Shu sababli arizangiz qabul qilinmadi."
		}
		details = "Ish: " + details
		next = "Boshqa mos ish e'lonlariga ariza yuborishingiz mumkin. Ushbu arizaning tafsilotlari quyidagi tugmada."
	case "application_cancelled":
		emoji = "ℹ️ "
		explanation = "Ushbu ish bo'yicha ariza bekor qilindi."
		next = "Bekor qilish tafsilotlarini quyidagi tugma orqali ko'rishingiz mumkin."
		switch {
		case n.Title == "Arizalaringiz bekor qilindi":
			// This event covers several applications, so it has no single job.
			details = ""
			explanation = "Siz bir ishga qabul qilinganingiz uchun, shu kundagi boshqa kutilayotgan arizalaringiz avtomatik bekor qilindi."
			next = "Qabul qilingan arizangiz o'z kuchida qoladi. Tafsilotlarni quyidagi tugma orqali ko'rishingiz mumkin."
		case n.Title == "Ish bekor qilindi":
			explanation = "Ish beruvchi ishni bekor qildi. Ushbu ishga yuborgan arizangiz ham bekor qilindi."
			next = "Boshqa mos ish e'lonlariga ariza yuborishingiz mumkin. Bekor qilish tafsilotlari quyidagi tugmada."
		case strings.HasSuffix(details, " — ishchi shu kunga boshqa ishga qabul qilindi"):
			details = strings.TrimSuffix(details, " — ishchi shu kunga boshqa ishga qabul qilindi")
			explanation = "Nomzod shu kunga boshqa ishga qabul qilingani uchun, sizning e'loningizga yuborgan arizasi avtomatik bekor qilindi."
			next = "Boshqa nomzodlarning arizalarini ko'rib chiqishingiz mumkin. Tafsilotlar quyidagi tugmada."
		}
		if details != "" {
			// Preserve the complete saved reason, including further separators.
			details = "Ish: " + strings.Replace(details, " — sabab: ", "\n\nSabab: ", 1)
		}
	case "elon_updated":
		emoji = "📝 "
		details = "Ish: " + strings.TrimSuffix(details, " — ish haqi, ish turi, sana yoki manzil o'zgartirildi. Yangilangan e'lonni tekshiring.")
		explanation = "Ish beruvchi siz ariza yuborgan e'londagi shartlarni yangiladi."
		next = "Quyidagi tugma orqali yangilangan e'lonni oching. Ish haqi, ish turi, sana va manzilni qayta tekshirib oling."
	case "job_completed_request":
		emoji = "⏳ "
		details = "Ish: " + strings.TrimPrefix(details, "Ish yakunlanganini tasdiqlang: ")
		explanation = "Ikkinchi tomon ish tugaganini bildirdi. Ish haqiqatan yakunlangan bo'lsa, siz ham tasdiqlang."
		next = "Quyidagi tugma orqali arizani oching. So'ng «Arizalarni ko'rish» bo'limida ish yakunlanganini tasdiqlashingiz mumkin."
	case "job_completed":
		emoji = "🏁 "
		explanation = "Ushbu ish yakunlandi va bajarilgan ishlar tarixiga o'tkazildi."
		if strings.HasSuffix(details, " — avtomatik yakunlandi va ish tarixiga o'tkazildi") {
			details = strings.TrimSuffix(details, " — avtomatik yakunlandi va ish tarixiga o'tkazildi")
			explanation = "Ushbu ish tizim tomonidan avtomatik yakunlandi va bajarilgan ishlar tarixiga o'tkazildi."
		}
		details = "Ish: " + details
		next = "Ish tafsilotlarini quyidagi tugma orqali ko'rishingiz mumkin. Hamkorligingiz uchun rahmat!"
	}

	// Bound dynamic text before HTML escaping. The remaining space is reserved
	// for the explanation and action, so even a long reason retains both.
	parts := []string{emoji + "<b>" + tgsend.EscapeHTML(telegramText(n.Title, 256)) + "</b>"}
	if strings.TrimSpace(n.Body) != "" && details != "" {
		parts = append(parts, tgsend.EscapeHTML(telegramText(details, 3000)))
	}
	for _, text := range []string{explanation, next} {
		if text != "" {
			parts = append(parts, tgsend.EscapeHTML(text))
		}
	}
	return strings.Join(parts, "\n\n")
}
