package main

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/ishchibormi/bot/internal/channels"
	"github.com/ishchibormi/bot/internal/envfile"
	"github.com/ishchibormi/bot/internal/posting"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

type userDoc struct {
	Phone      string `bson:"phone"`
	TelegramID int64  `bson:"telegramId"`
}

func main() {
	envfile.Load()
	token := os.Getenv("TELEGRAM_BOT_TOKEN")
	if token == "" {
		log.Fatal("TELEGRAM_BOT_TOKEN required")
	}
	mongoURI := getenv("MONGO_URI", "mongodb://localhost:27017")
	dbName := getenv("MONGO_DB", "ishchibormi")
	otpTTL := time.Duration(envInt("OTP_TTL_SECONDS", 180)) * time.Second
	otpLen := envInt("OTP_LENGTH", 6)

	// SIGINT/SIGTERM (docker compose stop) kelganda ctx bekor bo'ladi — quyida
	// updates kanali yopilib, sikl tugaydi va Mongo ulanishi toza uziladi.
	// Aks holda konteyner 10s kutib SIGKILL bilan o'lardi, yozuvlar chala qolardi.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	mc, err := mongo.Connect(ctx, options.Client().ApplyURI(mongoURI))
	if err != nil {
		log.Fatalf("mongo: %v", err)
	}
	defer func() {
		// ctx bu paytda bekor bo'lgan bo'lishi mumkin — uzish uchun yangi muddat.
		dctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = mc.Disconnect(dctx)
	}()

	// mongo.Connect is lazy — it doesn't actually reach the server. Ping now so a
	// misconfigured/unreachable MONGO_URI fails loudly at startup instead of every
	// user silently getting "Xatolik yuz berdi" when they share their contact.
	pingCtx, cancelPing := context.WithTimeout(ctx, 10*time.Second)
	if err := mc.Ping(pingCtx, nil); err != nil {
		cancelPing()
		log.Fatalf("mongo unreachable (db=%q): %v — check MONGO_URI", dbName, err)
	}
	cancelPing()

	otpCol := mc.Database(dbName).Collection("otp_codes")
	usersCol := mc.Database(dbName).Collection("users")

	bot, err := tgbotapi.NewBotAPIWithClient(token, tgbotapi.APIEndpoint, &http.Client{Timeout: 40 * time.Second})
	if err != nil {
		log.Fatalf("bot: %v", err)
	}
	log.Printf("bot started: @%s", bot.Self.UserName)
	postingAPI, err := posting.NewClient(getenv("BOT_API_BASE_URL", "http://127.0.0.1:8080"), os.Getenv("BOT_SHARED_SECRET"))
	if err != nil {
		log.Fatalf("posting configuration: %v", err)
	}
	s3PhotoBase := os.Getenv("AWS_S3_PUBLIC_BASE_URL")
	if s3PhotoBase == "" && os.Getenv("AWS_S3_BUCKET") != "" {
		s3PhotoBase = fmt.Sprintf("https://%s.s3.%s.amazonaws.com", os.Getenv("AWS_S3_BUCKET"), getenv("AWS_REGION", "eu-central-1"))
	}
	photoLoader, err := posting.NewJobPhotoLoader(
		strings.TrimRight(getenv("BOT_API_BASE_URL", "http://127.0.0.1:8080"), "/")+"/uploads",
		os.Getenv("UPLOAD_PUBLIC_BASE"), s3PhotoBase,
	)
	if err != nil {
		log.Fatalf("listing photo configuration: %v", err)
	}
	miniAppURL := getenv("TELEGRAM_MINIAPP_URL", strings.TrimRight(getenv("WEB_BASE_URL", "https://ishchibormi.uz"), "/")+"/miniapp/post")
	if !posting.ValidMiniAppURL(miniAppURL) {
		log.Fatal("TELEGRAM_MINIAPP_URL must be an HTTPS URL")
	}
	webURL := getenv("WEB_BASE_URL", "https://ishchibormi.uz")
	poster := &posting.Engine{
		Store: posting.MongoStore{Col: mc.Database(dbName).Collection("telegram_posting_drafts")},
		API:   postingAPI, Bot: bot, WebURL: webURL,
		MiniAppURL: miniAppURL,
		LoadPhoto:  photoLoader,
		Searches:   posting.NewSearchSessions(),
	}
	go poster.Searches.Run(ctx)
	menu, _ := json.Marshal(map[string]any{"type": "web_app", "text": "E'lon berish", "web_app": map[string]string{"url": miniAppURL}})
	if _, err := bot.MakeRequest("setChatMenuButton", tgbotapi.Params{"menu_button": string(menu)}); err != nil {
		log.Print("Mini App menu button could not be set")
	}
	_, _ = bot.Request(tgbotapi.NewSetMyCommands(
		tgbotapi.BotCommand{Command: "start", Description: "Bosh menyu"},
		tgbotapi.BotCommand{Command: "register", Description: "Ro'yxatdan o'tish yoki profilni ko'rish"},
		tgbotapi.BotCommand{Command: "jobs", Description: "Joylashuv bo'yicha yaqin ishlar"},
		tgbotapi.BotCommand{Command: "applications", Description: "Arizalarim va ish tarixi"},
		tgbotapi.BotCommand{Command: "myjobs", Description: "Qabul qilingan ishlarim"},
		tgbotapi.BotCommand{Command: "candidates", Description: "E'lonlarimga kelgan arizalar"},
		tgbotapi.BotCommand{Command: "alerts", Description: "Ish signali: yangi e'lonlar haqida xabar"},
		tgbotapi.BotCommand{Command: "menu", Description: "Bosh menyu"},
		tgbotapi.BotCommand{Command: "help", Description: "Botdan foydalanish"},
		tgbotapi.BotCommand{Command: "post", Description: "Mini App orqali e'lon berish"},
		tgbotapi.BotCommand{Command: "cancel", Description: "Joriy suhbatni to'xtatish"},
	))
	handlePosting := func(u tgbotapi.Update) {
		if err := poster.Handle(ctx, u); err != nil {
			log.Print("posting update failed; draft can be resumed")
			if u.Message != nil && u.Message.Chat != nil && u.Message.Chat.Type == "private" {
				_, _ = bot.Send(tgbotapi.NewMessage(u.Message.Chat.ID, "Oxirgi amalni bajarib bo'lmadi. /menu orqali davom eting. Ariza holati: /applications. E'lon formasi: /post."))
			}
		}
	}

	// Kanal reyestri: bot qaysi kanallarda administrator ekanini shu yerda
	// yozib boradi, backend esa undan o'qib yangi e'lonlarni yuboradi
	// (apps/api/internal/channelpost).
	channelRegistry := channels.New(mc.Database(dbName).Collection("telegram_channels"))
	handleMembership := func(ctx context.Context, u tgbotapi.Update) {
		event, ok := channels.FromUpdate(u)
		if !ok {
			return
		}
		mctx, cancel := context.WithTimeout(ctx, 15*time.Second)
		defer cancel()
		active, err := channelRegistry.Apply(mctx, event)
		if err != nil {
			log.Printf("channel registry not updated (chat=%d)", event.ChatID)
			return
		}
		if !active {
			log.Printf("channel no longer receives listings (chat=%d)", event.ChatID)
			return
		}
		log.Printf("channel receives listings (chat=%d)", event.ChatID)
		// Tasdiq ayni kanalga yoziladi: kanal egasi ulanganini darhol
		// ko'radi va bu xabar joylash huquqi haqiqatan borligini isbotlaydi.
		hello := tgbotapi.NewMessage(event.ChatID, "✅ Ishchi Bormi ulandi.\n\nBundan keyin saytda, mobil ilovada yoki botda joylangan har bir yangi ish e'loni shu kanalga chiqadi.\n\nTo'xtatish uchun botni kanal administratorlaridan chiqaring.")
		hello.DisableWebPagePreview = true
		if _, err := bot.Send(hello); err != nil {
			log.Printf("channel greeting not delivered (chat=%d)", event.ChatID)
		}
	}

	upd := tgbotapi.NewUpdate(0)
	upd.Timeout = 30
	// ATAYLAB aniq ro'yxat. my_chat_member standart to'plamda bor, lekin
	// ro'yxat bir marta boshqacha o'rnatilgan bo'lsa (masalan webhook bilan)
	// u saqlanib qoladi va kanalga qo'shilish hodisasi kelmay qo'yardi.
	// Bot faqat shu uchtasini o'qiydi — qolgani bekorga trafik.
	upd.AllowedUpdates = []string{"message", "callback_query", "my_chat_member"}
	updates := bot.GetUpdatesChan(upd)
	go func() {
		<-ctx.Done()
		log.Println("shutdown signal — stopping updates")
		bot.StopReceivingUpdates() // updates kanalini yopadi, quyidagi for tugaydi
	}()
	handleUpdates := func(updates <-chan tgbotapi.Update) {
		// Each chat always runs on the same worker, including its OTP handshake.
		pending := map[int64]string{}
		for u := range updates {
			if u.CallbackQuery != nil {
				if u.CallbackQuery.From != nil && (strings.HasPrefix(u.CallbackQuery.Data, "p:") || strings.HasPrefix(u.CallbackQuery.Data, "jobs:") || strings.HasPrefix(u.CallbackQuery.Data, "w:")) {
					delete(pending, u.CallbackQuery.From.ID)
				}
				handlePosting(u)
				continue
			}
			if u.Message == nil {
				continue
			}
			m := u.Message
			if m.From == nil || m.Chat == nil || m.Chat.Type != "private" || m.From.ID != m.Chat.ID {
				continue
			}
			// Doimiy pastki menyu tugmalari oddiy matn yuboradi. Ularni shu
			// yerda buyruqqa aylantiramiz — quyidagi butun mantiq (va Engine)
			// o'zgarishsiz qoladi.
			if !m.IsCommand() {
				if cmd := posting.KeyboardCommand(m.Text); cmd != "" {
					asCommand(m, cmd)
				}
			}
			if isBotCommand(m) {
				delete(pending, m.Chat.ID)
				handlePosting(u)
				continue
			}
			if m.Contact != nil && pending[m.Chat.ID] == "" {
				handlePosting(u)
				continue
			}
			switch {
			case m.IsCommand() && m.Command() == "start":
				poster.StopJobSearch(m.Chat.ID)
				if err := poster.StopWorker(ctx, m.Chat.ID); err != nil {
					log.Print("worker conversation reset failed")
					continue
				}
				args := strings.TrimSpace(m.CommandArguments())

				// ── Known user shortcut ───────────────────────────────────
				// If we've seen this Telegram user before AND we have their
				// phone in the users collection, skip the contact-share step
				// and just issue a fresh code immediately.
				if phone, ok := findKnownPhone(ctx, usersCol, m.From.ID); ok {
					// Sessiyaga bog'lanmagan kod foydasiz — noSessionMessage izohi.
					if args == "" {
						log.Printf("start without session token (known user tgID=%d)", m.From.ID)
						sendNoSession(bot, m.Chat.ID, webURL)
						continue
					}
					code, err := generateAndStore(ctx, otpCol, args, phone, m.From.ID, otpTTL, otpLen)
					if err != nil {
						log.Printf("otp store failed (known user tgID=%d): %v", m.From.ID, err)
						_, _ = bot.Send(tgbotapi.NewMessage(m.Chat.ID, "Xatolik yuz berdi. Iltimos, keyinroq qayta urinib ko'ring."))
						continue
					}
					sendOTP(bot, m.Chat.ID,
						fmt.Sprintf("Qaytib xush kelibsiz!\n\nTasdiqlash kodingiz: `%s`\n\n(Kodni nusxalash uchun ustiga bosing.)\nKodni saytda kiriting. Kod %d daqiqa amal qiladi.",
							code, int(otpTTL/time.Minute)))
					delete(pending, m.Chat.ID)
					continue
				}

				// ── First-time flow: ask for contact ──────────────────────
				if args == "" {
					log.Printf("start without session token (tgID=%d)", m.From.ID)
					sendNoSession(bot, m.Chat.ID, webURL)
					continue
				}
				pending[m.Chat.ID] = args
				// Xotiradagi nusxadan tashqari draft'ga ham yozamiz: bot shu
				// yerda qayta ishga tushsa, kontakt kelganda token topiladi.
				if err := poster.SaveAuthToken(ctx, m.Chat.ID, args); err != nil {
					log.Printf("auth token not persisted (tgID=%d)", m.From.ID)
				}
				req := tgbotapi.NewMessage(m.Chat.ID, "Salom! \"Ishchi Bormi\" ga xush kelibsiz.\n\nIltimos, telefon raqamingizni ulashing.")
				kb := tgbotapi.NewReplyKeyboard(
					tgbotapi.NewKeyboardButtonRow(tgbotapi.KeyboardButton{Text: "📞 Telefon raqamni ulashish", RequestContact: true}),
				)
				kb.OneTimeKeyboard = true
				kb.ResizeKeyboard = true
				req.ReplyMarkup = kb
				_, _ = bot.Send(req)

			case m.Contact != nil:
				// Telegram lets a user send an arbitrary saved contact, not only the
				// special "share my phone" contact produced by our keyboard.  Binding
				// that arbitrary number would let an attacker authenticate as somebody
				// else.  Telegram sets Contact.UserID only when the contact belongs to
				// the sender, so require an exact match before issuing an OTP.
				if !isOwnContact(m.From.ID, m.Contact.UserID) {
					log.Printf("rejected foreign contact sender=%d contactUser=%d", m.From.ID, m.Contact.UserID)
					_, _ = bot.Send(tgbotapi.NewMessage(m.Chat.ID,
						"Faqat o'zingizning telefon raqamingizni pastdagi tugma orqali yuboring."))
					continue
				}
				phone := normalizePhone(m.Contact.PhoneNumber)
				// Avval draft'dagi (Mongo) nusxa — u restartdan omon qoladi va
				// o'qilishi bilan o'chiriladi. Xotiradagi `pending` — yozuv
				// muvaffaqiyatsiz bo'lgan holat uchun zaxira.
				token := poster.TakeAuthToken(ctx, m.Chat.ID)
				if token == "" {
					token = pending[m.Chat.ID]
				}
				if token == "" {
					log.Printf("contact without session token (tgID=%d)", m.From.ID)
					sendNoSession(bot, m.Chat.ID, webURL)
					continue
				}
				code, err := generateAndStore(ctx, otpCol, token, phone, m.From.ID, otpTTL, otpLen)
				if err != nil {
					log.Printf("otp store failed (contact tgID=%d): %v", m.From.ID, err)
					_, _ = bot.Send(tgbotapi.NewMessage(m.Chat.ID, "Xatolik yuz berdi. Iltimos, keyinroq qayta urinib ko'ring."))
					continue
				}
				sendOTP(bot, m.Chat.ID, fmt.Sprintf("Tasdiqlash kodingiz: `%s`\n\n(Kodni nusxalash uchun ustiga bosing.)\nKodni saytda kiriting. Kod %d daqiqa amal qiladi.", code, int(otpTTL/time.Minute)))
				delete(pending, m.Chat.ID)

			default:
				handlePosting(u)
			}
		}
	}
	dispatchUpdates(ctx, updates, handleUpdates, handleMembership)
}

