package posting

import (
	"context"
	"strings"
	"time"

	tg "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

// Doimiy pastki menyu.
//
// NEGA KERAK: ilgari bot deyarli har xabardan keyin klaviaturani olib
// tashlardi, ya'ni foydalanuvchi /jobs, /applications kabi buyruqlarni yodda
// tutishi kerak edi. Bu tugmalar chatda turib qoladi va hech narsani eslab
// qolish shart emas. Tugma bosilganda oddiy matn keladi — cmd/bot/main.go uni
// KeyboardCommand orqali buyruqqa aylantiradi, shuning uchun qolgan mantiq
// o'zgarishsiz qoladi.
// Android ilovasining Play Store manzili. Yagona manba —
// apps/web/lib/contact.ts (APP_PACKAGE); paket nomi o'zgarsa ikkalasi ham
// yangilanishi kerak.
const androidAppURL = "https://play.google.com/store/apps/details?id=uz.ishchibormi.app"

const (
	btnFindJobs = "📍 Ish topish"
	btnMyApps   = "📋 Arizalarim"
	btnPostJob  = "➕ E'lon berish"
	btnRegister = "📝 Ro'yxatdan o'tish"
	btnHelp     = "ℹ️ Yordam"
	btnOffer    = "💡 Taklif"
)

func MainKeyboard() tg.ReplyKeyboardMarkup {
	kb := tg.NewReplyKeyboard(
		tg.NewKeyboardButtonRow(tg.NewKeyboardButton(btnFindJobs), tg.NewKeyboardButton(btnMyApps)),
		tg.NewKeyboardButtonRow(tg.NewKeyboardButton(btnPostJob), tg.NewKeyboardButton(btnHelp)),
		// Taklif ATAYLAB alohida, eni to'liq qatorda: u boshqalaridan farqli
		// o'laroq oqim boshlamaydi, balki loyihaning qolgan yo'llarini
		// (ilova, sayt, ikkita kanal) ko'rsatadi — ko'zga tashlanib tursin.
		tg.NewKeyboardButtonRow(tg.NewKeyboardButton(btnOffer)),
	)
	kb.ResizeKeyboard = true
	return kb
}

// KeyboardCommand pastki menyu tugmasining matnini buyruq nomiga o'giradi.
// Tanilmagan matn uchun bo'sh satr — bunday xabar odatdagidek ishlanadi.
func KeyboardCommand(text string) string {
	switch strings.TrimSpace(text) {
	case btnFindJobs:
		return "jobs"
	case btnMyApps:
		return "applications"
	case btnPostJob:
		return "post"
	case btnRegister:
		// Tugma endi klaviaturada YO'Q: ro'yxatdan o'tish taklifi faqat hisobi
		// bo'lmaganlarga chiqadi (worker.go: offerRegistration). Moslik uchun
		// qoldirilgan — Telegram eski klaviaturani yangi xabargacha ko'rsatib
		// turadi, ya'ni eski tugma hali bosilishi mumkin.
		return "register"
	case btnHelp:
		return "help"
	case btnOffer:
		return "taklif"
	}
	return ""
}

// Sayt tokeni shuncha vaqt kutiladi. OTP kodining o'zi qisqaroq yashaydi;
// bu — «/start bosdim, keyin chalg'idim» holati uchun yuqori chegara.
const authTokenTTL = 30 * time.Minute

// SaveAuthToken saytdan kelgan sessiya tokenini draft'ga yozadi.
//
// Token ilgari faqat bot xotirasida (map) turardi: /start bilan kontakt
// ulashish orasida bot qayta ishga tushsa, kod tokensiz yozilar va
// foydalanuvchi saytda «kod noto'g'ri» xabarini olardi. Mongo'dagi draft
// restartdan omon qoladi.
func (e *Engine) SaveAuthToken(ctx context.Context, chatID int64, token string) error {
	if token == "" {
		return nil
	}
	d, err := e.Store.Load(ctx, chatID)
	if err != nil {
		return err
	}
	if d == nil {
		d = freshDraft(chatID)
		d.Step = "menu"
	}
	d.AuthToken, d.AuthTokenAt = token, e.now()
	return e.Store.Save(ctx, d)
}

// TakeAuthToken tokenni qaytaradi va darhol o'chiradi: bitta token faqat
// bitta kod uchun. Muddati o'tgan token ham o'chiriladi va bo'sh qaytadi.
func (e *Engine) TakeAuthToken(ctx context.Context, chatID int64) string {
	d, err := e.Store.Load(ctx, chatID)
	if err != nil || d == nil || d.AuthToken == "" {
		return ""
	}
	token := d.AuthToken
	fresh := d.AuthTokenAt.Add(authTokenTTL).After(e.now())
	d.AuthToken, d.AuthTokenAt = "", time.Time{}
	if err := e.Store.Save(ctx, d); err != nil {
		return ""
	}
	if !fresh {
		return ""
	}
	return token
}
