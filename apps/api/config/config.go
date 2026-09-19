package config

import (
	"log"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	AppEnv string // "dev" | "production"

	MongoURI string
	MongoDB  string

	HTTPAddr string

	// TrustProxyHeaders: honor X-Forwarded-For for client IP / rate limiting.
	// Only enable when running behind a trusted reverse proxy.
	TrustProxyHeaders bool
	// TrustedProxyHops: how many reverse proxies sit in front of this process.
	// Selects which X-Forwarded-For element is the real client (proxies append,
	// so the client is `hops` from the right). 1 = the deploy/Caddyfile setup;
	// raise to 2 only when a CDN fronts Caddy.
	TrustedProxyHops int

	JWTAccessSecret  string
	JWTRefreshSecret string
	JWTAccessTTL     time.Duration
	JWTAdminTTL      time.Duration
	JWTRefreshTTL    time.Duration

	// AdminIdleTTL — admin sessiyasi qancha vaqt FOYDALANILMASA yopiladi.
	//
	// JWTAdminTTL dan farqi bor: u access tokenning umri (o'g'irlangan
	// token tez o'lsin uchun ataylab qisqa), bu esa "qachongacha parol va
	// 2FA qayta so'ralmaydi" degan oyna. Har bir so'rov oynani boshidan
	// boshlaydi, ya'ni ishlab turgan admin hech qachon chiqarilmaydi.
	//
	// Oyna HISOB bo'yicha yagona: veb panel va mobil admin ilovasi uni
	// bo'lishadi. Bittasida ishlash ikkinchisini ham tirik saqlaydi;
	// ikkalasiga ham tegilmasa ikkalasi birdan yopiladi.
	// Mexanizm: internal/admin/refresh.go va session.go.
	AdminIdleTTL time.Duration

	CORSOrigins []string

	AdminSeedUser string
	AdminSeedPass string

	BotSharedSecret string
	OTPLength       int
	OTPTTL          time.Duration
	OTPDevReturn    bool

	TelegramBotToken      string
	TelegramBotUsername   string
	TelegramJobsChannelID string // Empty disables automatic channel publication.
	WebBaseURL            string // Public website origin used in Telegram links.

	// ErrorAlertChatID — "3.12 · Xatoliklar" ning Kritik/Yuqori
	// ogohlantirishlari yuboriladigan Telegram chat (#dev-alerts).
	// 0 bo'lsa ogohlantirish JIMGINA o'chiq: jurnal panelda to'liq
	// ishlayveradi, faqat xabar ketmaydi. Sozlanmagani xizmatni
	// to'xtatmaydi — bu qulaylik, majburiy bog'liqlik emas.
	ErrorAlertChatID int64

	// FCMCredentialsFile — Firebase service-account JSON fayl yo'li (mobil
	// push uchun). Bo'sh bo'lsa push jimgina o'chiq: API to'liq ishlayveradi,
	// bildirishnomalar faqat in-app (polling) bo'lib qoladi.
	FCMCredentialsFile string

	// ---------------------------------------------------------------------
	// Kontent moderatsiyasi (Google Gemini). internal/moderation ga qarang.
	// ---------------------------------------------------------------------

	// GeminiAPIKey — Gemini API kaliti. FCM/S3 kabi IXTIYORIY: bo'sh bo'lsa
	// moderatsiya jimgina o'chiq va API to'liq ishlayveradi.
	// Kalit hech qachon javobda, logda yoki klientda ko'rinmaydi.
	GeminiAPIKey string
	// GeminiBaseURL — Gemini API ildizi. Faqat test/proksi uchun
	// o'zgartiriladi; bo'sh bo'lsa rasmiy manzil ishlatiladi.
	GeminiBaseURL string
	// GeminiModel — moderatsiya modeli. Bo'sh bo'lsa pkg/gemini.DefaultModel.
	GeminiModel string
	// GeminiTimeout — bitta moderatsiya chaqiruvi uchun yuqori chegara.
	// Rasm base64 holida yuborilgani uchun matnga qaraganda uzoqroq ketadi.
	GeminiTimeout time.Duration

	// ── Xatolik tahlili (AI) ─────────────────────────────────────────
	// "3.12.1 · Xatolik — batafsil" ekranidagi "Sababini aniqla" tugmasi.
	// internal/admin/errai.go ga qarang.

	// ErrorAIAPIKey — tahlil uchun Gemini kaliti. ATAYLAB alohida
	// o'zgaruvchi: moderatsiya har bir e'londa ishlaydigan yuqori hajmli
	// oqim, tahlil esa kamdan-kam va admin qo'li bilan chaqiriladi —
	// ikkalasi bitta bepul kvotani bo'lishsa, moderatsiya limitga urilib
	// qolishi mumkin. Bo'sh bo'lsa GEMINI_API_KEY ga tushadi; u ham bo'sh
	// bo'lsa tahlil jimgina o'chiq (tugma 503, panel ishlayveradi).
	ErrorAIAPIKey string
	// ErrorAIModel — bo'sh bo'lsa pkg/gemini.DefaultAnalyzeModel.
	ErrorAIModel string
	// ErrorAITimeout — bitta tahlil chaqiruvi uchun chegara. Server
	// WriteTimeout 60 s (cmd/api/main.go), shuning uchun bu qiymat undan
	// sezilarli kichik bo'lishi SHART: aks holda mijoz javob o'rniga
	// uzilgan ulanishni ko'radi.
	ErrorAITimeout time.Duration

	// ModerationMaxImageBytes — moderatsiyaga qabul qilinadigan rasm hajmi
	// chegarasi. E'lon rasmi chegarasi (8 MiB) bilan bir xil bo'lgani ma'qul.
	ModerationMaxImageBytes int64
	// ModerationEnforce — foydalanuvchi kiritgan matn avtomatik
	// tekshirilsinmi. Quyidagi oqimlarni qamraydi:
	//   POST/PATCH /api/elons   — e'lon matni va rasmlari
	//   PATCH      /api/me      — ism, bio, ko'nikmalar
	//   POST       /api/feedback — taklif/shikoyat matni
	//
	// Standart holat FALSE: shunda mavjud oqimlar bir zarracha o'zgarmaydi.
	ModerationEnforce bool
	// ModerationStrikeLimit — necha marta nomaqbul kontent yuborilgach
	// hisob bloklanadi. E'lon, profil matni va profil rasmi BITTA umumiy
	// hisobga qo'shiladi.
	// (standart 3 — internal/moderation.DefaultStrikeLimit bilan bir xil;
	// config eng quyi qatlam bo'lgani uchun u paketni import qilmaydi.)
	ModerationStrikeLimit int
	// ModerationBanDuration — avtomatik blok muddati.
	ModerationBanDuration time.Duration

	// ModerationFailClosed — moderatsiya xizmati ishlamay qolsa nima
	// qilinsin: true bo'lsa e'lon rad etiladi (fail-closed), false bo'lsa
	// tekshirilmasdan o'tkaziladi (fail-open). Xato har doim logga yoziladi.
	//
	// Standart qiymat MUHITGA bog'liq (MODERATION_FAIL_CLOSED berilmaganda):
	//   production — TRUE. Tekshirilmagan e'lon ommaga chiqib ketishi
	//     moderatsiyani umuman qo'ymaslik bilan barobar; agar tashqi xizmat
	//     ishlamasa e'lon joylash to'xtaydi, lekin nomaqbul kontent o'tmaydi.
	//   dev — FALSE. Lokal ishlashda Gemini kaliti bo'lmasligi normal;
	//     fail-closed bo'lsa dasturchi umuman e'lon joylay olmasdi.
	//
	// Productionda ATAYLAB false qilib qo'yilsa boot'da ogohlantirish
	// yoziladi (Gemini uzilishida vaqtincha ochish uchun yo'l ochiq qoladi).
	ModerationFailClosed bool
	// moderationFailClosedExplicit — qiymat env'da aniq berilganmi. Faqat
	// boot ogohlantirishi uchun.
	moderationFailClosedExplicit bool

	// AWS S3
	AWSRegion          string
	AWSAccessKeyID     string
	AWSSecretAccessKey string
	AWSS3Bucket        string
	AWSS3PublicBaseURL string

	// Local-disk upload fallback (used when AWS_S3_BUCKET is empty).
	UploadDir        string
	UploadPublicBase string

	// AccountRetentionDays — how long a deleted account is kept in its
	// soft-deleted state before it is permanently erased (see
	// internal/account/retention.go). Whatever this is set to must match the
	// number stated on /delete-account and in the privacy policy.
	AccountRetentionDays int

	// ---------------------------------------------------------------------
	// Google Play review login. See internal/auth/review.go for the whole
	// mechanism; these four values are the only way to switch it on.
	//
	// It exists because the normal login is a Telegram-bot OTP, which a Play
	// reviewer cannot complete. While enabled, ONE extra code is accepted at
	// /auth/otp/verify and resolves to ONE pre-created, sandboxed account.
	//
	// Every field defaults to "off". The code is never shipped in the mobile
	// app — it is typed by the reviewer into the normal 6-digit OTP field, so
	// there is nothing to recover from the APK.
	// ---------------------------------------------------------------------

	// ReviewLoginEnabled is the master switch. Default false, and the deploy
	// pipeline re-asserts false on every deploy, so it can only ever be on
	// through a deliberate manual act.
	ReviewLoginEnabled bool
	// ReviewLoginCode is the code the reviewer types. Exactly OTPLength digits
	// (it shares the app's OTP input) and must be generated with a CSPRNG.
	ReviewLoginCode string
	// ReviewLoginExpiresAt makes the window self-closing: once it passes, the
	// review branch goes inert even if nobody remembers to unset the flag.
	// Required whenever ReviewLoginEnabled is true.
	ReviewLoginExpiresAt time.Time
	// ReviewLoginUserID is the hex ObjectID of the pre-created review account.
	// Login resolves this exact document — it never creates or upserts one.
	ReviewLoginUserID string
}