// asCommand xabarni Telegram buyrug'iga aylantiradi: IsCommand()/Command()
// aynan matn va birinchi bot_command entity'siga qaraydi, shuning uchun
// ikkalasini ham o'rnatamiz. Uzunlik ASCII buyruq bo'lgani uchun bayt = belgi.
func asCommand(m *tgbotapi.Message, command string) {
	m.Text = "/" + command
	m.Entities = []tgbotapi.MessageEntity{{Type: "bot_command", Offset: 0, Length: len(m.Text)}}
}

// httpsURL — tugmaga URL qo'yish xavfsizmi? Telegram http va localhost
// manzillarini rad etadi va BUTUN xabarni yubormaydi, shuning uchun lokal
// muhitda tugma umuman qo'shilmaydi.
func httpsURL(raw string) bool {
	u, err := url.Parse(strings.TrimSpace(raw))
	return err == nil && u.Scheme == "https" && u.Host != ""
}

// sendNoSession — bot sayt havolasisiz ochilganda. Ilgari bu yerda quruq matn
// turardi va foydalanuvchi boshi berk ko'chaga tushardi; endi ikkala yo'l ham
// bitta bosishda: ishlarni ko'rish yoki saytga o'tib kirish.
func sendNoSession(bot *tgbotapi.BotAPI, chat int64, webURL string) {
	msg := tgbotapi.NewMessage(chat, noSessionMessage)
	rows := [][]tgbotapi.InlineKeyboardButton{
		tgbotapi.NewInlineKeyboardRow(tgbotapi.NewInlineKeyboardButtonData("📍 Yaqin ishlarni ko'rish", "jobs:start")),
	}
	if httpsURL(webURL) {
		rows = append(rows, tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonURL("🌐 Saytda kirish", strings.TrimRight(webURL, "/")+"/login")))
	}
	msg.ReplyMarkup = tgbotapi.NewInlineKeyboardMarkup(rows...)
	_, _ = bot.Send(msg)
}

