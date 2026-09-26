package main

import (
	"context"
	"errors"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"

	"github.com/ishchibormi/backend/config"
	"github.com/ishchibormi/backend/internal/account"
	"github.com/ishchibormi/backend/internal/admin"
	"github.com/ishchibormi/backend/internal/application"
	"github.com/ishchibormi/backend/internal/auth"
	"github.com/ishchibormi/backend/internal/category"
	"github.com/ishchibormi/backend/internal/channelpost"
	"github.com/ishchibormi/backend/internal/elon"
	"github.com/ishchibormi/backend/internal/errlog"
	"github.com/ishchibormi/backend/internal/feedback"
	"github.com/ishchibormi/backend/internal/jobalert"
	"github.com/ishchibormi/backend/internal/moderation"
	"github.com/ishchibormi/backend/internal/notification"
	"github.com/ishchibormi/backend/internal/push"
	"github.com/ishchibormi/backend/internal/report"
	"github.com/ishchibormi/backend/internal/upload"
	"github.com/ishchibormi/backend/internal/user"
	"github.com/ishchibormi/backend/pkg/db"
	"github.com/ishchibormi/backend/pkg/envfile"
	"github.com/ishchibormi/backend/pkg/gemini"
	"github.com/ishchibormi/backend/pkg/httpx"
	"github.com/ishchibormi/backend/pkg/logger"
	"github.com/ishchibormi/backend/pkg/storage"
	"github.com/ishchibormi/backend/pkg/tgsend"
)