func envStr(k, def string) string {
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

// envTime parses an RFC3339 timestamp. An unset or unparseable value yields the
// zero Time, which every caller must treat as "not configured" — never as
// "no deadline".
func envTime(k string) time.Time {
	v := strings.TrimSpace(os.Getenv(k))
	if v == "" {
		return time.Time{}
	}
	t, err := time.Parse(time.RFC3339, v)
	if err != nil {
		return time.Time{}
	}
	return t
}

func envBool(k string, def bool) bool {
	v := strings.ToLower(os.Getenv(k))
	switch v {
	case "1", "true", "yes", "y", "on":
		return true
	case "0", "false", "no", "n", "off":
		return false
	}
	return def
}

// envBoolSet — envBool bilan bir xil, lekin qiymat ANIQ berilganini ham
// qaytaradi. Standart qiymati muhitga bog'liq bo'lgan bayroqlar uchun kerak:
// "berilmagan" va "false deb berilgan" ni ajratish shart.
func envBoolSet(k string) (val bool, ok bool) {
	switch strings.ToLower(strings.TrimSpace(os.Getenv(k))) {
	case "1", "true", "yes", "y", "on":
		return true, true
	case "0", "false", "no", "n", "off":
		return false, true
	}
	return false, false
}

func envList(k, def string) []string {
	raw := strings.Split(envStr(k, def), ",")
	out := make([]string, 0, len(raw))
	for _, item := range raw {
		if item = strings.TrimSpace(item); item != "" {
			out = append(out, item)
		}
	}
	return out
}

func Load() Config {
	cfg := Config{
		AppEnv:            strings.ToLower(envStr("APP_ENV", "dev")),
		MongoURI:          envStr("MONGO_URI", "mongodb://localhost:27017"),
		MongoDB:           envStr("MONGO_DB", "ishchibormi"),
		HTTPAddr:          envStr("HTTP_ADDR", ":8080"),
		TrustProxyHeaders: envBool("TRUST_PROXY_HEADERS", false),
		TrustedProxyHops:  envInt("TRUSTED_PROXY_HOPS", 1),
		JWTAccessSecret:   envStr("JWT_ACCESS_SECRET", "dev-access-secret"),
		JWTRefreshSecret:  envStr("JWT_REFRESH_SECRET", "dev-refresh-secret"),
		// 72 soat (4320 daqiqa). Mobil ilova 401 da refresh oqimi bilan o'zi
		// yangilaydi, shuning uchun unga sezilmaydi. Web frontend refresh
		// tokenni ataylab saqlamaydi (XSS yuzasini qisqartirish uchun) — web
		// seansi shu muddat bilan tugaydi va foydalanuvchi qayta OTP orqali
		// kiradi. O'g'irlangan token ilgari 15 kun yashardi; endi ko'pi 3 kun.
		JWTAccessTTL:  time.Duration(envInt("JWT_ACCESS_TTL_MIN", 4320)) * time.Minute,
		JWTAdminTTL:   time.Duration(envInt("JWT_ADMIN_TTL_MIN", 30)) * time.Minute,
		JWTRefreshTTL: time.Duration(envInt("JWT_REFRESH_TTL_HRS", 720)) * time.Hour,
		// 72 soat = 3 kun.
		AdminIdleTTL:    time.Duration(envInt("ADMIN_IDLE_TTL_HOURS", 72)) * time.Hour,
		CORSOrigins:     envList("CORS_ORIGINS", "http://localhost:3000,http://127.0.0.1:3000"),
		AdminSeedUser:   envStr("ADMIN_SEED_USER", "admin"),
		AdminSeedPass:   envStr("ADMIN_SEED_PASS", "Admin123!"),
		BotSharedSecret: envStr("BOT_SHARED_SECRET", "dev-shared"),
		OTPLength:       envInt("OTP_LENGTH", 6),
		OTPTTL:          time.Duration(envInt("OTP_TTL_SECONDS", 180)) * time.Second,
		// Defaults to false: returning OTP codes over the API is a dev-only
		// convenience and a credential leak in production.
		OTPDevReturn:          envBool("OTP_DEV_RETURN", false),
		TelegramBotToken:      envStr("TELEGRAM_BOT_TOKEN", ""),
		TelegramBotUsername:   envStr("TELEGRAM_BOT_USERNAME", ""),
		TelegramJobsChannelID: strings.TrimSpace(envStr("TELEGRAM_JOBS_CHANNEL_ID", "")),
		WebBaseURL:            strings.TrimRight(strings.TrimSpace(envStr("WEB_BASE_URL", "https://ishchibormi.uz")), "/"),
		ErrorAlertChatID:      int64(envInt("ERROR_ALERT_CHAT_ID", 0)),

		FCMCredentialsFile: envStr("FCM_CREDENTIALS_FILE", ""),

		GeminiAPIKey:  strings.TrimSpace(envStr("GEMINI_API_KEY", "")),
		GeminiBaseURL: strings.TrimSpace(envStr("GEMINI_BASE_URL", "")),
		GeminiModel:   strings.TrimSpace(envStr("GEMINI_MODEL", "")),
		GeminiTimeout: time.Duration(envInt("GEMINI_TIMEOUT_SECONDS", 20)) * time.Second,
		// Tahlil kaliti berilmasa moderatsiya kalitiga tushadi — bitta
		// kalitli o'rnatishda ham tugma ishlab ketaveradi.
		ErrorAIAPIKey:           strings.TrimSpace(envStr("ERROR_AI_API_KEY", envStr("GEMINI_API_KEY", ""))),
		ErrorAIModel:            strings.TrimSpace(envStr("ERROR_AI_MODEL", "")),
		ErrorAITimeout:          time.Duration(envInt("ERROR_AI_TIMEOUT_SECONDS", 40)) * time.Second,
		ModerationMaxImageBytes: int64(envInt("MODERATION_MAX_IMAGE_MB", 8)) << 20,
		ModerationStrikeLimit:   envInt("MODERATION_STRIKE_LIMIT", 3),
		ModerationBanDuration:   time.Duration(envInt("MODERATION_BAN_DAYS", 730)) * 24 * time.Hour,
		// MODERATION_ELON_ENFORCE — eski nom, orqaga moslik uchun qoldirilgan
		// (bayroq endi e'londan tashqari profil va taklif/shikoyatni ham
		// qamraydi, shuning uchun MODERATION_ENFORCE afzal).
		ModerationEnforce: envBool("MODERATION_ENFORCE", envBool("MODERATION_ELON_ENFORCE", false)),
		// Quyida, AppEnv ma'lum bo'lgach hisoblanadi.
		ModerationFailClosed: false,

		AWSRegion:          envStr("AWS_REGION", "eu-central-1"),
		AWSAccessKeyID:     envStr("AWS_ACCESS_KEY_ID", ""),
		AWSSecretAccessKey: envStr("AWS_SECRET_ACCESS_KEY", ""),
		AWSS3Bucket:        envStr("AWS_S3_BUCKET", ""),
		AWSS3PublicBaseURL: envStr("AWS_S3_PUBLIC_BASE_URL", ""),

		UploadDir:        envStr("UPLOAD_DIR", "./data/uploads"),
		UploadPublicBase: envStr("UPLOAD_PUBLIC_BASE", "http://localhost:8080/uploads"),

		AccountRetentionDays: envInt("ACCOUNT_RETENTION_DAYS", 90),

		// Defaults are the "off" state; mustValidate refuses to start in
		// production if the switch is on but the guard rails are missing.
		ReviewLoginEnabled:   envBool("REVIEW_LOGIN_ENABLED", false),
		ReviewLoginCode:      strings.TrimSpace(envStr("REVIEW_LOGIN_CODE", "")),
		ReviewLoginExpiresAt: envTime("REVIEW_LOGIN_EXPIRES_AT"),
		ReviewLoginUserID:    strings.TrimSpace(envStr("REVIEW_LOGIN_USER_ID", "")),
	}
	// Gemini inline so'rov hajmini cheklaydi va base64 o'rash hajmni ~33%
	// oshiradi, shuning uchun 15 MiB dan yuqorisi baribir rad etilardi.
	// Xato sozlama tufayli boot yiqilmasin — jimgina chegaralaymiz.
	// Fail-closed standarti muhitga bog'liq — AppEnv shu yerda ma'lum.
	if v, ok := envBoolSet("MODERATION_FAIL_CLOSED"); ok {
		cfg.ModerationFailClosed = v
		cfg.moderationFailClosedExplicit = true
	} else {
		cfg.ModerationFailClosed = cfg.IsProd()
	}
	if cfg.ModerationMaxImageBytes < 1<<20 {
		cfg.ModerationMaxImageBytes = 1 << 20
	} else if cfg.ModerationMaxImageBytes > 15<<20 {
		cfg.ModerationMaxImageBytes = 15 << 20
	}
	cfg.mustValidate()
	return cfg
}

func (c Config) IsProd() bool { return c.AppEnv == "production" || c.AppEnv == "prod" }

// ModerationFailOpenInProd — productionda fail-open ATAYLAB yoqilganmi.
// main.go boot'da shuni ogohlantirish sifatida yozadi: bu holatda moderatsiya
// uzilganda tekshirilmagan e'lonlar ommaga chiqadi.
func (c Config) ModerationFailOpenInProd() bool {
	return c.IsProd() && c.moderationFailClosedExplicit && !c.ModerationFailClosed
}

// InsecureDefaults — hali ham o'rnida turgan DEV standartlarining ro'yxati
// (nomlar, qiymatlar emas). Bo'sh bo'lsa hammasi joyida.
//
// # NEGA KERAK
//
// Productionda bularning har biri boot'ni to'xtatadi (mustValidate quyida),
// ya'ni u yerda muammo yo'q. Xavf boshqa joyda: APP_ENV=dev bilan ko'tarilgan
// HAQIQIY server — staging, demo, "vaqtinchalik" mashina. U tashqaridan
// ochiq bo'ladi, standart parol bilan panelga kiriladi va hech qanday
// ogohlantirish chiqmaydi, chunki hamma tekshiruv `IsProd()` ostida.
// main.go shu ro'yxatni "3.12 · Xatoliklar" jurnaliga Kritik sifatida
// yozadi — jim ochiq eshik hech bo'lmasa ko'rinadigan bo'lsin.
//
// Sirlarning O'ZI hech qachon qaytarilmaydi — faqat o'zgaruvchi nomlari.
func (c Config) InsecureDefaults() []string {
	weak := map[string]bool{
		"": true, "dev-access-secret": true, "dev-refresh-secret": true,
		"change-me-access": true, "change-me-refresh": true,
		"dev-shared": true, "change-me-shared": true,
	}
	var out []string
	if weak[c.JWTAccessSecret] || len(c.JWTAccessSecret) < 32 {
		out = append(out, "JWT_ACCESS_SECRET")
	}
	if weak[c.JWTRefreshSecret] || len(c.JWTRefreshSecret) < 32 {
		out = append(out, "JWT_REFRESH_SECRET")
	}
	if weak[c.BotSharedSecret] || len(c.BotSharedSecret) < 32 {
		out = append(out, "BOT_SHARED_SECRET")
	}
	if c.AdminSeedPass == "Admin123!" || len(c.AdminSeedPass) < 12 {
		out = append(out, "ADMIN_SEED_PASS")
	}
	if c.OTPDevReturn {
		out = append(out, "OTP_DEV_RETURN")
	}
	return out
}

// mustValidate fails fast in production when insecure defaults are left in
// place. This prevents accidentally shipping forgeable JWTs, a default admin
// password, or OTP leakage.
func (c Config) mustValidate() {
	// Review-login validation runs in EVERY environment, not just production:
	// it is a security switch, and a dev/staging box is often reachable too.
	// With the switch off (the default) none of these can fire.
	problems := c.reviewLoginProblems()
	if problem := webBaseURLProblem(c.WebBaseURL, c.IsProd()); problem != "" {
		log.Fatal(problem)
	}

	if !c.IsProd() {
		if len(problems) > 0 {
			log.Fatalf("invalid review-login configuration:\n  - %s", strings.Join(problems, "\n  - "))
		}
		return
	}
	weak := map[string]bool{
		"": true, "dev-access-secret": true, "dev-refresh-secret": true,
		"change-me-access": true, "change-me-refresh": true,
		"dev-shared": true, "change-me-shared": true,
	}
	if weak[c.JWTAccessSecret] || len(c.JWTAccessSecret) < 32 {
		problems = append(problems, "JWT_ACCESS_SECRET must be set to a strong (>=32 char) random value")
	}
	if weak[c.JWTRefreshSecret] || len(c.JWTRefreshSecret) < 32 {
		problems = append(problems, "JWT_REFRESH_SECRET must be set to a strong (>=32 char) random value")
	}
	if c.JWTAccessSecret == c.JWTRefreshSecret {
		problems = append(problems, "JWT_ACCESS_SECRET and JWT_REFRESH_SECRET must differ")
	}
	if weak[c.BotSharedSecret] {
		problems = append(problems, "BOT_SHARED_SECRET must be set to a strong random value")
	}
	if len(c.BotSharedSecret) < 32 {
		problems = append(problems, "BOT_SHARED_SECRET must be at least 32 characters")
	}
	if c.AdminSeedPass == "Admin123!" || len(c.AdminSeedPass) < 12 {
		problems = append(problems, "ADMIN_SEED_PASS must be changed and contain at least 12 characters")
	}
	if c.OTPDevReturn {
		problems = append(problems, "OTP_DEV_RETURN must be false in production")
	}
	// Majburiy moderatsiya kalitsiz — jim yolg'on: har bir e'lon
	// tekshirilmasdan o'tib ketaveradi (fail-open) yoki hammasi rad etiladi
	// (fail-closed). Ikkalasi ham sezilmay qoladi, shuning uchun boot'da
	// to'xtatamiz.
	if c.ModerationEnforce && c.GeminiAPIKey == "" {
		problems = append(problems, "GEMINI_API_KEY must be set when MODERATION_ENFORCE=true")
	}
	if c.JWTAdminTTL < 5*time.Minute || c.JWTAdminTTL > time.Hour {
		problems = append(problems, "JWT_ADMIN_TTL_MIN must be between 5 and 60 minutes")
	}
	// Yuqori chegara sessiyaning qat'iy umri bilan bir xil (30 kun): undan
	// katta oyna hech qachon ishga tushmasdi, ya'ni jimgina yolg'on sozlama
	// bo'lardi. Quyi chegara bir soat — undan qisqasi kun bo'yi ishlaydigan
	// adminni bekorga chiqarib yuborardi.
	if c.AdminIdleTTL < time.Hour || c.AdminIdleTTL > 30*24*time.Hour {
		problems = append(problems, "ADMIN_IDLE_TTL_HOURS must be between 1 and 720 hours")
	}
	if c.OTPLength < 6 || c.OTPLength > 8 || c.OTPTTL < time.Minute || c.OTPTTL > 10*time.Minute {
		problems = append(problems, "OTP_LENGTH/OTP_TTL_SECONDS must stay within 6-8 digits and 60-600 seconds")
	}
	if len(c.CORSOrigins) == 0 {
		problems = append(problems, "CORS_ORIGINS must contain at least one trusted HTTPS origin")
	}
	// A wrong hop count silently mis-identifies the client: too high buckets
	// every visitor together (mass 429s), too low reads an attacker-supplied
	// X-Forwarded-For element and removes rate limiting altogether.
	if c.TrustProxyHeaders && (c.TrustedProxyHops < 1 || c.TrustedProxyHops > 4) {
		problems = append(problems, "TRUSTED_PROXY_HOPS must be between 1 and 4 when TRUST_PROXY_HEADERS=true")
	}
	for _, origin := range c.CORSOrigins {
		u, err := url.Parse(origin)
		if err != nil || u.Scheme != "https" || u.Host == "" || u.Path != "" || u.RawQuery != "" || u.Fragment != "" {
			problems = append(problems, "CORS_ORIGINS must contain exact HTTPS origins only (no wildcard/path): "+origin)
		}
	}
	if c.AWSS3Bucket == "" {
		u, err := url.Parse(c.UploadPublicBase)
		if err != nil || u.Scheme != "https" || u.Host == "" {
			problems = append(problems, "UPLOAD_PUBLIC_BASE must be an HTTPS URL in production when local storage is used")
		}
	} else if c.AWSS3PublicBaseURL != "" {
		u, err := url.Parse(c.AWSS3PublicBaseURL)
		if err != nil || u.Scheme != "https" || u.Host == "" {
			problems = append(problems, "AWS_S3_PUBLIC_BASE_URL must be an HTTPS URL in production")
		}
	}
	if len(problems) > 0 {
		log.Fatalf("insecure configuration for production:\n  - %s", strings.Join(problems, "\n  - "))
	}
}

// MaxReviewLoginWindow caps how far ahead REVIEW_LOGIN_EXPIRES_AT may be set.
// A Play review takes days, not months; anything longer is a standing backdoor.
const MaxReviewLoginWindow = 30 * 24 * time.Hour

// reviewLoginProblems validates the Play-review switch. It returns nothing at
// all unless ReviewLoginEnabled is true, so the default configuration can never
// trip it.
//
// Note what is deliberately NOT an error here: an expiry that has already
// passed. Making that fatal would take production down the moment the review
// window lapsed. Instead the gate goes inert at request time — see
// internal/auth/review.go. Boot only refuses configurations that would open a
// window with no way of closing itself.
func (c Config) reviewLoginProblems() []string {
	if !c.ReviewLoginEnabled {
		return nil
	}
	var problems []string

	switch {
	case c.ReviewLoginCode == "":
		problems = append(problems, "REVIEW_LOGIN_CODE must be set when REVIEW_LOGIN_ENABLED=true")
	case len(c.ReviewLoginCode) != c.OTPLength:
		// The reviewer types this into the app's normal OTP box, which accepts
		// exactly OTPLength digits — a different length simply cannot be entered.
		problems = append(problems, "REVIEW_LOGIN_CODE must be exactly "+strconv.Itoa(c.OTPLength)+" digits (it is typed into the app's OTP field)")
	case !isAllDigits(c.ReviewLoginCode):
		problems = append(problems, "REVIEW_LOGIN_CODE must contain digits only")
	case isWeakNumericCode(c.ReviewLoginCode):
		problems = append(problems, "REVIEW_LOGIN_CODE is trivially guessable (repeated or sequential digits) — generate it with a CSPRNG")
	}

	switch {
	case c.ReviewLoginExpiresAt.IsZero():
		problems = append(problems, "REVIEW_LOGIN_EXPIRES_AT must be set (RFC3339) when REVIEW_LOGIN_ENABLED=true — the window has to close by itself")
	case time.Until(c.ReviewLoginExpiresAt) > MaxReviewLoginWindow:
		problems = append(problems, "REVIEW_LOGIN_EXPIRES_AT is more than 30 days away — shorten it")
	}

	if !isObjectIDHex(c.ReviewLoginUserID) {
		problems = append(problems, "REVIEW_LOGIN_USER_ID must be the 24-char hex id of the pre-created review account")
	}
	return problems
}

func isAllDigits(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

// isWeakNumericCode rejects the codes a human picks when told "make one up":
// every digit the same (000000) or a run of consecutive digits (123456/654321).
func isWeakNumericCode(s string) bool {
	if len(s) < 2 {
		return true
	}
	same, asc, desc := true, true, true
	for i := 1; i < len(s); i++ {
		if s[i] != s[0] {
			same = false
		}
		if s[i] != s[i-1]+1 {
			asc = false
		}
		if s[i] != s[i-1]-1 {
			desc = false
		}
	}
	return same || asc || desc
}

// isObjectIDHex reports whether s looks like a Mongo ObjectID, without pulling
// the driver into the config package.
func isObjectIDHex(s string) bool {
	if len(s) != 24 {
		return false
	}
	for _, r := range s {
		switch {
		case r >= '0' && r <= '9', r >= 'a' && r <= 'f', r >= 'A' && r <= 'F':
		default:
			return false
		}
	}
	return true
}
