package auth

import (
	"context"
	"errors"
	"log"
	"net/http"
	"time"

	"github.com/ishchibormi/backend/config"
	"github.com/ishchibormi/backend/internal/models"
	"github.com/ishchibormi/backend/internal/moderation"
	"github.com/ishchibormi/backend/pkg/httpx"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

type Handler struct {
	cfg    config.Config
	otp    *OTPRepo
	users  *mongo.Collection
	review *reviewGate
	// strikes — moderatsiya buzilishlari hisobi. Login oqimi undan telefon
	// bo'yicha blokni tekshiradi.
	strikes *moderation.StrikeStore
}

func NewHandler(cfg config.Config, db *mongo.Database) *Handler {
	return &Handler{
		cfg:     cfg,
		otp:     NewOTPRepo(db, cfg.OTPTTL, cfg.OTPLength),
		users:   db.Collection("users"),
		review:  newReviewGate(cfg),
		strikes: moderation.NewStrikeStore(db, cfg.ModerationStrikeLimit, cfg.ModerationBanDuration),
	}
}

// ReviewLoginStatus renders the Play-review switch for the startup banner. It
// never contains the code. Deliberately not exposed over HTTP: telling the
// world that a review window is open is an invitation to start guessing.
func (h *Handler) ReviewLoginStatus() string { return h.review.describe(time.Now()) }

// Users exposes the users collection for wiring auth middleware in main.
func (h *Handler) Users() *mongo.Collection { return h.users }

type requestOTPResp struct {
	TGToken  string `json:"tgToken"`
	BotURL   string `json:"botUrl"`
	DevCode  string `json:"devCode,omitempty"`
	DevPhone string `json:"devPhone,omitempty"`
}

func (h *Handler) RequestOTP(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	tok, err := h.otp.RequestToken(ctx)
	if err != nil {
		httpx.Err(w, err)
		return
	}
	resp := requestOTPResp{TGToken: tok}
	// Username sozlanmagan bo'lsa bo'sh qoldiramiz — aks holda "https://t.me/?start="
	// kabi buzuq havola qaytadi va frontend zaxira havolasi (NEXT_PUBLIC_BOT_USERNAME)
	// hech qachon ishlamaydi.
	if h.cfg.TelegramBotUsername != "" {
		resp.BotURL = "https://t.me/" + h.cfg.TelegramBotUsername + "?start=" + tok
	}
	httpx.JSON(w, 200, resp)
}

type verifyReq struct {
	Token string `json:"token"`
	Phone string `json:"phone"`
	Code  string `json:"code" validate:"required"`
}

type verifyResp struct {
	AccessToken  string      `json:"accessToken"`
	RefreshToken string      `json:"refreshToken"`
	User         models.User `json:"user"`
}

func (h *Handler) VerifyOTP(w http.ResponseWriter, r *http.Request) {
	var req verifyReq
	if err := httpx.Decode(r, &req); err != nil {
		httpx.Err(w, err)
		return
	}
	if len(req.Code) != h.cfg.OTPLength || !isAllDigits(req.Code) ||
		len(req.Token) > 64 || len(req.Phone) > 32 {
		httpx.Err(w, httpx.NewError(400, "bad_request", "invalid verification input"))
		return
	}
	ctx := r.Context()
	var (
		phone string
		tgID  int64
		err   error
	)
	// SECURITY: verification MUST be bound to a specific OTP record — either the
	// deep-link token (normal flow) or the user's own phone number. The previous
	// "match by code only" fallback let an attacker brute-force the 6-digit space
	// against EVERY active code in the database and log in as an arbitrary user
	// (account takeover). That path has been removed.
	switch {
	case req.Token != "":
		phone, tgID, err = h.otp.VerifyByToken(ctx, req.Token, req.Code)
	case req.Phone != "":
		phone, tgID, err = h.otp.VerifyByPhone(ctx, req.Phone, req.Code)
	default:
		httpx.Err(w, httpx.NewError(400, "bad_request", "token or phone required"))
		return
	}
	if err != nil {
		// The normal Telegram path has failed. Only now — and only while a Play
		// review window is open — is the review code considered. With the switch
		// off (the default) tryReviewLogin returns immediately and this endpoint
		// behaves exactly as it did before the feature existed.
		if ru := h.tryReviewLogin(r, req); ru != nil {
			h.issueSession(w, ru)
			return
		}
		httpx.Err(w, httpx.NewError(401, "invalid_code", "invalid or expired code"))
		return
	}
	if phone == "" {
		httpx.Err(w, httpx.NewError(401, "no_phone_bound", "bot has not bound phone yet"))
		return
	}
	// Telefon bo'yicha moderatsiya bloki — hisob o'chirilib qayta ro'yxatdan
	// o'tilgan bo'lsa ham ushlaydi (yozuv alohida kolleksiyada, telefon
	// kaliti bilan turadi).
	if until, banned, berr := h.strikes.BanByPhone(ctx, phone); berr == nil && banned {
		httpx.Err(w, httpx.NewErrorWithDetails(403, "account_banned",
			moderation.BanMessage(until), map[string]any{"bannedUntil": until}))
		return
	}
	user, err := h.upsertUser(ctx, phone, tgID, httpx.ClientPlatform(r), httpx.ClientDevice(r))
	if err != nil {
		// A distinct code from account_disabled: the clients treat that one as
		// "your session was revoked" and tear down local state, which is the
		// wrong reaction on a login screen where there is no session yet.
		if errors.Is(err, errAccountBlocked) {
			httpx.Err(w, httpx.NewError(403, "account_blocked", "account is blocked"))
			return
		}
		httpx.Err(w, err)
		return
	}
	h.issueSession(w, user)
}

// issueSession mints the access/refresh pair and writes the login response.
// Both the Telegram path and the review path go through here, so a review
// session is an ordinary user session in every respect — same claims, same
// secrets, same TTLs, no extra privilege of any kind.
func (h *Handler) issueSession(w http.ResponseWriter, user *models.User) {
	access, err := httpx.IssueUserSessionToken(h.cfg.JWTAccessSecret, user.ID.Hex(), user.SessionVersion, h.cfg.JWTAccessTTL)
	if err != nil {
		httpx.Err(w, err)
		return
	}
	refresh, err := httpx.IssueUserSessionToken(h.cfg.JWTRefreshSecret, user.ID.Hex(), user.SessionVersion, h.cfg.JWTRefreshTTL)
	if err != nil {
		httpx.Err(w, err)
		return
	}
	httpx.JSON(w, 200, verifyResp{AccessToken: access, RefreshToken: refresh, User: *user})
}

// tryReviewLogin resolves the Google Play review account when the submitted
// code is the review code and a review window is open. It returns nil for every
// other case, including the overwhelmingly common one of the switch being off.
//
// Order matters here, and each step is a guard rather than a convenience:
//
//  1. Gate inactive (default) → return immediately. Nothing below runs, and the
//     caller falls through to the same 401 it always returned.
//  2. Token already has a phone bound by the bot → a real user mistyped their
//     real OTP. Return without comparing or charging anything, so ordinary
//     traffic can never exhaust the reviewer's budget.
//  3. This IP is out of guesses → refuse without comparing.
//  4. Constant-time compare; a miss costs one guess.
//  5. Resolve the configured account by _id. Never upsert, never create. Fail
//     closed unless it exists, is flagged as a review account, and is neither
//     blocked nor deleted — which is what makes blocking that one document an
//     instant, total revocation.
func (h *Handler) tryReviewLogin(r *http.Request, req verifyReq) *models.User {
	now := time.Now()
	if !h.review.active(now) {
		return nil
	}
	ctx := r.Context()
	if h.otp.HasLiveBoundCode(ctx, req.Token) {
		return nil
	}
	ip := httpx.ClientIP(r)
	if h.review.budgetExhausted(ip, now) {
		log.Printf("review-login: guess budget exhausted, refusing ip=%s", ip)
		return nil
	}
	if !h.review.matches(req.Code) {
		h.review.recordFailure(ip, now)
		log.Printf("review-login: wrong code from ip=%s", ip)
		return nil
	}

	var u models.User
	if err := h.users.FindOne(ctx, bson.M{"_id": h.review.userID}).Decode(&u); err != nil {
		log.Printf("review-login: correct code but account %s does not exist — refusing", h.review.userID.Hex())
		return nil
	}
	if !u.IsReviewAccount || u.IsBlocked || u.IsDeleted {
		log.Printf("review-login: correct code but account %s is not an active review account "+
			"(isReviewAccount=%t blocked=%t deleted=%t) — refusing",
			u.ID.Hex(), u.IsReviewAccount, u.IsBlocked, u.IsDeleted)
		return nil
	}
	log.Printf("review-login: SUCCESS account=%s ip=%s ua=%q", u.ID.Hex(), ip, r.UserAgent())
	return &u
}

// errAccountBlocked is returned by upsertUser when the account behind a verified
// phone number is blocked. Login has to fail loudly here: RequireActiveUser
// rejects every subsequent request anyway, so issuing tokens would only produce
// a client that "logs in" and is bounced back out a moment later.
var errAccountBlocked = errors.New("account is blocked")

// releaseDeletedIdentity detaches phone/telegramId from any soft-deleted account
// still holding them.
//
// account.softDelete releases the identity itself, but admin.DeleteUser only
// flips isDeleted — so a number deleted from the admin panel stays attached to a
// dead document. Without this, upsertUser's filter matched that document and
// login returned a deleted account: the client stored its tokens, navigated
// inside, and the next API call came back 403 account_disabled, which the
// clients (correctly) treat as "session revoked" and route back to login. The
// user saw an endless login → home → login bounce.
//
// Detaching also keeps the insert below from colliding with the unique-sparse
// indexes on phone and telegramId.
func (h *Handler) releaseDeletedIdentity(ctx context.Context, phone string, tgID int64) error {
	or := []bson.M{{"phone": phone}}
	if tgID != 0 {
		or = append(or, bson.M{"telegramId": tgID})
	}
	cur, err := h.users.Find(ctx, bson.M{"isDeleted": true, "$or": or})
	if err != nil {
		return err
	}
	defer func() { _ = cur.Close(ctx) }()

	now := time.Now()
	for cur.Next(ctx) {
		var u models.User
		if err := cur.Decode(&u); err != nil {
			continue
		}
		// Copied to deleted* so support can still trace the account, mirroring
		// what account.softDelete records.
		set := bson.M{"updatedAt": now}
		if u.Phone != "" {
			set["deletedPhone"] = u.Phone
		}
		if u.TelegramID != 0 {
			set["deletedTelegramId"] = u.TelegramID
		}
		if _, err := h.users.UpdateOne(ctx,
			bson.M{"_id": u.ID},
			bson.M{"$set": set, "$unset": bson.M{"phone": "", "telegramId": ""}},
		); err != nil {
			return err
		}
	}
	return cur.Err()
}

// upsertUser telefon bo'yicha hisobni topadi yoki yaratadi.
//
// platform — so'rov qaysi klientdan kelgani (httpx.ClientPlatform). Bo'sh
// bo'lishi mumkin: sarlavha yubormaydigan eski klient. Bo'sh qiymat hech
// qachon YOZILMAYDI — aks holda mavjud yozuvni "noma'lum" bilan ustidan
// bosib ketardi.
//
// device — veb so'rov qaysi qurilma OS'idan kelgani (httpx.ClientDevice).
// Xuddi shu qoida: bo'sh bo'lsa yozilmaydi. Mobil ilova uchun u doim bo'sh,
// chunki u yerda platformaning o'zi qurilma OS'i.
func (h *Handler) upsertUser(ctx context.Context, phone string, tgID int64, platform, device string) (*models.User, error) {
	now := time.Now()
	if err := h.releaseDeletedIdentity(ctx, phone, tgID); err != nil {
		return nil, err
	}
	// isDeleted is part of the filter as a belt-and-braces guard: even if a
	// deleted document somehow still holds this number, it is never revived —
	// the upsert inserts a fresh account instead.
	filter := bson.M{"phone": phone, "isDeleted": bson.M{"$ne": true}}
	update := bson.M{
		"$setOnInsert": bson.M{
			"createdAt":           now,
			"firstName":           "",
			"lastName":            "",
			"rating":              0.0,
			"reviewsCount":        0,
			"completedJobsCount":  0,
			"langPref":            "latin",
			"themePref":           "light",
			"isBlocked":           false,
			"isDeleted":           false,
			"onboardingCompleted": false,
		},
		"$set": bson.M{
			"phone":           phone,
			"telegramId":      tgID,
			"isPhoneVerified": true,
			"updatedAt":       now,
		},
	}
	// Platforma. Login — signupPlatform ni yozish uchun YAGONA imkoniyat:
	// hujjat aynan shu yerda tug'iladi, keyin uni to'ldirish "qayerdan
	// ro'yxatdan o'tgan" degan savolga taxmin bilan javob berish bo'lardi.
	if platform != "" {
		update["$setOnInsert"].(bson.M)["signupPlatform"] = platform
		update["$set"].(bson.M)["lastPlatform"] = platform
		update["$set"].(bson.M)["lastSeenAt"] = now
	}
	// Qurilma — platforma bilan bir xil qoida bo'yicha. Alohida `if`:
	// platforma aniq, qurilma noaniq bo'lishi mumkin (sarlavhada "web"
	// yozilgan, UA esa brauzernikiga o'xshamaydi), va bunda platformani
	// yozmay qo'yish noto'g'ri bo'lardi.
	if device != "" {
		update["$setOnInsert"].(bson.M)["signupDevice"] = device
		update["$set"].(bson.M)["lastDevice"] = device
	}
	opts := options.FindOneAndUpdate().SetUpsert(true).SetReturnDocument(options.After)
	var u models.User
	if err := h.users.FindOneAndUpdate(ctx, filter, update, opts).Decode(&u); err != nil {
		return nil, err
	}
	if u.IsBlocked {
		return nil, errAccountBlocked
	}
	return &u, nil
}

type refreshReq struct {
	RefreshToken string `json:"refreshToken"`
}

func (h *Handler) Refresh(w http.ResponseWriter, r *http.Request) {
	var req refreshReq
	if err := httpx.Decode(r, &req); err != nil {
		httpx.Err(w, err)
		return
	}
	if len(req.RefreshToken) < 32 || len(req.RefreshToken) > 4096 {
		httpx.Err(w, httpx.NewError(401, "bad_refresh", "invalid refresh token"))
		return
	}
	uid, sessionVersion, err := httpx.ParseUserSessionToken(h.cfg.JWTRefreshSecret, req.RefreshToken)
	if err != nil {
		httpx.Err(w, httpx.NewError(401, "bad_refresh", "invalid refresh token"))
		return
	}
	oid, err := primitive.ObjectIDFromHex(uid)
	if err != nil {
		httpx.Err(w, httpx.NewError(401, "bad_refresh", "invalid refresh token"))
		return
	}
	var u models.User
	if err := h.users.FindOne(r.Context(), bson.M{"_id": oid}).Decode(&u); err != nil || u.IsBlocked || u.IsDeleted ||
		u.SessionVersion != sessionVersion {
		httpx.Err(w, httpx.NewError(401, "session_revoked", "account session revoked"))
		return
	}
	access, err := httpx.IssueUserSessionToken(h.cfg.JWTAccessSecret, uid, u.SessionVersion, h.cfg.JWTAccessTTL)
	if err != nil {
		httpx.Err(w, err)
		return
	}
	httpx.JSON(w, 200, map[string]string{"accessToken": access})
}

// RevokeSessions — "barcha qurilmalardan chiqish". Sessiya versiyasini oshiradi:
// shu paytgacha chiqarilgan barcha access/refresh tokenlar (o'g'irlangani ham)
// darhol yaroqsiz bo'ladi. Chaqirgan qurilma chiqib ketmasligi uchun unga yangi
// versiyadagi juftlik qaytariladi — javob shakli login bilan bir xil.
func (h *Handler) RevokeSessions(w http.ResponseWriter, r *http.Request) {
	oid, err := primitive.ObjectIDFromHex(httpx.UserID(r))
	if err != nil {
		httpx.Err(w, httpx.NewError(401, "bad_token", "bad user id"))
		return
	}
	var u models.User
	err = h.users.FindOneAndUpdate(r.Context(),
		bson.M{"_id": oid, "isDeleted": bson.M{"$ne": true}},
		bson.M{"$inc": bson.M{"sessionVersion": 1}},
		options.FindOneAndUpdate().SetReturnDocument(options.After)).Decode(&u)
	if err != nil {
		httpx.Err(w, httpx.NewError(401, "no_account", "account not found"))
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	h.issueSession(w, &u)
}

func isAllDigits(s string) bool {
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return s != ""
}

// DevPeekOTP — dev-only endpoint that returns the most recent OTP for a token.
// Guarded by config.OTPDevReturn.
func (h *Handler) DevPeekOTP(w http.ResponseWriter, r *http.Request) {
	if !h.cfg.OTPDevReturn {
		httpx.Err(w, httpx.NewError(404, "not_found", "dev peek disabled"))
		return
	}
	tok := r.URL.Query().Get("token")
	if tok == "" {
		httpx.Err(w, httpx.NewError(400, "bad_request", "token required"))
		return
	}
	doc, err := h.otp.LatestForToken(r.Context(), tok)
	if err != nil {
		httpx.JSON(w, 200, map[string]any{"code": "", "phone": "", "telegramId": 0})
		return
	}
	httpx.JSON(w, 200, map[string]any{
		"code":       doc.Code,
		"phone":      doc.Phone,
		"telegramId": doc.TelegramID,
	})
}

// RequireActiveUser is middleware that runs AFTER httpx.UserAuth. It rejects
// requests from accounts that have been blocked or soft-deleted, so an admin's
// block/delete actually revokes API access instead of only hiding the user.
func RequireActiveUser(users *mongo.Collection) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			oid, err := primitive.ObjectIDFromHex(httpx.UserID(r))
			if err != nil {
				httpx.Err(w, httpx.NewError(401, "bad_token", "bad user id"))
				return
			}
			var u models.User
			err = users.FindOne(r.Context(), bson.M{"_id": oid}).Decode(&u)
			if err != nil {
				httpx.Err(w, httpx.NewError(401, "no_account", "account not found"))
				return
			}
			if u.IsBlocked || u.IsDeleted {
				httpx.Err(w, httpx.NewError(403, "account_disabled", "account is blocked or deleted"))
				return
			}
			// Sessiyalar bekor qilingan (versiya oshgan) — eski token yaroqsiz.
			// 401: klient refresh'ga uriniladi, u ham session_revoked qaytaradi.
			if u.SessionVersion != httpx.UserSessionVersion(r) {
				httpx.Err(w, httpx.NewError(401, "session_revoked", "account session revoked"))
				return
			}
			// Avtomatik moderatsiya bloki — mavjud seansni darhol to'xtatadi.
			// Muddati o'tgach o'z-o'zidan kuchini yo'qotadi, hech kim qo'lda
			// ochishi shart emas.
			if u.ModerationBannedUntil != nil && u.ModerationBannedUntil.After(time.Now()) {
				httpx.Err(w, httpx.NewErrorWithDetails(403, "account_banned",
					moderation.BanMessage(*u.ModerationBannedUntil),
					map[string]any{"bannedUntil": *u.ModerationBannedUntil}))
				return
			}
			// Hujjat allaqachon o'qilgan — oxirgi ishlatilgan platformani
			// shu yerda yozib qo'yish qo'shimcha so'rov talab qilmaydi.
			touchPlatform(r, users, &u)
			// The user document is already loaded here, so tagging the request
			// as review traffic is free. Downstream handlers use it to keep the
			// review account's activity inside its sandbox.
			if u.IsReviewAccount {
				r = r.WithContext(context.WithValue(r.Context(), httpx.CtxReviewActor, true))
			}
			next.ServeHTTP(w, r)
		})
	}
}

