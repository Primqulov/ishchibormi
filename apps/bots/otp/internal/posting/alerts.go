package posting

import (
	"context"
	"fmt"
	"strings"

	tg "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

// Standart radius. Backend ham shu qiymatni standart deb biladi
// (internal/jobalert.DefaultRadiusM); bu yerda takrorlangan, chunki bot uni
// foydalanuvchiga matn sifatida ko'rsatadi.
const alertRadiusM = 10000

// handleAlert — «ish signali»ni yoqish, o'chirish va holatini ko'rsatish.
//
// Hudud alohida so'ralmaydi: qidiruvda ishlatilgan joy (yoki oxirgi eslab
// qolingan joy) olinadi. Ya'ni foydalanuvchi uchun bu bitta tugma — «shu
// yerdagi yangi ishlar haqida xabar ber».
func (e *Engine) handleAlert(ctx context.Context, d *Draft, s Session, mode string) error {
	switch mode {
	case "off":
		if err := e.API.DeleteJobAlert(ctx, s); err != nil {
			return e.say(d.ChatID, workerError(err))
		}
		return e.workerMessage(d.ChatID,
			"🔕 Ish signali o'chirildi. Yangi e'lonlar haqida xabar kelmaydi.\n\nQayta yoqish: hudud bo'yicha qidiruvdan keyin «🔔 Signal yoqish».",
			tg.NewInlineKeyboardRow(tg.NewInlineKeyboardButtonData("📍 Ish qidirish", "jobs:start")),
			tg.NewInlineKeyboardRow(workerButton("⬅️ Bosh menyu", "home", "")))
	case "on":
		lat, lng, place, categoryID, ok := e.alertTarget(d)
		if !ok {
			return e.workerMessage(d.ChatID,
				"Signalni yoqish uchun avval hududni belgilang: «📍 Ish qidirish» → joylashuvni yuboring yoki viloyatni tanlang. Shundan keyin signal aynan shu hudud bo'yicha ishlaydi.",
				tg.NewInlineKeyboardRow(tg.NewInlineKeyboardButtonData("📍 Ish qidirish", "jobs:start")),
				tg.NewInlineKeyboardRow(workerButton("⬅️ Bosh menyu", "home", "")))
		}
		alert, err := e.API.SaveJobAlert(ctx, s, JobAlert{
			Lat: lat, Lng: lng, RadiusM: alertRadiusM, CategoryID: categoryID, PlaceName: place,
		})
		if err != nil {
			return e.say(d.ChatID, workerError(err))
		}
		return e.workerMessage(d.ChatID,
			"🔔 Ish signali yoqildi.\n\n"+alertSummary(alert)+"\n\nShu hududda yangi e'lon chiqqanda sizga xabar beraman. Kunlik ishlar tez to'ladi — birinchilardan bo'lib ariza yuborishingiz mumkin.",
			tg.NewInlineKeyboardRow(workerButton("🔕 Signalni o'chirish", "alert", "off")),
			tg.NewInlineKeyboardRow(tg.NewInlineKeyboardButtonData("📍 Hududni yangilash", "jobs:start")),
			tg.NewInlineKeyboardRow(workerButton("⬅️ Bosh menyu", "home", "")))
	default:
		alert, err := e.API.JobAlert(ctx, s)
		if err != nil {
			return e.say(d.ChatID, workerError(err))
		}
		if alert == nil {
			return e.workerMessage(d.ChatID,
				"🔕 Ish signali o'chiq.\n\nYoqsangiz, belgilagan hududingizda yangi ish e'loni chiqqanda darhol xabar beraman — har safar o'zingiz qidirib kelishingiz shart emas.",
				tg.NewInlineKeyboardRow(workerButton("🔔 Signalni yoqish", "alert", "on")),
				tg.NewInlineKeyboardRow(tg.NewInlineKeyboardButtonData("📍 Hududni tanlash", "jobs:start")),
				tg.NewInlineKeyboardRow(workerButton("⬅️ Bosh menyu", "home", "")))
		}
		return e.workerMessage(d.ChatID,
			"🔔 Ish signali yoqilgan.\n\n"+alertSummary(alert),
			tg.NewInlineKeyboardRow(workerButton("🔕 Signalni o'chirish", "alert", "off")),
			tg.NewInlineKeyboardRow(tg.NewInlineKeyboardButtonData("📍 Hududni yangilash", "jobs:start")),
			tg.NewInlineKeyboardRow(workerButton("⬅️ Bosh menyu", "home", "")))
	}
}

// alertTarget — signal uchun hudud va ish turi: oxirgi qidiruv nimani
// ko'rsatgan bo'lsa, signal ham aynan shuni kuzatadi.
//
// Manba ATAYLAB draft: worker oqimiga o'tilganda xotiradagi qidiruv sessiyasi
// tozalanadi (worker.go: StopJobSearch), ya'ni tugma bosilgan paytda u
// mavjud bo'lmaydi.
func (e *Engine) alertTarget(d *Draft) (lat, lng float64, place, categoryID string, ok bool) {
	if d == nil || !d.HasLastSearch {
		return 0, 0, "", "", false
	}
	return d.LastSearchLat, d.LastSearchLng, d.LastSearchPlace, d.LastSearchCategoryID, true
}

func alertSummary(a *JobAlert) string {
	if a == nil {
		return ""
	}
	place := strings.TrimSpace(a.PlaceName)
	if place == "" {
		place = fmt.Sprintf("xaritadagi nuqta (%.4f, %.4f)", a.Lat, a.Lng)
	}
	category := strings.TrimSpace(a.CategoryName)
	if category == "" {
		category = "barcha ish turlari"
	}
	return fmt.Sprintf("📍 Hudud: %s\n📏 Radius: %s\n🏷 Ish turi: %s", place, radiusText(a.RadiusM), category)
}

func radiusText(meters int) string {
	if meters <= 0 {
		meters = alertRadiusM
	}
	if meters < 1000 {
		return fmt.Sprintf("%d m", meters)
	}
	return fmt.Sprintf("%d km", meters/1000)
}
