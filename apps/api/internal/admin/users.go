package admin

import (
	"context"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/ishchibormi/backend/internal/models"
	"github.com/ishchibormi/backend/internal/moderation"
	"github.com/ishchibormi/backend/internal/upload"
	"github.com/ishchibormi/backend/pkg/httpx"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// usersFilter builds the Mongo query shared by ListUsers and the users CSV
// export. Params: q (name/phone), region, blocked=1|0, deleted=hide|only,
// platform=web|android|ios|unknown.
func usersFilter(q url.Values) bson.M {
	// O'chirilganlar ham ko'rinadi — `?deleted=` bilan ajratiladi.
	// Nega standart shunday: deletemode.go izohiga qarang.
	filter := bson.M{}
	applyDeletedFilter(filter, q.Get("deleted"))
	if s := strings.TrimSpace(q.Get("q")); s != "" {
		rx := bson.M{"$regex": escRe(s), "$options": "i"}
		filter["$or"] = bson.A{
			bson.M{"firstName": rx}, bson.M{"lastName": rx}, bson.M{"phone": rx},
			// O'chirilgan hisobda raqam `phone` dan `deletedPhone` ga ko'chadi
			// (identifikatorni bo'shatish uchun). Uni qidiruvga qo'shmasak,
			// admin "bu raqam nima bo'ldi?" degan savolga javob topa olmasdi —
			// hisob ro'yxatda turadi-yu, raqami bo'yicha topilmaydi.
			bson.M{"deletedPhone": rx},
		}
	}
	if region := strings.TrimSpace(q.Get("region")); region != "" {
		filter["region"] = region
	}
	// "Bloklangan" filtri IKKALA blokni ham qamraydi.
	//
	// Ilgari faqat `isBlocked` ga qarardi, ya'ni nomaqbul kontent uchun
	// avtomatik bloklangan foydalanuvchilar "Faol" ro'yxatida chiqardi —
	// admin uchun bu shunchaki noto'g'ri javob edi. Panelda blok bitta
	// tushuncha, demak filtr ham bitta bo'lishi kerak.
	activeBan := bson.M{"moderationBannedUntil": bson.M{"$gt": time.Now()}}
	switch q.Get("blocked") {
	case "1":
		filter["$and"] = append(andOf(filter), bson.M{"$or": bson.A{
			bson.M{"isBlocked": true}, activeBan,
		}})
	case "0":
		filter["$and"] = append(andOf(filter), bson.M{
			"isBlocked": bson.M{"$ne": true},
			"$nor":      bson.A{activeBan},
		})
	}
	// Platforma filtri — oxirgi ishlatilgan klient bo'yicha.
	//
	// "unknown" ALOHIDA qiymat, "hech qanday filtr yo'q" emas: sarlavha
	// yubormaydigan eski klientlarni ataylab ko'rib chiqish kerak bo'lishi
	// mumkin (masalan, ilovaning eski versiyasi qancha qolganini bilish).
	switch p := q.Get("platform"); p {
	case httpx.PlatformWeb, httpx.PlatformAndroid, httpx.PlatformIOS, httpx.PlatformTelegram:
		filter["lastPlatform"] = p
	case httpx.PlatformUnknown:
		filter["lastPlatform"] = bson.M{"$in": bson.A{nil, ""}}
	}
	return filter
}

// andOf — filtrdagi mavjud `$and` ro'yxati (yo'q bo'lsa bo'sh).
//
// Kerak, chunki qidiruv `$or` dan foydalanadi va blok filtri ham `$or`
// qo'shadi: ikkalasini bitta hujjatga yozsak, biri ikkinchisini bosib
// ketardi va qidiruv jimgina ishlamay qolardi.
func andOf(filter bson.M) bson.A {
	if existing, ok := filter["$and"].(bson.A); ok {
		return existing
	}
	return bson.A{}
}

// ListUsers: paginated + searchable + filterable. Query params:
//
//	page, limit, q (name/phone), region, blocked=1|0, deleted=hide|only,
//	platform=web|android|ios|unknown
func (h *Handler) ListUsers(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	page, limit, skip := pageParams(r)
	filter := usersFilter(r.URL.Query())

	cur, err := h.Users.Find(ctx, filter,
		options.Find().SetSort(bson.D{{Key: "createdAt", Value: -1}}).SetSkip(skip).SetLimit(int64(limit)))
	if err != nil {
		httpx.Err(w, err)
		return
	}
	defer cur.Close(ctx)
	out := []models.User{}
	for cur.Next(ctx) {
		var u models.User
		if err := cur.Decode(&u); err == nil {
			out = append(out, u)
		}
	}
	total, _ := h.Users.CountDocuments(ctx, filter)
	paged(w, out, page, limit, total)
}

// GetUser returns a single user plus their related records (elons, applications
// as worker, reviews about them, and reports filed against them) — the "batafsil
// ko'rinish" the doc asks for, in one round-trip.
func (h *Handler) GetUser(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id, err := primitive.ObjectIDFromHex(chi.URLParam(r, "id"))
	if err != nil {
		httpx.Err(w, httpx.NewError(400, "bad_id", "bad id"))
		return
	}
	var u models.User
	if err := h.Users.FindOne(ctx, bson.M{"_id": id}).Decode(&u); err != nil {
		httpx.Err(w, httpx.NewError(404, "not_found", "user not found"))
		return
	}
	elons := decodeElons(ctx, h.Elons, bson.M{"ownerId": id}, 100)
	apps := decodeApps(ctx, h.Apps, bson.M{"workerId": id}, 100)
	reports := decodeReports(ctx, h.Reports, bson.M{"targetType": "user", "targetId": id}, 100)
	// Buzilishlar tarixi — blok sababining tafsiloti.
	//
	// `user.blockReason` bitta jumla ("3 marta urinildi"), bu esa aynan
	// QAYSI urinishlar, qachon va nima ustida bo'lganini ko'rsatadi. Admin
	// qaroriga e'tiroz kelganda ("men hech narsa qilmadim") yagona dalil shu.
	// Yozuv TELEFON bo'yicha saqlanadi, ya'ni hisob o'chirilib qayta ochilgan
	// bo'lsa ham tarix joyida qoladi.
	var strikeRec *moderation.StrikeRecord
	if h.Strikes != nil {
		if rec, err := h.Strikes.FindByUser(ctx, id); err == nil && rec != nil {
			strikeRec = rec
		}
	}
	// `any` ataylab: yozuv yo'q bo'lsa javobda `null` turishi kerak, bo'sh
	// obyekt emas — panel "hech qachon qoida buzmagan" ni shu orqali biladi.
	var strikes any
	if strikeRec != nil {
		strikes = strikeRec
	}
	httpx.JSON(w, 200, map[string]any{
		"user": u, "elons": elons, "applications": apps, "reports": reports,
		"moderationStrikes": strikes,
		// Adminlar shu odamga QO'LDA yozgan xabarlar. Broadcast'lar kirmaydi
		// (ular `sentByAdminId` siz saqlanadi) — savol "men bu odamga nima
		// yozganman?", "u qanday e'lonlarni olgan?" emas.
		"adminNotifications": h.adminNotificationsFor(ctx, id),
		// Hisob holatining vaqt chizig'i — mexanizmi user_status.go da.
		// Rol faqat superadminga ko'rsatiladi (adminBrief.label izohi).
		"statusHistory": h.statusHistory(ctx, &u, strikeRec, httpx.AdminRole(r) == "superadmin"),
	})
}

// adminNotificationEntry — foydalanuvchi kartasidagi bitta xabar.
//
// models.Notification ni to'g'ridan-to'g'ri qaytarmaymiz: unda panelga
// keraksiz maydonlar bor (userId, type, relatedEntity), va aksincha,
// kerak bo'lgani yo'q — yuborgan adminning NOMI. Faqat id qaytarilsa,
// panel uni ko'rsata olmasdi va "kim yozgan?" degan savol javobsiz
// qolardi.
type adminNotificationEntry struct {
	ID        primitive.ObjectID `json:"id"`
	Title     string             `json:"title"`
	Body      string             `json:"body"`
	IsRead    bool               `json:"isRead"`
	CreatedAt time.Time          `json:"createdAt"`
	// SentBy — yuborgan adminning username'i. Admin o'chirilgan bo'lsa
	// bo'sh qoladi: xabar yozuvi qoladi, muallifi esa endi noma'lum.
	SentBy string `json:"sentBy,omitempty"`
}

// maxAdminNotifications — kartada ko'rsatiladigan xabarlar chegarasi.
// Bu tarix sahifasi emas: bitta odamga yuzlab xabar yozilgan bo'lsa,
// kartani cheksiz uzaytirishning ma'nosi yo'q.
const maxAdminNotifications = 50

// adminNotificationsFor — shu foydalanuvchiga QO'LDA yuborilgan xabarlar,
// yangisidan boshlab.
//
// Filtr `sentByAdminId` MAVJUDLIGIGA qarab ishlaydi, `type` ga emas:
// broadcast ham `type: "system"` bilan saqlanadi, ya'ni tur bo'yicha
// ajratib bo'lmaydi. Bu maydonsiz eski yozuvlar ham tushmaydi — ular
// haqiqatan qaysi yo'l bilan yuborilgani endi bilib bo'lmaydi.
func (h *Handler) adminNotificationsFor(ctx context.Context, userID primitive.ObjectID) []adminNotificationEntry {
	out := []adminNotificationEntry{}
	if h.Notify == nil || h.Notify.Col == nil {
		return out
	}
	cur, err := h.Notify.Col.Find(ctx,
		bson.M{"userId": userID, "sentByAdminId": bson.M{"$exists": true}},
		options.Find().
			SetSort(bson.D{{Key: "createdAt", Value: -1}}).
			SetLimit(maxAdminNotifications),
	)
	if err != nil {
		return out
	}
	defer cur.Close(ctx)

	rows := []models.Notification{}
	adminIDs := map[primitive.ObjectID]bool{}
	for cur.Next(ctx) {
		var n models.Notification
		if cur.Decode(&n) != nil {
			continue
		}
		rows = append(rows, n)
		if !n.SentByAdminID.IsZero() {
			adminIDs[n.SentByAdminID] = true
		}
	}

	names := h.adminNames(ctx, adminIDs)
	for _, n := range rows {
		out = append(out, adminNotificationEntry{
			ID: n.ID, Title: n.Title, Body: n.Body,
			IsRead: n.IsRead, CreatedAt: n.CreatedAt,
			SentBy: names[n.SentByAdminID],
		})
	}
	return out
}

// adminNames — id -> username.
//
// `adminBriefs` (user_status.go) ustida qurilgan: ikkovi ham bir xil
// so'rovni yuborardi, va ikkita nusxa ertaga bir-biridan farq qilib
// qolardi (masalan proyeksiya bir joyda yangilanib, ikkinchisida yo'q).
// Rol bu yerda ataylab tashlanadi — xabarlar ro'yxatida faqat "kim
// yozgan" degan savol bor.
func (h *Handler) adminNames(ctx context.Context, ids map[primitive.ObjectID]bool) map[primitive.ObjectID]string {
	out := map[primitive.ObjectID]string{}
	for id, b := range h.adminBriefs(ctx, ids) {
		out[id] = b.Username
	}
	return out
}

func (h *Handler) VerifyUser(w http.ResponseWriter, r *http.Request) {
	id, err := primitive.ObjectIDFromHex(chi.URLParam(r, "id"))
	if err != nil {
		httpx.Err(w, httpx.NewError(400, "bad_id", "bad id"))
		return
	}
	if _, err := h.Users.UpdateOne(r.Context(), bson.M{"_id": id}, bson.M{"$set": bson.M{"isPhoneVerified": true}}); err != nil {
		httpx.Err(w, err)
		return
	}
	h.audit(r, "user_verify", id.Hex(), "")
	httpx.JSON(w, 200, map[string]bool{"ok": true})
}

type notifyUserReq struct {
	Title string `json:"title" validate:"required"`
	Body  string `json:"body"`
}

// NotifyUser sends a single admin-authored notification to one user.
func (h *Handler) NotifyUser(w http.ResponseWriter, r *http.Request) {
	id, err := primitive.ObjectIDFromHex(chi.URLParam(r, "id"))
	if err != nil {
		httpx.Err(w, httpx.NewError(400, "bad_id", "bad id"))
		return
	}
	var req notifyUserReq
	if err := httpx.Decode(r, &req); err != nil {
		httpx.Err(w, err)
		return
	}
	req.Title = strings.TrimSpace(req.Title)
	req.Body = strings.TrimSpace(req.Body)
	if req.Title == "" || len([]rune(req.Title)) > 160 || len([]rune(req.Body)) > 4000 {
		httpx.Err(w, httpx.NewError(400, "bad_notification", "notification is empty or too long"))
		return
	}
	// Yuborgan adminni yozuvda qoldiramiz — shu belgi orqali bu xabar
	// keyinchalik foydalanuvchi kartasida ko'rinadi va broadcast'dan
	// ajratiladi.
	adminID, _ := primitive.ObjectIDFromHex(httpx.AdminID(r))
	h.Notify.PushFromAdmin(r.Context(), id, adminID, req.Title, req.Body)
	h.audit(r, "user_notify", id.Hex(), req.Title)
	httpx.JSON(w, 200, map[string]bool{"ok": true})
}

type setBlockReq struct {
	IsBlocked bool `json:"isBlocked"`
	// Reason — bloklashda MAJBURIY. Nega: blokni ochgan yoki e'tirozni ko'rib
	// chiqqan admin ko'pincha uni qo'ygan admin emas, va oradan oylar o'tgan
	// bo'ladi. Sababsiz blok — hech kim javob bera olmaydigan qaror.
	Reason string `json:"reason"`
}

// maxBlockReasonRunes — sabab uzunligi chegarasi. Erkin matn, lekin bu
// izohlar maydoni emas: uzun matn admin ro'yxatida ham, mobil ilovada ham
// o'qib bo'lmaydigan bo'lib qolardi.
const maxBlockReasonRunes = 500

func (h *Handler) BlockUser(w http.ResponseWriter, r *http.Request) {
	id, err := primitive.ObjectIDFromHex(chi.URLParam(r, "id"))
	if err != nil {
		httpx.Err(w, httpx.NewError(400, "bad_id", "bad id"))
		return
	}
	var req setBlockReq
	if err := httpx.Decode(r, &req); err != nil {
		httpx.Err(w, err)
		return
	}
	ctx := r.Context()
	var u models.User
	if err := h.Users.FindOne(ctx, bson.M{"_id": id}).Decode(&u); err != nil {
		httpx.Err(w, httpx.NewError(404, "not_found", "user not found"))
		return
	}
	now := time.Now()

	if req.IsBlocked {
		reason := strings.TrimSpace(req.Reason)
		if reason == "" {
			httpx.Err(w, httpx.NewError(400, "reason_required",
				"bloklash sababini yozing"))
			return
		}
		if len([]rune(reason)) > maxBlockReasonRunes {
			httpx.Err(w, httpx.NewError(400, "reason_too_long",
				"sabab juda uzun"))
			return
		}
		if err := moderation.RecordBanHistory(ctx, h.AuditCol, &u); err != nil {
			httpx.Err(w, err)
			return
		}
		set := bson.M{
			"isBlocked":   true,
			"blockReason": reason,
			"blockSource": moderation.BlockSourceAdmin,
			"blockedAt":   now,
			"blockedBy":   httpx.AdminID(r),
			"updatedAt":   now,
		}
		if _, err := h.Users.UpdateOne(ctx, bson.M{"_id": id}, bson.M{"$set": set}); err != nil {
			httpx.Err(w, err)
			return
		}
		// Keep public listing queries join-free while applying moderation instantly.
		_, _ = h.Elons.UpdateMany(ctx, bson.M{"ownerId": id},
			bson.M{"$set": bson.M{"ownerBlocked": true, "updatedAt": now}})
		h.audit(r, "user_block", id.Hex(), reason)
		httpx.JSON(w, 200, map[string]bool{"ok": true})
		return
	}

	// ── Blokni ochish ────────────────────────────────────────────────────
	//
	// Panelda blok BITTA tushuncha, shuning uchun "Blokdan chiqarish" ham
	// bitta amal bo'lishi kerak: qo'lda qo'yilgani ham, avtomatik qo'yilgani
	// ham shu yerda ochiladi. Ilgari ular ikki alohida tugma edi va admin
	// birinchisini bosib, foydalanuvchi baribir kira olmaganini ko'rardi.
	//
	// Ruxsat qoidasi o'z kuchida: avtomatik blokni faqat superadmin ocha
	// oladi (bu jazoni bekor qilish). Moderator uni ochmoqchi bo'lsa aniq
	// javob oladi — jimgina yarim ish qilinmaydi.
	moderationActive := u.ModerationBannedUntil != nil && u.ModerationBannedUntil.After(now)
	if moderationActive {
		if httpx.AdminRole(r) != "superadmin" {
			httpx.Err(w, httpx.NewError(403, "moderation_ban_superadmin_only",
				"avtomatik moderatsiya blokini faqat superadmin ocha oladi"))
			return
		}
		if h.Strikes == nil {
			httpx.Err(w, httpx.NewError(503, "moderation_disabled", "moderatsiya sozlanmagan"))
			return
		}
		// Buzilishlar hisobini ham nolga tushiradi — aks holda keyingi bitta
		// buzilish foydalanuvchini darhol qayta bloklardi.
		if err := h.Strikes.LiftBanByUser(ctx, id); err != nil {
			httpx.Err(w, err)
			return
		}
	}
	// `moderationBannedUntil` bu yerda ham tozalanadi — LiftBanByUser uni
	// allaqachon o'chirgan bo'lsa ham.
	//
	// Nega takror: javobda "muvaffaqiyatli" deyilgani bilan foydalanuvchi
	// hujjatida blok qolib ketishi mumkin bo'lgan har qanday yo'lni yopish
	// uchun. Aynan shunday holat bo'lgan: admin "blokdan chiqarildi" degan
	// xabarni ko'rar, foydalanuvchi esa baribir kira olmasdi.
	if err := moderation.RecordBanHistory(ctx, h.AuditCol, &u); err != nil {
		httpx.Err(w, err)
		return
	}
	if _, err := h.Users.UpdateOne(ctx, bson.M{"_id": id}, bson.M{
		"$set": bson.M{"isBlocked": false, "updatedAt": now},
		"$unset": bson.M{
			"moderationBannedUntil": "",
			"blockReason":           "", "blockSource": "", "blockedAt": "", "blockedBy": "",
		},
	}); err != nil {
		httpx.Err(w, err)
		return
	}
	_, _ = h.Elons.UpdateMany(ctx, bson.M{"ownerId": id},
		bson.M{"$set": bson.M{"ownerBlocked": false, "updatedAt": now}})
	detail := "unblock"
	if moderationActive {
		detail = "unblock (+moderatsiya bloki)"
	}
	h.audit(r, "user_unblock", id.Hex(), detail)
	httpx.JSON(w, 200, map[string]bool{"ok": true})
}

func (h *Handler) DeleteUser(w http.ResponseWriter, r *http.Request) {
	id, err := primitive.ObjectIDFromHex(chi.URLParam(r, "id"))
	if err != nil {
		httpx.Err(w, httpx.NewError(400, "bad_id", "bad id"))
		return
	}
	mode, err := deleteMode(r)
	if err != nil {
		httpx.Err(w, err)
		return
	}
	if mode == deleteModePurge {
		h.purgeUserAndRespond(w, r, id)
		return
	}

	// Best-effort: remove the user's avatar from S3, plus images of all their elons.
	var u models.User
	if err := h.Users.FindOne(r.Context(), bson.M{"_id": id}).Decode(&u); err == nil {
		go upload.DeleteByURL(h.Storage, u.AvatarURL)
	}
	cur, _ := h.Elons.Find(r.Context(), bson.M{"ownerId": id})
	if cur != nil {
		defer cur.Close(r.Context())
		for cur.Next(r.Context()) {
			var e models.Elon
			if err := cur.Decode(&e); err == nil {
				for _, u := range e.Images {
					go upload.DeleteByURL(h.Storage, u)
				}
			}
		}
	}
	// Release the identity, exactly as account.softDelete does for self-service
	// deletion: unsetting phone/telegramId drops this document out of the
	// unique-sparse indexes, which is what lets the number register again later.
	// Leaving them attached used to strand the user — auth.upsertUser matched
	// this dead document, so they logged in and were bounced straight back to
	// the login screen by RequireActiveUser.
	set := bson.M{"isDeleted": true, "deletedAt": time.Now()}
	if u.Phone != "" {
		set["deletedPhone"] = u.Phone
	}
	if u.TelegramID != 0 {
		set["deletedTelegramId"] = u.TelegramID
	}
	_, err = h.Users.UpdateOne(r.Context(), bson.M{"_id": id}, bson.M{
		"$set":   set,
		"$unset": bson.M{"phone": "", "telegramId": ""},
	})
	if err != nil {
		httpx.Err(w, err)
		return
	}
	now := time.Now()
	_, _ = h.Elons.UpdateMany(r.Context(), bson.M{"ownerId": id}, bson.M{"$set": bson.M{
		"isDeleted": true, "status": "cancelled", "updatedAt": now,
	}})
	live := []string{"pending", "accepted"}
	_, _ = h.Apps.UpdateMany(r.Context(), bson.M{
		"$or":    []bson.M{{"workerId": id}, {"employerId": id}},
		"status": bson.M{"$in": live},
	}, bson.M{"$set": bson.M{
		"status": "cancelled", "cancelReason": "account_deleted", "decidedAt": now,
	}})
	// O'chirilgan hisobning qurilmalariga push (masalan broadcast) ketmasin.
	_, _ = h.Users.Database().Collection("device_tokens").DeleteMany(r.Context(), bson.M{"userId": id})
	h.audit(r, "user_delete", id.Hex(), "hidden — foydalanuvchilardan yashirildi, bazada qoldi")
	httpx.JSON(w, 200, map[string]any{"ok": true, "mode": deleteModeHidden})
}

// purgeUserAndRespond — "bazadan ham o'chirish" rejimi.
//
// Butun ishni account.Purger bajaradi: hisob, e'lonlari, arizalari,
// bildirishnomalari, shikoyatlari, bir martalik kodlari va yuklangan
// fayllari. Bu yerda qayta yozilmaydi — retention oqimi bilan bir xil kod
// bo'lishi shart, aks holda "butunlay o'chirish" ikki xil ma'no kasb etardi.
func (h *Handler) purgeUserAndRespond(w http.ResponseWriter, r *http.Request, id primitive.ObjectID) {
	if h.Purger == nil {
		httpx.Err(w, httpx.NewError(503, "purge_unavailable",
			"permanent deletion is not configured on this server"))
		return
	}
	// Audit yozuvi AVVAL yoziladi. Sabab: o'chirish tugagach bu hisob haqida
	// hech narsa qolmaydi, xato yuz bersa esa nima qilinmoqchi bo'lgani
	// jurnalda turishi kerak.
	h.audit(r, "user_delete", id.Hex(), "purge — bazadan butunlay o'chirildi (qaytarib bo'lmaydi)")
	if err := h.Purger.PurgeUserNow(r.Context(), id); err != nil {
		httpx.Err(w, err)
		return
	}
	httpx.JSON(w, 200, map[string]any{"ok": true, "mode": deleteModePurge})
}

// LiftModerationBan — avtomatik moderatsiya blokini bekor qiladi.
//
// DELETE /api/admin/users/{id}/moderation-ban
//
// FAQAT SUPERADMIN: route superadmin guruhida turadi (cmd/api/main.go).
// Sabab — bu jazoni bekor qilish, ya'ni moderator yoki support xodimi
// o'zboshimchalik bilan qiladigan amal emas.
//
// Qo'lda qo'yilgan blokga (isBlocked) TEGMAYDI — u alohida tugma orqali
// boshqariladi.
func (h *Handler) LiftModerationBan(w http.ResponseWriter, r *http.Request) {
	id, err := primitive.ObjectIDFromHex(chi.URLParam(r, "id"))
	if err != nil {
		httpx.Err(w, httpx.NewError(400, "bad_id", "bad id"))
		return
	}
	if h.Strikes == nil {
		httpx.Err(w, httpx.NewError(503, "moderation_disabled", "moderatsiya sozlanmagan"))
		return
	}
	if err := h.Strikes.LiftBanByUser(r.Context(), id); err != nil {
		httpx.Err(w, err)
		return
	}
	h.audit(r, "moderation_ban_lift", id.Hex(), "")
	httpx.JSON(w, 200, map[string]bool{"ok": true})
}