// platformSeenInterval — lastSeenAt qancha eskirganda qayta yoziladi.
//
// Har so'rovda yozish mumkin emas: feed'ni aylantirayotgan bitta
// foydalanuvchi daqiqada o'nlab yozuv hosil qilardi va bu hech qanday yangi
// ma'lumot bermasdi. 30 daqiqa "bugun faol edi" degan savolga javob berish
// uchun yetarlicha aniq.
const platformSeenInterval = 30 * time.Minute

// touchPlatform foydalanuvchining oxirgi ishlatgan klientini yangilaydi.
//
// Yozuv TO'RTTA holatda bo'ladi va boshqa hech qachon:
//   - klient o'zgargan (ilovadan vebga o'tgan) — bu darhol qiziq;
//   - qurilma o'zgargan (stol brauzeridan telefonga o'tgan) — xuddi shunday;
//   - lastSeenAt yo'q (bu funksiyadan oldingi eski hisob);
//   - lastSeenAt [platformSeenInterval] dan eski.
//
// Yozuv so'rov kontekstida, sinxron bajariladi. Bu javobni sezilarli
// kechiktirmaydi, chunki u foydalanuvchi boshiga yarim soatda bir marta
// sodir bo'ladi. Xato jimgina yutiladi: statistika yozuvi tufayli
// foydalanuvchining haqiqiy so'rovi yiqilishi mumkin emas.
func touchPlatform(r *http.Request, users *mongo.Collection, u *models.User) {
	p := httpx.ClientPlatform(r)
	if p == "" {
		return // klient o'zini tanitmadi — mavjud yozuvga tegmaymiz
	}
	d := httpx.ClientDevice(r)
	now := time.Now()
	fresh := u.LastSeenAt != nil && now.Sub(*u.LastSeenAt) < platformSeenInterval
	// Bo'sh `d` — "aniqlay olmadik", "o'zgardi" EMAS. Shuning uchun u
	// yangilanishni qo'zg'atmaydi va quyida yozilmaydi ham.
	if p == u.LastPlatform && (d == "" || d == u.LastDevice) && fresh {
		return
	}
	set := bson.M{"lastPlatform": p, "lastSeenAt": now}
	if d != "" {
		set["lastDevice"] = d
	}
	// Eski hisoblarda signupPlatform/signupDevice bo'sh. Ularni SHU YERDA
	// to'ldirmaymiz: hozir ishlatayotgan klient ro'yxatdan o'tgan klient
	// degani emas, va taxmin qilingan qiymat hisobotda haqiqiydan farq
	// qilmay qolardi.
	//
	// Ilovaga o'tilganda eski `lastDevice` ham o'chirilmaydi: qurilma
	// qiymati faqat platforma "web" bo'lganda o'qiladi (platformLabel'ga
	// qarang), shuning uchun eskirgan qiymat hech qayerda ko'rinmaydi —
	// va foydalanuvchi vebga qaytsa, u qayta aniqlanadi.
	if _, err := users.UpdateOne(r.Context(), bson.M{"_id": u.ID}, bson.M{"$set": set}); err != nil {
		return
	}
	u.LastPlatform, u.LastSeenAt = p, &now
	if d != "" {
		u.LastDevice = d
	}
}