// sendOTP faqat kodni yuboradi.
//
// Ilgari bu yerda "Saytga qaytish" havolasi va undan keyin alohida taklif
// xabari bo'lardi. Ikkalasi ham olib tashlandi: odam kodni olgani uchun
// keladi va uni darhol ko'rishi kerak — qo'shimcha tugma bilan matn kodni
// pastga surib, chatni shovqinga to'ldirardi.
//
// Pastki menyu esa AYNI KOD XABARIGA biriktiriladi, alohida xabar bilan
// emas. Bu ataylab: kontakt so'ragan bir martalik klaviatura shu yerda
// almashtirilishi kerak, aks holda u chatda osilib qolardi. Ya'ni menyu
// avvalgidek o'rnatiladi, faqat ortiqcha xabarsiz.
func sendOTP(bot *tgbotapi.BotAPI, chat int64, text string) {
	msg := tgbotapi.NewMessage(chat, text)
	msg.ParseMode = "Markdown"
	msg.ReplyMarkup = posting.MainKeyboard()
	if _, err := bot.Send(msg); err != nil {
		log.Print("otp message not delivered")
	}
}

func isBotCommand(m *tgbotapi.Message) bool {
	if !m.IsCommand() {
		return false
	}
	arg := strings.TrimSpace(m.CommandArguments())
	return m.Command() != "start" || arg == "" || arg == "post" || arg == "jobs" || strings.HasPrefix(arg, "app_") || strings.HasPrefix(arg, "job_")
}