func main() {
	envfile.Load()
	log := logger.New()
	cfg := config.Load()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	mdb, err := db.Connect(ctx, cfg.MongoURI, cfg.MongoDB)
	if err != nil {
		log.Error("mongo connect failed", "err", err)
		os.Exit(1)
	}
	// Dastur xatoliklari jurnali ("3.12 · Xatoliklar"). Eng birinchi
	// xizmatlardan biri bo'lib ko'tariladi — quyidagi boot tekshiruvlari
	// (indekslar, migratsiya, moderatsiya, sirlar) o'z nosozliklarini shu
	// yerga yozadi. Aks holda ular faqat boot logida qolib, hech kim
	// ko'rmaydigan bir qator bo'lardi.
	errRec := errlog.New(mdb, log, tgsend.New(cfg.TelegramBotToken), cfg.ErrorAlertChatID, cfg.JWTAccessSecret)
	defer func() {
		c, done := context.WithTimeout(context.Background(), 3*time.Second)
		defer done()
		errRec.Close(c)
	}()
	// httpx quyi qatlam bo'lgani uchun errlog'ni import qila olmaydi —
	// panic'lar ilgak orqali keladi (pkg/httpx/middleware.go).
	httpx.PanicHook = errRec.Hook

	if err := db.EnsureIndexes(ctx, mdb); err != nil {
		log.Warn("ensure indexes", "err", err)
		errRec.Record(errlog.Event{
			Code: "index_create_failed", Where: "pkg/db.EnsureIndexes",
			Message: err.Error(), Origin: errlog.OriginServer,
		})
	}
	// Bir martalik, versiyalangan migratsiyalar (schema_migrations' da qayd
	// etiladi — har biri faqat bir marta ishlaydi, shuning uchun ma'lumot
	// o'sgani sayin boot sekinlashmaydi). Yuqoridagi EnsureIndexes va quyidagi
	// EnsureDefaults ataylab har boot'da ishlaydi (idempotent / biznes
	// moslashtiruvi) va bu registrga kirmaydi.
	if err := db.RunMigrations(ctx, mdb); err != nil {
		log.Warn("run migrations", "err", err)
		errRec.Record(errlog.Event{
			Code: "migration_failed", Where: "pkg/db.RunMigrations",
			Message: err.Error(), Origin: errlog.OriginServer,
		})
	}
	// Boshlang'ich tizim turkumlarini upsert qilamiz. Admin yaratgan turkumlar
	// deploy/restart paytida o'zgartirilmaydi.
	if err := category.EnsureDefaults(ctx, mdb); err != nil {
		log.Warn("ensure categories", "err", err)
	}

	// services
	notif := notification.New(mdb)
	if tg := tgsend.New(cfg.TelegramBotToken); tg.Configured() {
		telegram := notification.NewTelegram(mdb, tg, log)
		telegram.WebBaseURL = cfg.WebBaseURL
		notif.AttachTelegram(telegram)
		go telegram.Run(ctx)
		log.Info("telegram job notifications ready")
	}

	// Mobil push (FCM). Credentials berilmasa API va Telegram bildirishnomalari
	// ishlayveradi; ilova bildirishnomalarni polling orqali oladi.
	// Ulanish nuqtasi: notification.Service.Push ichidagi Pusher chaqiruvi —
	// har bir in-app notification (ariza, qabul, rad, broadcast) shu yerdan
	// o'tadi, shuning uchun alohida "push yuborish" kodi hech qayerda kerak emas.
	pushH := push.NewHandler(mdb)
	if cfg.FCMCredentialsFile != "" {
		if fcm, err := push.NewFCM(cfg.FCMCredentialsFile, mdb, log); err != nil {
			log.Warn("fcm init failed — mobile push disabled", "err", err)
			// Credentials fayli BERILGAN, ya'ni push kutilgan — lekin
			// ishga tushmadi. Bu jimgina o'chib qolish: hech kim
			// bildirishnoma kelmaganini darrov sezmaydi.
			errRec.Record(errlog.Event{
				Code: "fcm_init_failed", Where: "push.NewFCM",
				Message: err.Error(), Origin: errlog.OriginServer,
			})
		} else {
			notif.AttachPusher(fcm)
			log.Info("fcm push ready", "project", fcm.ProjectID())
		}
	} else {
		log.Info("fcm push disabled (FCM_CREDENTIALS_FILE not set)")
	}

	// Kontent moderatsiyasi (Google Gemini) — ixtiyoriy. GEMINI_API_KEY
	// berilmasa jimgina o'chiq: /api/moderation/* 503 qaytaradi va e'lon
	// yaratish oqimi umuman tegilmaydi. Kalit hech qachon logga chiqmaydi —
	// faqat yoqilgan/o'chiq holati va model nomi.
	modSvc := moderation.New(gemini.New(
		cfg.GeminiAPIKey, cfg.GeminiBaseURL, cfg.GeminiModel, cfg.GeminiTimeout))
	modH := moderation.NewHandler(modSvc, cfg.ModerationMaxImageBytes)
	// Guard — e'lon, profil va taklif/shikoyat oqimlarida BIR XIL siyosat
	// (bayroq + fail-open/closed). Uni har bir domenda qayta yozish
	// xavfsizlik qoidasini uch joyda ushlab turishni anglatardi.
	// Buzilishlar hisobi TELEFON raqami bo'yicha yuritiladi — hisobni
	// o'chirib qayta ro'yxatdan o'tish jazoni nolga qaytarmasligi uchun.
	modStrikes := moderation.NewStrikeStore(mdb, cfg.ModerationStrikeLimit, cfg.ModerationBanDuration)
	modGuard := moderation.NewGuard(modSvc, modStrikes, moderation.GuardOptions{
		Enforce:    cfg.ModerationEnforce,
		FailClosed: cfg.ModerationFailClosed,
	})
	if modSvc.Enabled() {
		log.Info("content moderation ready", "model", modSvc.Model(),
			"enforce", cfg.ModerationEnforce, "failClosed", cfg.ModerationFailClosed,
			"strikeLimit", modStrikes.Limit(), "banDays", int(modStrikes.BanDuration().Hours()/24))
		// Productionda fail-open — ataylab qo'yilgan vaqtinchalik holat.
		// Jimgina qolsa, moderatsiya "ishlayapti" deb o'ylanadi, aslida esa
		// tashqi xizmat uzilgan paytda hamma e'lon tekshirilmasdan o'tadi.
		if cfg.ModerationFailOpenInProd() {
			log.Warn("moderation runs FAIL-OPEN in production — unchecked listings will be published whenever the moderation service is unavailable",
				"fix", "unset MODERATION_FAIL_CLOSED or set it to true")
			errRec.Record(errlog.Event{
				Code: "moderation_fail_open", Where: "cmd/api/main.go",
				Message: "MODERATION_FAIL_CLOSED o'rnatilmagan — xizmat uzilganda hamma e'lon tekshirilmay chop etiladi",
				Origin:  errlog.OriginServer,
			})
		}
	} else {
		log.Info("content moderation disabled (GEMINI_API_KEY not set)")
	}

	// S3 storage — optional. If creds aren't set, upload endpoints return 503.
	var s3svc *storage.Service
	if cfg.AWSS3Bucket != "" {
		s3svc, err = storage.New(ctx, storage.Config{
			Region: cfg.AWSRegion, AccessKeyID: cfg.AWSAccessKeyID,
			SecretAccessKey: cfg.AWSSecretAccessKey,
			Bucket:          cfg.AWSS3Bucket, PublicBaseURL: cfg.AWSS3PublicBaseURL,
		})
		if err != nil {
			log.Warn("s3 init", "err", err)
			errRec.Record(errlog.Event{
				Code: "storage_unavailable", Where: "storage.New",
				Message: err.Error(), Origin: errlog.OriginServer,
			})
		} else {
			log.Info("s3 ready", "bucket", cfg.AWSS3Bucket, "region", cfg.AWSRegion)
		}
	} else {
		// No S3 configured — fall back to local-disk storage so uploads work
		// out of the box. Files are written under cfg.UploadDir and served by
		// this API at cfg.UploadPublicBase (see the /uploads/* route below).
		s3svc, err = storage.NewLocal(cfg.UploadDir, cfg.UploadPublicBase)
		if err != nil {
			log.Warn("local storage init failed", "err", err)
			errRec.Record(errlog.Event{
				Code: "storage_unavailable", Where: "storage.NewLocal",
				Message: err.Error(), Origin: errlog.OriginServer,
			})
		} else {
			log.Info("local storage ready", "dir", cfg.UploadDir, "base", cfg.UploadPublicBase)
		}
	}

	authH := auth.NewHandler(cfg, mdb)
	userH := user.NewHandler(mdb, s3svc)
	accountH := account.NewHandler(cfg, mdb, s3svc)
	catH := category.NewHandler(mdb)
	elonH := elon.NewHandler(mdb, s3svc, notif)
	// Kanalga chiqarish endi sozlangan bitta kanalga emas, bot administrator
	// bo'lgan BARCHA kanallarga ishlaydi. TelegramJobsChannelID faqat eski
	// sozlamani reyestrga kiritish uchun (ixtiyoriy).
	channelPublisher, channelErr := channelpost.New(mdb, tgsend.New(cfg.TelegramBotToken), cfg.TelegramBotUsername, cfg.TelegramJobsChannelID, log)
	if channelErr != nil {
		log.Warn("channel publication disabled", "reason", channelErr.Error())
	} else if channelPublisher != nil {
		elonH.Channel = channelPublisher
		go channelPublisher.Run(ctx)
		log.Info("channel publication worker ready")
	}
	// Moderatsiyani e'lon yaratish oqimiga ulaymiz. MODERATION_ELON_ENFORCE
	// o'chiq (standart) bo'lsa bu chaqiruv POST /api/elons xatti-harakatini
	// o'zgartirmaydi — moderatsiya faqat /api/moderation/* orqali ishlaydi.
	elonH.AttachModerator(modGuard, cfg.ModerationMaxImageBytes)
	// Ish signali: foydalanuvchi belgilagan hududda yangi e'lon chiqqanda
	// xabar beradi. Yetkazish odatdagi bildirishnoma quvuri orqali ketadi,
	// shuning uchun bu yerda alohida worker yo'q.
	alertH := jobalert.New(mdb, notif, log)
	elonH.Alerts = alertH
	appH := application.NewHandler(mdb, notif)
	repH := report.NewHandler(mdb)
	fbH := feedback.NewHandler(mdb)
	// Profil (bio, ism, ko'nikmalar) va taklif/shikoyat matni ham shu
	// guard orqali tekshiriladi.
	userH.AttachModerator(modGuard)
	fbH.AttachModerator(modGuard)
	uploadH := upload.NewHandler(s3svc)
	uploadH.AvatarUploads = mdb.Collection("avatar_uploads")
	// Profil rasmi yuklash paytida tekshiriladi.
	uploadH.AttachModerator(modGuard)
	adminH := admin.NewHandler(cfg, mdb, notif, s3svc)
	// Buzilishlar do'koni admin paneliga ham kerak: blokni ochish uni telefon
	// bo'yicha o'chiradi, "batafsil" ko'rinishi esa undan buzilishlar tarixini
	// o'qiydi. Bog'lanmasa `h.Strikes` nil qoladi va blokni ochish jimgina
	// 503 qaytaradi — aynan shu sabab moderatsiya bloki hech qachon
	// ochilmasdi.
	//
	// Gemini kalitidan MUSTAQIL: kalit yo'q bo'lsa yangi blok qo'yilmaydi,
	// lekin allaqachon qo'yilganlarini ochish kerak bo'lib qolaveradi.
	adminH.Strikes = modStrikes
	// Background scheduler: delivers due scheduled broadcasts (checks every
	// minute). Stops when ctx is cancelled on shutdown.
	go adminH.RunScheduler(ctx)
	go adminH.RunAvatarDeletionWorker(ctx)
	go adminH.RunElonModerationWorker(ctx)
	// Background scheduler: qabul qilingan ishlarni belgilangan vaqtdan 18 soat
	// o'tgach (agar ikki tomon ham bekor qilmagan bo'lsa) avtomatik yakunlab,
	// ish tarixiga (arxivga) o'tkazadi. ctx bekor qilinganda to'xtaydi.
	go appH.RunAutoCompleteScheduler(ctx)
	// Retry owner edits/cancellations whose candidate updates or notifications
	// were interrupted by a disconnect or transient database failure.
	go elonH.RunOwnerActionWorker(ctx)
	// Background sweeper: permanently erases accounts whose retention window
	// (ACCOUNT_RETENTION_DAYS, default 90) has closed — the second stage of
	// account deletion required by Google Play. Covers both self-service and
	// admin deletions; see internal/account/retention.go. Stops on ctx cancel.
	purger := account.NewPurger(mdb, s3svc, cfg.AccountRetentionDays, log)
	go purger.Run(ctx)
	// Admin panelidagi "bazadan ham o'chirish" rejimi AYNAN shu purger'ni
	// chaqiradi (internal/admin/deletemode.go). Bog'lanmasa o'sha rejim 503
	// qaytaradi va yashirish odatdagidek ishlayveradi.
	adminH.Purger = purger
	log.Info("account retention active", "days", purger.RetentionDays())
	// "Telegram'ga yuborish" tugmasi (Figma 3.12.1 sarlavhasi va 3.12.3 · L).
	// Konstruktor imzosi o'zgarmadi: bog'lanmagan holat NORMAL — tugma 503
	// qaytaradi, panelning qolgan qismi ishlayveradi.
	adminH.TG = tgsend.New(cfg.TelegramBotToken)
	adminH.AlertChatID = cfg.ErrorAlertChatID

	// "Sababini aniqla" tugmasi (Figma 3.12.1) — xatolik kontekstini AI
	// tahlil qiladi. Moderatsiyadan ALOHIDA kalit va model: bu ikki ish
	// bir-biriga bog'liq emas va biri o'chganda ikkinchisi ishlab turishi
	// kerak. Kalit berilmasa faqat shu tugma 503 qaytaradi.
	aiModel := cfg.ErrorAIModel
	if aiModel == "" {
		aiModel = gemini.DefaultAnalyzeModel
	}
	adminH.AI = gemini.New(cfg.ErrorAIAPIKey, cfg.GeminiBaseURL, aiModel, cfg.ErrorAITimeout)
	if adminH.AI.Configured() {
		log.Info("error AI analysis ready", "model", adminH.AI.Model(),
			"timeout", cfg.ErrorAITimeout.String())
	} else {
		log.Info("error AI analysis disabled (ERROR_AI_API_KEY not set)")
	}

	// Rate limiting keys off the real client IP. Only trust forwarding headers
	// when explicitly configured to sit behind a trusted proxy; otherwise XFF is
	// spoofable and defeats the limiter. TrustedProxyHops decides which element
	// of the forwarded chain is the client — see httpx.forwardedClientIP.
	httpx.TrustProxyHeaders = cfg.TrustProxyHeaders
	httpx.TrustedProxyHops = cfg.TrustedProxyHops

	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	// NOTE: chi's middleware.RealIP is deliberately NOT mounted. It trusts
	// True-Client-IP / X-Real-IP / the LEFTMOST X-Forwarded-For element — all
	// three fully attacker-controlled behind an appending proxy — and it
	// overwrites r.RemoteAddr with the result, poisoning the fallback path too.
	// httpx.clientIP resolves the client itself, counting from the trusted end.
	r.Use(httpx.AccessLog)
	// So'rov egasining "qutisi" — auth middleware'i uni ichkarida to'ldiradi,
	// quyidagi ikkalasi esa tashqaridan o'qiydi ("Ta'sirlangan foydalanuvchi"
	// ko'rsatkichi shundan chiqadi). Kontekst faqat pastga oqqani uchun
	// boshqa yo'l yo'q — httpx.ActorSink izohiga qarang.
	r.Use(httpx.ActorSink)
	r.Use(httpx.Recover)
	// Xatolik jurnali Recover'dan ICHKARIDA: panic bu yerdan o'tib ketadi va
	// faqat PanicHook orqali bir marta yoziladi. Aks holda bitta panic ikki
	// qator bo'lib tushardi — biri `panic`, ikkinchisi 500 javob sifatida.
	r.Use(errRec.Middleware)
	r.Use(httpx.SecurityHeaders)
	// Foydalanuvchi oqimida auth — Authorization sarlavhasidagi Bearer token,
	// cookie umuman ishlatilmaydi.
	//
	// AllowCredentials esa YOQILGAN va u faqat BITTA narsa uchun kerak: admin
	// panelining refresh cookie'si (internal/admin/refresh.go). Productionda
	// panel va admin API bitta originda bo'lgani uchun bu yo'l umuman ishga
	// tushmaydi; lokal ishlashda esa panel :3000 da, backend :8080 da turadi
	// va bayroqsiz cookie yuborilmasdi — ya'ni dasturchi sessiya oqimini
	// sinab ko'ra olmasdi.
	//
	// Xavf qo'shmaydi: ruxsat etilgan originlar ro'yxati aniq (config
	// wildcard'ni umuman qabul qilmaydi), cookie esa HttpOnly + SameSite=Strict
	// — begona sayt uni brauzerga yubortira olmaydi.
	r.Use(cors.Handler(cors.Options{
		AllowedOrigins:   cfg.CORSOrigins,
		AllowCredentials: true,
		AllowedMethods:   []string{"GET", "POST", "PATCH", "PUT", "DELETE", "OPTIONS"},
		// X-Client-Platform — brauzer klienti o'zini tanitadi (httpx.
		// ClientPlatformHeader). Standart bo'lmagan sarlavha, ya'ni ruxsat
		// berilmasa brauzer preflight'da so'rovni butunlay to'xtatadi.
		AllowedHeaders: []string{"Authorization", "Content-Type", "Idempotency-Key", httpx.ClientPlatformHeader},
		MaxAge:         300,
	}))

	otpLimiter := httpx.NewLimiter(10, 0.5)    // 10 burst, 1 / 2s
	applyLimiter := httpx.NewLimiter(20, 0.5)  // 20 burst, slow refill
	loginLimiter := httpx.NewLimiter(8, 0.2)   // 8 burst, 1 / 5s — throttles credential brute-force
	deleteLimiter := httpx.NewLimiter(5, 0.05) // 5 burst, 1 / 20s — account-deletion codes hit Telegram
	// Refresh legitimately fires on every 401 from the mobile interceptor, so
	// the budget is generous — this only caps offline brute-force of refresh
	// tokens and accidental client retry-loops.
	refreshLimiter := httpx.NewLimiter(30, 0.1) // 30 burst, 1 / 10s
	uploadLimiter := httpx.NewLimiter(12, 0.1)  // 12 burst, then 1 / 10s per authenticated user
	// Har bir moderatsiya so'rovi pullik tashqi API chaqiruvi — budjet
	// e'lon joylash tezligiga moslangan, undan sal kengroq (klient e'lonni
	// yuborishdan oldin bir necha marta tekshirishi mumkin).
	moderationLimiter := httpx.NewLimiter(20, 0.2) // 20 burst, then 1 / 5s per authenticated user
	// Profilni saqlash odatda kamdan-kam bo'ladi (onboarding + vaqti-vaqti
	// bilan tahrir), lekin endi u ham tashqi tekshiruv chaqiradi.
	profileLimiter := httpx.NewLimiter(15, 0.1)   // 15 burst, then 1 / 10s
	alertLimiter := httpx.NewLimiter(15, 0.1)     // 15 burst, keyin 1 / 10s — signal sozlash
	publicReadLimiter := httpx.NewLimiter(120, 5) // generous public-query budget per client IP
	// Authenticated writes that leave permanent, publicly visible or
	// admin-facing residue. Keyed by user id, not IP: the point is to bound what
	// one account can produce, and IP is both spoofable and shared (one mobile
	// carrier NAT would otherwise throttle a whole city).
	elonLimiter := httpx.NewLimiter(10, 0.02) // 10 burst, then 1 / 50s — job posting
	// Reports and feedback fan out to a notification document PER ACTIVE ADMIN,
	// so one request is an amplified write. Tight budget on purpose.
	inboxLimiter := httpx.NewLimiter(5, 0.01) // 5 burst, then 1 / 100s
	// Device-token registration is idempotent for a real client (one token per
	// install, re-sent on rotation), so a low ceiling costs nothing legitimate
	// and stops the device_tokens collection being used as free storage.
	deviceLimiter := httpx.NewLimiter(10, 0.05) // 10 burst, then 1 / 20s

	// Mijoz xatoliklarini qabul qilish (internal/errlog · POST
	// /api/client-errors). Ataylab TOR: bu endpoint admin ekraniga matn
	// yozadigan yagona tashqi yo'l. Bitta hisob soatiga ~72 ta xabar
	// yubora oladi — haqiqiy ilova uchun ortig'i bilan yetadi, jurnalni
	// ko'mib tashlash uchun esa kam.
	errReportLimiter := httpx.NewLimiter(10, 0.02) // 10 burst, keyin 1 / 50s
	// CSV eksport bitta so'rovda 50 000 qatorgacha shaxsiy ma'lumot (telefon
	// raqamlari) chiqaradi — perimetrdan chiqib ketadigan, muddati yo'q yassi
	// fayl. Kaliti ADMIN ID, IP emas: bitta ofisdagi ikkinchi admin
	// birinchisining budjetini yeb qo'ymasligi, o'g'irlangan token esa IP
	// almashtirib cheklovni aylanib o'tolmasligi kerak. Qo'lda tahlil uchun
	// 6 ta ketma-ket fayl yetadi; skript bilan "hammasini so'rib olish"
	// esa shu yerda to'xtaydi.
	exportLimiter := httpx.NewLimiter(6, 0.05) // 6 burst, keyin 1 / 20s (admin bo'yicha)
	// Turkum yozuvlari (Figma 3.7) butun platformaga darhol ko'rinadi: turkum
	// mobil ilovadagi feed filtri va e'lon berish formasidagi ro'yxat.
	// Skript bilan «qo'shish → o'chirish» aylanmasi bazani ham, audit
	// jurnalini ham chippakka chiqarardi, nishonni tez-tez bosish esa
	// foydalanuvchilar uchun turkumni miltillatardi. Kaliti ADMIN ID: cheklov
	// bitta hisob nima qila olishini chegaralaydi, IP esa almashtiriladi.
	// Qo'lda ishlashga 30 ta ketma-ket amal yetib ortadi.
	catWriteLimiter := httpx.NewLimiter(30, 0.5) // 30 burst, keyin 1 / 2s
	// Ikonka yuklash — perimetrga 2 MB'lik fayl yozadigan yagona turkum
	// amali, shuning uchun budjeti alohida va torroq.
	catIconLimiter := httpx.NewLimiter(10, 0.05) // 10 burst, keyin 1 / 20s
	// Ommaviy tarqatma (Figma 3.8) — paneldagi eng qaytarib bo'lmaydigan
	// amal: bitta so'rov o'n minglab bildirishnomaga aylanadi va fon
	// jarayoni butun `users` kolleksiyasini aylanib chiqadi. Yuborilgan
	// xabarni ortga qaytarish yo'q. Skript bilan (yoki o'g'irlangan
	// superadmin tokeni bilan) ketma-ket bosilsa — foydalanuvchilarning
	// ommaviy spamlanishi va bir vaqtda ishlayotgan o'nlab to'liq
	// skanerlash. Kaliti ADMIN ID: cheklov bitta hisob nima qila olishini
	// chegaralaydi, IP esa almashtiriladi. Qo'lda ishlashga ketma-ket 3 ta
	// tarqatma yetadi, keyin har ~2 daqiqada bittasi ochiladi.
	//
	// # NIMA ATAYLAB CHEKLANMAGAN
	//
	// «Bekor qilish» (DELETE /broadcasts/{id}) bu budjetga QO'SHILMAGAN:
	// u to'xtatuvchi amal — rejalashtirilgan xabarni yuborilmasdan oldin
	// olib qo'yadi. Uni sekinlashtirish xavfni kamaytirmaydi, oshiradi:
	// vaqt qisilganda admin bekor qilib qolishga ulgurmasdi.
	broadcastLimiter := httpx.NewLimiter(3, 0.0083) // 3 burst, keyin ~1 / 2 daqiqa
	// Segment ro'yxati (Figma 3.8b: GET /broadcast/regions) — tarqatma
	// formasidagi viloyat tanlagichi. Boshqa admin o'qishlaridan farqi:
	// u sahifalanmaydigan GURUHLASH, ya'ni butun `users` kolleksiyasini
	// aylanib chiqadi. Yuborishning o'zi 3 ta bilan cheklangani holda
	// bu so'rovni cheksiz qoldirish — eng qimmat so'rovni eng ochiq
	// qoldirish bo'lardi. Budjet forma bilan ishlashga mo'l: sahifa
	// ochilganda bir marta va «Faqat faol» katakchasi almashganda
	// so'raladi. Kaliti ADMIN ID.
	broadcastSegmentLimiter := httpx.NewLimiter(20, 0.2) // 20 burst, keyin 1 / 5s
	// Kadr hisoblari (Figma 3.9: POST/PATCH/DELETE /admins) — panelning
	// KALITLARINI tarqatadigan yozuvlar: yangi admin, rol o'zgarishi, parol
	// tiklash, 2FA ni o'chirish. Har biri bcrypt hisoblaydi yoki sessiyalarni
	// uzadi, ya'ni arzon emas. O'g'irlangan superadmin tokeni bilan skript
	// bir soniyada o'nlab yashirin hisob ochib ketishi mumkin edi — cheklov
	// buni sekinlashtiradi va audit jurnalida ko'rinadigan iz qoldiradi.
	// Kaliti ADMIN ID: cheklov bitta hisob nima qila olishini chegaralaydi,
	// IP esa almashtiriladi. Qo'lda ishlashga 20 ta ketma-ket amal yetadi.
	//
	// # NIMA ATAYLAB CHEKLANMAGAN
	//
	// `GET /admins` bu budjetga QO'SHILMAGAN: jadval har amaldan keyin
	// o'zini qayta so'raydi, ya'ni o'qish yozuvlar bilan bir budjetga
	// qo'yilsa, cheklov jadvalni ham to'xtatib, adminni eskirgan ro'yxat
	// oldida qoldirardi.
	staffLimiter := httpx.NewLimiter(20, 0.2) // 20 burst, keyin 1 / 5s

	// Evict idle per-IP buckets so the limiter maps don't grow unbounded (each
	// unique client IP would otherwise leave a permanent entry). The 15-min idle
	// threshold is far above every bucket's full-refill time (<=40s), so eviction
	// never grants a returning client extra allowance. Stops on ctx cancel.
	otpLimiter.StartCleanup(ctx, 5*time.Minute, 15*time.Minute)
	applyLimiter.StartCleanup(ctx, 5*time.Minute, 15*time.Minute)
	loginLimiter.StartCleanup(ctx, 5*time.Minute, 15*time.Minute)
	deleteLimiter.StartCleanup(ctx, 5*time.Minute, 15*time.Minute)
	refreshLimiter.StartCleanup(ctx, 5*time.Minute, 15*time.Minute)
	uploadLimiter.StartCleanup(ctx, 5*time.Minute, 15*time.Minute)
	moderationLimiter.StartCleanup(ctx, 5*time.Minute, 15*time.Minute)
	profileLimiter.StartCleanup(ctx, 5*time.Minute, 15*time.Minute)
	publicReadLimiter.StartCleanup(ctx, 5*time.Minute, 15*time.Minute)
	// These three refill slowly, so their buckets need a correspondingly longer
	// idle threshold — evicting one before it has refilled would hand the owner
	// a fresh full budget and undo the limit.
	elonLimiter.StartCleanup(ctx, 15*time.Minute, 30*time.Minute)
	inboxLimiter.StartCleanup(ctx, 15*time.Minute, 30*time.Minute)
	deviceLimiter.StartCleanup(ctx, 15*time.Minute, 30*time.Minute)
	errReportLimiter.StartCleanup(ctx, 15*time.Minute, 30*time.Minute)
	exportLimiter.StartCleanup(ctx, 15*time.Minute, 30*time.Minute)
	catWriteLimiter.StartCleanup(ctx, 5*time.Minute, 15*time.Minute)
	catIconLimiter.StartCleanup(ctx, 15*time.Minute, 30*time.Minute)
	// Tarqatma budjeti bo'shdan to'lguncha ~6 daqiqa ketadi, shuning uchun
	// 30 daqiqalik bo'sh turish chegarasi xavfsiz: undan oldin tozalash
	// egasiga yangi to'liq budjet berib, cheklovni bekor qilardi.
	broadcastLimiter.StartCleanup(ctx, 15*time.Minute, 30*time.Minute)
	broadcastSegmentLimiter.StartCleanup(ctx, 5*time.Minute, 15*time.Minute)
	staffLimiter.StartCleanup(ctx, 5*time.Minute, 15*time.Minute)

	r.Get("/healthz", func(w http.ResponseWriter, _ *http.Request) { httpx.JSON(w, 200, map[string]string{"status": "ok"}) })

	// Serve locally-stored uploads (only when running without S3). Public, no
	// auth — these are image URLs embedded in elons/avatars.
	if s3svc != nil && s3svc.LocalDir() != "" {
		fs := http.StripPrefix("/uploads/", http.FileServer(http.Dir(s3svc.LocalDir())))
		serveUpload := func(w http.ResponseWriter, r *http.Request) {
			// http.FileServer generates directory listings by default. Upload URLs
			// are public, but the complete object inventory/user-id hierarchy is not.
			ext := strings.ToLower(filepath.Ext(r.URL.Path))
			if strings.HasSuffix(r.URL.Path, "/") || (ext != ".jpg" && ext != ".jpeg" && ext != ".png" && ext != ".webp") {
				http.NotFound(w, r)
				return
			}
			w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
			if strings.HasPrefix(r.URL.Path, "/uploads/avatars/") || strings.HasPrefix(r.URL.Path, "/uploads/elons/") {
				w.Header().Set("Cache-Control", "no-store")
			}
			fs.ServeHTTP(w, r)
		}
		r.Get("/uploads/*", serveUpload)
		r.Head("/uploads/*", serveUpload)
	}

	r.Route("/api", func(r chi.Router) {
		// /api/healthz — /healthz ning taxallusi.
		//
		// Nega kerak: asosiy domenda Caddy faqat /api/* ni backendga
		// yo'naltiradi, ya'ni ishchibormi.uz/healthz frontendga tushib 404
		// beradi. Monitoring vositalari va deploy tekshiruvlari odatda
		// /api/healthz ni so'raydi — natijada uzluksiz 404 oqimi hosil
		// bo'lib, fail2ban (caddy-4xx) tekshiruvchining IP'sini bloklab
		// qo'yardi. Aynan shu tarzda sayt egasi ham saytdan chiqib qolgan.
		r.Get("/healthz", func(w http.ResponseWriter, _ *http.Request) {
			httpx.JSON(w, 200, map[string]string{"status": "ok"})
		})

		// Public auth
		r.Post("/auth/bot/session", authH.BotSession)
		r.With(otpLimiter.Middleware("miniapp-auth")).Post("/auth/miniapp/session", authH.MiniAppSession)
		r.Group(func(r chi.Router) {
			r.Use(otpLimiter.Middleware("otp"))
			r.Post("/auth/otp/request", authH.RequestOTP)
			r.Post("/auth/otp/verify", authH.VerifyOTP)
			r.Get("/auth/otp/peek", authH.DevPeekOTP)
		})
		r.With(refreshLimiter.Middleware("refresh")).Post("/auth/refresh", authH.Refresh)

		// Public listing. OptionalUserAuth doesn't gate anything — it only lets
		// the feed recognise the Play review account, so a reviewer sees the
		// elon they just posted instead of it silently vanishing. Anonymous
		// visitors are unaffected. The review id is passed only while a window
		// is actually open.
		reviewUserID := ""
		if cfg.ReviewLoginEnabled {
			reviewUserID = cfg.ReviewLoginUserID
		}
		r.Group(func(r chi.Router) {
			r.Use(httpx.OptionalUserAuth(cfg.JWTAccessSecret, reviewUserID))
			r.With(publicReadLimiter.Middleware("public-read")).Get("/elons", elonH.Feed)
			r.With(publicReadLimiter.Middleware("public-read")).Get("/elons/nearby", elonH.Nearby)
			r.With(publicReadLimiter.Middleware("public-read")).Get("/elons/{id}", elonH.Get)
		})
		r.Get("/elons/sitemap", elonH.Sitemap) // XML sitemap uchun yengil ro'yxat
		r.With(publicReadLimiter.Middleware("public-read")).Get("/users/{id}", userH.GetPublic)
		r.With(publicReadLimiter.Middleware("public-read")).Get("/users", userH.Search)
		r.Get("/categories", catH.List)

		// Auth-protected
		r.Group(func(r chi.Router) {
			r.Use(httpx.UserAuth(cfg.JWTAccessSecret))
			r.Use(auth.RequireActiveUser(authH.Users()))

			r.Get("/me", userH.Me)
			// Barcha qurilmalardagi sessiyalarni bekor qilish (o'g'irlangan
			// token ham). Chaqirgan qurilma yangi juftlik oladi.
			r.With(refreshLimiter.Middleware("revoke-sessions"), auth.DenyReviewAccount).
				Post("/me/sessions/revoke", authH.RevokeSessions)
			// Profil saqlash endi kontent tekshiruvidan o'tadi, ya'ni pullik
			// tashqi chaqiruv qiladi — limitsiz qoldirish abuz vektori
			// bo'lardi. Kalit foydalanuvchi id'si: bir hisob nima
			// ishlab chiqarishini cheklaymiz.
			r.With(profileLimiter.MiddlewareKey("profile", httpx.UserID)).
				Patch("/me", userH.UpdateMe)

			// Mobil qurilmaning FCM push tokenini ro'yxatga olish (login'dan
			// keyin / token yangilanganda) va o'chirish (logout'da).
			r.With(deviceLimiter.MiddlewareKey("device-token", httpx.UserID)).
				Post("/users/me/device-token", pushH.Register)
			r.Delete("/users/me/device-token", pushH.Unregister)

			// Mijoz ilovasidagi DASTUR xatoliklari ("3.12 · Xatoliklar",
			// C6 guruhi). Autentifikatsiya ostida ATAYLAB: bu endpoint
			// admin ekraniga matn olib chiqadigan yagona tashqi yo'l,
			// ochiq bo'lsa uni to'ldirib qo'yish arzon bo'lardi. Daraja
			// va modul mijozdan SO'RALMAYDI — ular kod bo'yicha
			// katalogdan olinadi (internal/errlog/ingest.go).
			r.With(errReportLimiter.MiddlewareKey("client-errors", httpx.UserID)).
				Post("/client-errors", errRec.ClientReport)

			// Self-service account deletion, confirmed by a code pushed to the
			// user's Telegram. Rate-limited on both halves: /request pushes a
			// Telegram message (spam vector), /confirm is a guess against the
			// code (the per-code attempt counter is the hard cap, this just
			// slows a distributed grind).
			r.Group(func(r chi.Router) {
				r.Use(deleteLimiter.Middleware("acct-delete"))
				r.Use(auth.DenyReviewAccount)
				r.Post("/me/delete/request", accountH.RequestDelete)
				r.Post("/me/delete/confirm", accountH.ConfirmDelete)
			})

			r.With(auth.DenyReviewAccount).Post("/users/{id}/block", userH.Block)
			r.With(auth.DenyReviewAccount).Delete("/users/{id}/block", userH.Unblock)

			// Turkumlarni faqat tizim/admin belgilaydi — oddiy foydalanuvchi
			// yangi turkum qo'sha olmaydi (turkumlar oldindan beriladi).

			r.With(elonLimiter.MiddlewareKey("elon-create", httpx.UserID), elon.BotPublicationSource(cfg.BotSharedSecret)).Post("/elons", elonH.Create)
			r.Patch("/elons/{id}", elonH.Update)
			r.Delete("/elons/{id}", elonH.Delete)
			r.Post("/elons/{id}/cancel", elonH.Cancel)
			r.Get("/my/elons", elonH.MyElons)

			r.Group(func(r chi.Router) {
				r.Use(applyLimiter.Middleware("apply"))
				r.Post("/elons/{id}/apply", appH.Apply)
			})
			r.Get("/applications/{id}", appH.Get)
			r.Post("/applications/{id}/accept", appH.Accept)
			r.Post("/applications/{id}/reject", appH.Reject)
			r.Post("/applications/{id}/cancel", appH.Cancel)
			r.Post("/applications/{id}/confirm-done", appH.ConfirmDone)

			r.Get("/my/applications", appH.MyApplications)
			r.Get("/my/elons/applications", appH.MyElonsApplications)
			r.Get("/me/history", appH.History)

			r.Get("/notifications", notif.List)
			r.Post("/notifications/read-all", notif.ReadAll)
			r.Post("/notifications/read", notif.Read)

			// Ish signali. O'qish arzon — cheklov faqat yozuvda, chunki har
			// saqlash hududni qayta indekslanadigan yozuvga aylantiradi.
			r.Get("/job-alerts", alertH.Get)
			r.With(alertLimiter.MiddlewareKey("job-alert", httpx.UserID)).Put("/job-alerts", alertH.Save)
			r.With(alertLimiter.MiddlewareKey("job-alert", httpx.UserID)).Delete("/job-alerts", alertH.Delete)

			// Shikoyat admin moderatsiya navbatiga tushadi — demo hisob real
			// odam haqida shikoyat yubora olmasligi kerak.
			r.With(auth.DenyReviewAccount,
				inboxLimiter.MiddlewareKey("report", httpx.UserID)).Post("/reports", repH.Create)

			// Taklif va shikoyatlar (support Telegram botiga uzatiladi)
			r.With(auth.DenyReviewAccount,
				inboxLimiter.MiddlewareKey("feedback", httpx.UserID)).Post("/feedback", fbH.Create)
			r.Get("/feedback", fbH.Mine)

			// Uploads — demo hisob ommaviy CDN'ga fayl yuklay olmaydi.
			r.With(auth.DenyReviewAccount,
				uploadLimiter.MiddlewareKey("upload", httpx.UserID)).Post("/uploads", uploadH.Upload)
			r.With(auth.DenyReviewAccount).Delete("/uploads", uploadH.Delete)

			// Kontent moderatsiyasi (Google Gemini).
			// Autentifikatsiya ostida ATAYLAB: har bir chaqiruv pullik tashqi
			// so'rov, ochiq endpoint pul sarflaydigan abuz vektori bo'lardi.
			// Klient e'lonni yuborishdan OLDIN shu yerga murojaat qiladi.
			r.Group(func(r chi.Router) {
				r.Use(moderationLimiter.MiddlewareKey("moderation", httpx.UserID))
				r.Post("/moderation/text", modH.Text)
				r.Post("/moderation/image", modH.Image)
				r.Post("/moderation/check", modH.Check)
			})
		})

		// Admin
		r.With(loginLimiter.Middleware("admin-login")).Post("/admin/login", adminH.Login)
		// Admin sessiyasini yangilash. ATAYLAB autentifikatsiyasiz: chaqiruv
		// paytida access token allaqachon eskirgan bo'ladi, ya'ni AdminAuth uni
		// o'tkaza olmasdi. Dalil sifatida refresh tokenning o'zi xizmat qiladi
		// (internal/admin/refresh.go). Budjet foydalanuvchi refreshi bilan bir
		// xil: haqiqiy klient uni har 401 da chaqiradi.
		r.With(refreshLimiter.Middleware("admin-refresh")).Post("/admin/refresh", adminH.Refresh)
		r.Route("/admin", func(r chi.Router) {
			r.Use(httpx.AdminAuth(cfg.JWTAccessSecret))
			r.Use(adminH.RequireActiveAdmin())

			// Overview — read-only, any authenticated admin (incl. support).
			r.Get("/dashboard", adminH.Dashboard)
			r.Get("/stats", adminH.Stats)
			r.Get("/categories", adminH.ListCategories)

			// Current admin + own two-factor — any authenticated admin.
			r.Get("/me", adminH.Me)
			r.Post("/logout", adminH.Logout)
			r.Post("/2fa/setup", adminH.Setup2FA)
			r.Post("/2fa/enable", adminH.Enable2FA)
			r.Post("/2fa/disable", adminH.Disable2FA)
			// Admin ilovasining (Flutter) o'z xatoliklari — C5 guruhi va
			// biometrik qulf. Har qanday admin roli yubora oladi: bu
			// o'sha adminning qurilmasida yuz bergan nosozlik.
			r.With(errReportLimiter.MiddlewareKey("admin-client-errors", httpx.AdminID)).
				Post("/client-errors", errRec.AdminReport)

			// Moderation — superadmin + moderator.
			r.Group(func(r chi.Router) {
				r.Use(httpx.RequireRole("moderator"))
				// Audit log — superadmin + moderator only (support ko'rmaydi).
				r.Get("/audit", adminH.Audit)
				// Dastur xatoliklari (Figma 3.12) — O'QISH. RBAC aynan
				// 3.12.2 · G dagidek: superadmin hammasini, moderator
				// faqat ko'radi, support uchun sahifa umuman yo'q.
				// Stek va so'rov qiymatlari bu yerda saqlanmaydi
				// (internal/errlog/scrub.go), lekin ro'yxatning o'zi
				// ham tizim ichki tuzilishini ochadi — shuning uchun
				// support darajasidan yuqorida.
				r.Get("/errors", adminH.Errors)
				r.Get("/errors/stats", adminH.ErrorStats)
				// Batafsil ko'rinish (Figma 3.12.1 va 3.12.3). Statik
				// `/errors/stats` yuqorida turadi, lekin tartib muhim
				// emas: chi statik yo'lni `{id}` shablonidan oldin
				// tekshiradi, shuning uchun ular to'qnashmaydi.
				// Mas'ul tanlash ro'yxati — TOR proyeksiya, faqat faol
				// hisoblar (internal/admin/errors.go · Assignees).
				// `GET /admins` bu maqsad uchun ishlatilmaydi: u
				// superadmin darajasida va kadr hisobining hammasini
				// beradi.
				r.Get("/errors/assignees", adminH.Assignees)
				r.Get("/errors/{id}", adminH.GetError)
				r.Get("/errors/{id}/events", adminH.ErrorEvents)
				// AI uchun kontekst (3.12.3 · L). Matn SERVERDA
				// yig'iladi va niqob o'chirib bo'lmaydi; o'z chastota
				// chegarasi bor — internal/admin/errexport.go.
				r.Get("/errors/{id}/context", adminH.GetErrorContext)
				// Xuddi shu kontekst bo'yicha AI xulosasi (3.12.1
				// "Sababini aniqla"). GET emas, POST: chaqiruv tashqi
				// xizmatga chiqadi, pul turadi va natijani guruhga
				// yozadi — ya'ni bu o'zgartiruvchi amal.
				r.Post("/errors/{id}/ai", adminH.PostErrorAI)
				// Hayot siklini yuritish (3.12.3 · J) — kuzatish va
				// tuzatish oqimi moderator uchun ham ochiq.
				// "E'tiborsiz qoldirish" esa handler ichida YANA BIR
				// MARTA superadmin deb tekshiriladi va majburiy sabab
				// talab qiladi. Nima uchun handler ichida: bu bitta
				// endpointning bitta qiymatiga tegishli chegara, uni
				// router darajasida ajratish oqimni ikkiga bo'lardi.
				r.Patch("/errors/{id}/status", adminH.PatchErrorStatus)
				r.Patch("/errors/{id}/assignee", adminH.PatchErrorAssignee)
				r.Post("/errors/{id}/notes", adminH.PostErrorNote)
				// Kanalga QO'LDA yuborish — guruh bo'yicha 60 s sovish
				// oynasi bilan (avtomatik ogohlantirishning o'z
				// throttle'i alohida).
				r.Post("/errors/{id}/telegram", adminH.PostErrorTelegram)
				r.Get("/users", adminH.ListUsers)
				r.Get("/users/{id}", adminH.GetUser)
				r.Post("/users/{id}/block", adminH.BlockUser)
				r.Delete("/users/{id}", adminH.DeleteUser)
				r.Post("/users/{id}/verify", adminH.VerifyUser)
				r.Post("/users/{id}/notify", adminH.NotifyUser)
				r.Delete("/users/{id}/avatar", adminH.DeleteUserAvatar)
				r.Get("/elons", adminH.ListElons)
				// Bitta e'lonning batafsil ko'rinishi (Figma 3.5.1) —
				// `GET /users/{id}` bilan bir xil daraja: superadmin +
				// moderator. Mexanizmi internal/admin/elon_detail.go.
				r.Get("/elons/{id}", adminH.GetElon)
				r.Delete("/elons/{id}", adminH.DeleteElon)
				r.Patch("/elons/{id}/status", adminH.SetElonStatus)
				r.Get("/reports", adminH.ListReports)
				r.Patch("/reports/{id}/resolve", repH.Resolve)
				// Arizalar ro'yxati (Figma 3.6) va bitta arizaning batafsil
				// ko'rinishi (Figma 3.6.1) — FAQAT o'qish uchun.
				// Arizani qabul qilish/rad etish admin ishi emas: holatni
				// ishchi va ish beruvchi o'zgartiradi, shuning uchun bu
				// yerda birorta PATCH/DELETE yo'q (Figma 3.6a · qoida).
				r.Get("/applications", adminH.ListApplications)
				r.Get("/applications/{id}", adminH.GetApplication)
				// CSV eksportlar alohida guruhda: ular yagona yo'l bo'lib,
				// bitta so'rovda minglab telefon raqamini chiqaradi.
				r.Group(func(r chi.Router) {
					r.Use(exportLimiter.MiddlewareKey("admin-export", httpx.AdminID))
					r.Get("/export/users.csv", adminH.ExportUsers)
					r.Get("/export/elons.csv", adminH.ExportElons)
					r.Get("/export/applications.csv", adminH.ExportApplications)
				})
			})

			// Support desk — superadmin + moderator + support.
			r.Group(func(r chi.Router) {
				r.Use(httpx.RequireRole("moderator", "support"))
				r.Get("/users/{id}/avatar", adminH.GetUserAvatar)
				r.Post("/users/{id}/avatar/download", adminH.RecordAvatarDownload)
				r.Get("/feedback", fbH.ListAdmin)
				r.Patch("/feedback/{id}/resolve", fbH.Resolve)
			})

			// Superadmin only — category management, staff accounts, broadcast.
			// RequireRole() with no args admits only superadmin (always-allowed).
			r.Group(func(r chi.Router) {
				r.Use(httpx.RequireRole())
				// Avtomatik moderatsiya blokini ochish — faqat superadmin.
				r.Delete("/users/{id}/moderation-ban", adminH.LiftModerationBan)
				// Turkum yozuvlari — o'z budjeti bilan (yuqoridagi
				// catWriteLimiter izohiga qarang). O'qish (`GET
				// /categories`) bu guruhdan tashqarida: uni jadval har
				// amaldan keyin qayta so'raydi.
				r.Group(func(r chi.Router) {
					r.Use(catWriteLimiter.MiddlewareKey("admin-cat", httpx.AdminID))
					r.Patch("/categories/{id}/active", adminH.SetCategoryActive)
					r.Post("/categories", adminH.CreateCategory)
					r.Put("/categories/{id}", adminH.UpdateCategory)
					r.Delete("/categories/{id}", adminH.DeleteCategory)
				})
				r.With(catIconLimiter.MiddlewareKey("admin-cat-icon", httpx.AdminID)).
					Post("/categories/icon", adminH.UploadCategoryIcon)
				// Kadr hisoblari (Figma 3.9). O'qish cheklovsiz — jadval
				// har amaldan keyin o'zini qayta so'raydi; yozuvlar esa
				// o'z budjetida (yuqoridagi staffLimiter izohiga qarang).
				r.Get("/admins", adminH.ListAdmins)
				r.Group(func(r chi.Router) {
					r.Use(staffLimiter.MiddlewareKey("admin-staff", httpx.AdminID))
					r.Post("/admins", adminH.CreateAdmin)
					r.Patch("/admins/{id}", adminH.UpdateAdmin)
					r.Delete("/admins/{id}", adminH.DeleteAdmin)
				})
				// Yuborish — o'z budjeti bilan (yuqoridagi broadcastLimiter
				// izohiga qarang). Tarix o'qish va bekor qilish ataylab
				// cheklanmagan: biri o'qish, ikkinchisi to'xtatuvchi amal.
				r.With(broadcastLimiter.MiddlewareKey("admin-broadcast", httpx.AdminID)).
					Post("/broadcast", adminH.Broadcast)
				// Segment ro'yxati — o'z budjeti bilan (yuqoridagi
				// broadcastSegmentLimiter izohiga qarang). Bu yerda,
				// superadmin guruhida: viloyatlar bo'yicha sanoq —
				// foydalanuvchi bazasi haqidagi ma'lumot.
				r.With(broadcastSegmentLimiter.MiddlewareKey("admin-broadcast-segment", httpx.AdminID)).
					Get("/broadcast/regions", adminH.BroadcastRegions)
				r.Get("/broadcasts", adminH.ListBroadcasts)
				r.Delete("/broadcasts/{id}", adminH.CancelBroadcast)
			})
		})
	})

	// Google Play review login state. Logged on every boot so an open review
	// window is never a silent condition — if this says ACTIVE outside an
	// actual submission, someone has left the switch on and it must be closed.
	// Intentionally log-only: no HTTP endpoint reports this, because
	// advertising an open window is an invitation to start guessing the code.
	if status := authH.ReviewLoginStatus(); status == "disabled" {
		log.Info("play review login", "state", status)
	} else {
		log.Warn("play review login IS NOT DISABLED", "state", status)
		errRec.Record(errlog.Event{
			Code: "review_login_enabled", Where: "cmd/api/main.go",
			Message: "Play tekshiruv kirishi ochiq: " + status,
			Origin:  errlog.OriginServer,
		})
	}

	// Dev standartlari haqiqiy serverda. Productionda bu holat boot'ni
	// to'xtatadi (config.mustValidate), shuning uchun bu yerga faqat
	// APP_ENV=dev bilan ko'tarilgan mashina tushadi — staging yoki demo.
	// O'zgaruvchi NOMLARI yoziladi, qiymatlari emas.
	if bad := cfg.InsecureDefaults(); len(bad) > 0 {
		log.Warn("insecure dev defaults are in use", "vars", strings.Join(bad, ", "), "env", cfg.AppEnv)
		errRec.Record(errlog.Event{
			Code: "insecure_default_secret", Where: "config.Load",
			Message: "APP_ENV=" + cfg.AppEnv + " · o'zgartirilmagan: " + strings.Join(bad, ", "),
			Origin:  errlog.OriginServer,
		})
	}

	srv := &http.Server{
		Addr:              cfg.HTTPAddr,
		Handler:           r,
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       60 * time.Second,
		WriteTimeout:      60 * time.Second,
		IdleTimeout:       90 * time.Second,
		MaxHeaderBytes:    32 << 10,
	}

	go func() {
		log.Info("api listening", "addr", cfg.HTTPAddr)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Error("listen", "err", err)
			os.Exit(1)
		}
	}()

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
	<-sig
	log.Info("shutting down")
	shutdownCtx, c := context.WithTimeout(context.Background(), 10*time.Second)
	defer c()
	_ = srv.Shutdown(shutdownCtx)
}