// DenyReviewAccount blocks routes the sandboxed Play review account has no
// business calling. Everything a reviewer must actually exercise to assess the
// app — posting a job, applying to one, editing their profile, browsing,
// notifications — stays open; this covers only the actions that would either
// touch a real person or leave real-world residue:
//
//	POST/DELETE /uploads          arbitrary files on our public CDN
//	POST /me/delete/request|confirm  would destroy the review account mid-review
//	POST /reports                 lands in the admins' moderation queue
//	POST /feedback                relays to the support Telegram bot
//	POST/DELETE /users/{id}/block affects a real user's visibility
//
// Note there is no route to change a phone number: updateMeReq has no phone
// field, so that is already impossible for every account, review or not.
//
// It reads the flag RequireActiveUser set, so it must be mounted after it.
func DenyReviewAccount(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if httpx.IsReviewActor(r.Context()) {
			// 403 with a neutral message. A real user can never see this, and
			// it tells a reviewer plainly that the demo account is limited
			// rather than that the app is broken.
			httpx.Err(w, httpx.NewError(403, "not_available",
				"This action is not available on the demo account."))
			return
		}
		next.ServeHTTP(w, r)
	})
}

// Used by /api/me to expand the current user object.
func LoadUser(ctx context.Context, users *mongo.Collection, idHex string) (*models.User, error) {
	oid, err := primitive.ObjectIDFromHex(idHex)
	if err != nil {
		return nil, httpx.NewError(401, "bad_token", "bad user id")
	}
	var u models.User
	if err := users.FindOne(ctx, bson.M{"_id": oid}).Decode(&u); err != nil {
		return nil, httpx.NewError(404, "not_found", "user not found")
	}
	return &u, nil
}