// noSessionMessage — foydalanuvchi botni sayt/ilova havolasisiz ochganda
// ko'rsatiladigan matn.
//
// NEGA KOD CHIQARILMAYDI: kod `tgToken` bilan bog'lanadi va sayt ham, mobil
// ilova ham tasdiqlashda AYNAN shu tokenni yuboradi
// (auth_remote_datasource.dart: verifyOtp({token, code})). Tokensiz yozilgan
// kodni ikkala klient ham topa olmaydi — foydalanuvchi ishlamaydigan kodni
// kiritib, "Kod noto'g'ri yoki muddati tugagan" xabarini oladi. Shuning
// uchun bunday kodni umuman bermaymiz va nima qilish kerakligini aytamiz.
var noSessionMessage = strings.Join([]string{
	"Kod olish uchun saytdagi yoki ilovadagi «Telegram orqali kirish» tugmasini bosing —",
	"shundan keyin men sizga kod yuboraman.",
	"",
	"Bu suhbatni Telegram tarixidan ochsangiz kod ishlamaydi.",
}, "\n")

func isOwnContact(senderID, contactUserID int64) bool {
	return senderID != 0 && contactUserID == senderID
}

// findKnownPhone looks up a user by telegramId in the users collection.
// Returns the verified phone if available.
func findKnownPhone(ctx context.Context, users *mongo.Collection, tgID int64) (string, bool) {
	if tgID == 0 {
		return "", false
	}
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	var u userDoc
	err := users.FindOne(ctx,
		bson.M{"telegramId": tgID, "phone": bson.M{"$ne": ""}},
		options.FindOne().SetProjection(bson.M{"phone": 1, "telegramId": 1}),
	).Decode(&u)
	if err != nil || u.Phone == "" {
		return "", false
	}
	return u.Phone, true
}

func generateAndStore(ctx context.Context, col *mongo.Collection, token, phone string, tgID int64, ttl time.Duration, length int) (string, error) {
	code, err := randDigits(length)
	if err != nil {
		return "", err
	}
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	now := time.Now()
	if token != "" {
		res := col.FindOneAndUpdate(ctx,
			bson.M{"tgToken": token, "used": false, "expiresAt": bson.M{"$gt": now}},
			bson.M{"$set": bson.M{
				"phone": phone, "telegramId": tgID, "code": code, "expiresAt": now.Add(ttl),
			}},
		)
		if res.Err() == nil {
			return code, nil
		}
	}
	// Fallback (bot opened directly, without a web token): keep one active
	// phone-based code per Telegram user. Upsert instead of insert so retrying
	// the contact-share updates the same record rather than piling up duplicates
	// — which also removes any chance of a unique-index collision. Web verifies
	// this via the code-only (phone) fallback.
	_, err = col.UpdateOne(ctx,
		bson.M{"telegramId": tgID, "tgToken": ""},
		bson.M{
			"$set": bson.M{
				"phone": phone, "code": code, "used": false,
				"attempts": 0, "expiresAt": now.Add(ttl),
			},
			"$setOnInsert": bson.M{"createdAt": now},
		},
		options.Update().SetUpsert(true),
	)
	return code, err
}

func normalizePhone(p string) string {
	p = strings.TrimSpace(p)
	if !strings.HasPrefix(p, "+") {
		p = "+" + p
	}
	return p
}

func randDigits(n int) (string, error) {
	const digits = "0123456789"
	out := make([]byte, n)
	b := make([]byte, 1)
	for i := 0; i < n; {
		if _, err := rand.Read(b); err != nil {
			return "", err
		}
		// Rejection sampling: 250..255 tashlab yuboriladi, aks holda 256%10=6
		// tufayli 0-5 raqamlari boshqalardan tez-tezroq chiqardi (modulo bias).
		if b[0] >= 250 {
			continue
		}
		out[i] = digits[int(b[0])%10]
		i++
	}
	return string(out), nil
}

func getenv(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}

func envInt(k string, def int) int {
	if v := os.Getenv(k); v != "" {
		if i, err := strconv.Atoi(v); err == nil {
			return i
		}
	}
	return def
}
