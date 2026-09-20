package admin

import (
	"context"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/ishchibormi/backend/pkg/httpx"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// staleDays — Figma 3.6a: "Ariza 3+ kundan beri «Kutilmoqda» bo'lsa" u uzoq
// kutayotgan hisoblanadi. Bir joyda yozilgan: filtr ham, mijozdagi chip ham
// SHU chegaradan kelib chiqadi.
const staleDays = 3

// appStatuses — arizaning holat ro'yxati, Figma 3.6 dagi voronka kartalari
// tartibida. Ayni ro'yxat `?status=` uchun oq ro'yxat vazifasini ham bajaradi.
var appStatuses = []string{"pending", "accepted", "rejected", "cancelled", "completed"}

// appStatusKnown — oq ro'yxatning tez qaraladigan ko'rinishi.
var appStatusKnown = func() map[string]bool {
	m := make(map[string]bool, len(appStatuses))
	for _, s := range appStatuses {
		m[s] = true
	}
	return m
}()

// appStatusLabel — CSV dagi `holat` ustuni uchun o'zbekcha yorliqlar
// (Figma 3.6a · CSV: "Kutilmoqda | Qabul qilingan | ..."). Faylni ochgan odam
// `pending` degan kodni emas, ekranda ko'rgan so'zni ko'rishi kerak.
var appStatusLabel = map[string]string{
	"pending":   "Kutilmoqda",
	"accepted":  "Qabul qilingan",
	"rejected":  "Rad etilgan",
	"cancelled": "Bekor qilingan",
	"completed": "Bajarilgan",
}

// appStatusText returns the Uzbek label, or the raw code when it is unknown.
//
// Noma'lum kod YO'QOTILMAYDI: bazada begona holat paydo bo'lsa, eksportda
// bo'sh katak emas, aynan o'sha kod ko'rinishi kerak — aks holda muammo
// jimgina yashiringan bo'lardi.
func appStatusText(code string) string {
	if s, ok := appStatusLabel[code]; ok {
		return s
	}
	if len(code) > 24 {
		return code[:24]
	}
	return code
}

// appsBase is the filter every admin application query starts from.
//
// # NEGA `isReviewData` CHIQARIB TASHLANADI
//
// Do'kon tekshiruvi (Play/App Store) uchun yaratilgan namoyish arizalari
// haqiqiy arizalar orasida turmasligi kerak: adminning sanoqlari
// buzilardi va ularning "telefon raqamlari" CSV faylga tushib ketardi.
// `internal/admin/stats.go` (notDeletedNotReview) shu qoidani allaqachon
// qo'llaydi — arizalar filtri esa ortda qolib kelgan edi.
func appsBase() bson.M {
	return bson.M{"isReviewData": bson.M{"$ne": true}}
}

// appsFilter is shared by ListApplications and the applications CSV export.
// Params: status (whitelisted), stale=1 (pending older than staleDays),
// worker=<ObjectID> (one worker's applications).
//
// # NEGA OQ RO'YXAT
//
// Ilgari `?status=` qiymati to'g'ridan-to'g'ri Mongo filtriga tushardi.
// Begona qiymat xatolik bermay, bo'sh ro'yxat qaytarardi — admin uchun bu
// "bunday ariza yo'q" degan YOLG'ON javob. Endi faqat tanish holat filtr
// bo'ladi, noma'lumi esa e'tiborsiz qoldiriladi (ro'yxat kengroq ko'rinadi,
// lekin hech narsa yashirilmaydi).
//
// # NEGA `$and`
//
// `stale=1` avval `filter["status"]` ni bosib ketardi: admin "Qabul
// qilingan + 3+ kun" so'rasa, faylga `pending` qatorlar tushardi va u
// boshqa ma'lumot ustida qaror qabul qilishi mumkin edi. Endi ikki shart
// birga qo'yiladi — mos kelmasa natija ochiq-oydin bo'sh bo'ladi.
func appsFilter(q url.Values) bson.M {
	filter := appsScopedBase(q)
	and := []bson.M{}
	if status := strings.TrimSpace(q.Get("status")); status != "" && appStatusKnown[status] {
		and = append(and, bson.M{"status": status})
	}
	if q.Get("stale") == "1" {
		and = append(and,
			bson.M{"status": "pending"},
			bson.M{"appliedAt": bson.M{"$lte": time.Now().AddDate(0, 0, -staleDays)}},
		)
	}
	// `worker=<ObjectID>` — bitta ishchining barcha arizalari. Manba: batafsil
	// sahifadagi «Barchasini ko'rish» havolasi (Figma 3.6.1 · 7-bo'lim).
	//
	// Qiymat ObjectID ga aylantirib ko'riladi, ya'ni satr filtrga XOM holda
	// tushmaydi: `?worker={"$ne":null}` kabi qiymat bu yerda xatolik beradi va
	// tashlab yuboriladi. Bu ham oq ro'yxat qoidasi — noma'lum qiymat
	// e'tiborsiz qoldiriladi, ro'yxat kengroq ko'rinadi, lekin yolg'on bo'sh
	// javob qaytmaydi.
	if w, err := primitive.ObjectIDFromHex(strings.TrimSpace(q.Get("worker"))); err == nil {
		and = append(and, bson.M{"workerId": w})
	}
	if len(and) > 0 {
		filter["$and"] = and
	}
	return filter
}

func appsScopedBase(q url.Values) bson.M {
	filter := appsBase()
	if raw := strings.TrimSpace(q.Get("elonId")); raw != "" {
		// Invalid IDs must not silently broaden a listing-specific request.
		id, _ := primitive.ObjectIDFromHex(raw)
		filter["elonId"] = id
	}
	return filter
}

// appsScope describes the applied filters for the audit log / diagnostics.
//
// Xom `r.URL.RawQuery` ATAYLAB ishlatilmaydi: u tekshirilmagan, cheksiz
// uzunlikdagi begona matn bo'lib, audit yozuvi ichida saqlanib qolardi.
// Bu yerda faqat OQ RO'YXATdan o'tgan qiymatlar yoziladi.
func appsScope(q url.Values) string {
	parts := []string{}
	if id, err := primitive.ObjectIDFromHex(strings.TrimSpace(q.Get("elonId"))); err == nil {
		parts = append(parts, "e'lon="+id.Hex())
	}
	if s := strings.TrimSpace(q.Get("status")); appStatusKnown[s] {
		parts = append(parts, "holat="+s)
	}
	if q.Get("stale") == "1" {
		parts = append(parts, "uzoq-kutayotgan")
	}
	// Ishchi ID si YOZILADI: eksport bitta odamning barcha arizalarini
	// (telefon raqami bilan) faylga chiqaradi, ya'ni auditda "kimning"
	// degan savolga javob turishi kerak. Hex `ObjectIDFromHex` dan
	// o'tgani uchun 24 belgidan oshmaydi.
	if w, err := primitive.ObjectIDFromHex(strings.TrimSpace(q.Get("worker"))); err == nil {
		parts = append(parts, "ishchi="+w.Hex())
	}
	if len(parts) == 0 {
		return "filtrsiz"
	}
	return strings.Join(parts, " ")
}

// adminApplicationRow is exactly what the admin applications table (Figma 3.6)
// and its CSV export render — nothing else.
//
// # NEGA TO'LIQ HUJJAT EMAS
//
// `models.Application` da 30 ga yaqin maydon bor: `employerId`,
// `workerAvatarUrl`, `workerRating`, `cancelReason`, `cancelledBy`,
// `workerVerified` ... Ularning birortasi ham bu ekranda chizilmagan.
// O'qilmagan maydon oqib ketolmaydi — shuning uchun proyeksiya jadval
// ustunlari bilan aynan bir xil (xuddi elon_detail.go dagi kabi).
//
// Telefon esa ATAYLAB bor: Figma 3.6 da «Ishchi» katagi ism ostida
// raqamni ko'rsatadi va ism bo'lmasa raqamning O'ZI sarlavha bo'ladi —
// ya'ni bu ekranda telefon "qo'shimcha tafsilot" emas, ustunning mazmuni.
//
// `workerId` esa MOBIL uchun qo'shildi: Figma 3.6b kartochkasida «Ishchi
// profili» tugmasi bor va u 3.4b — ishchining batafsil sahifasiga olib
// boradi (3.6b-a · 4-bo'lim: «Mobilda ustun yo'q, lekin ishchiga o'tish
// ehtiyoji qoladi»). Vebda bunday havola yo'q, shuning uchun ID ilgari
// kerak bo'lmagan. Telefon bo'yicha ishchini qidirib topish yo'li
// ATAYLAB tanlanmadi: bitta raqam ostida hisob qayta ochilgan bo'lishi
// mumkin va admin BOSHQA profilga tushib qolardi. ID o'zi shaxsiy
// ma'lumot emas — qator allaqachon ism va telefonni ko'rsatib turibdi.
type adminApplicationRow struct {
	ID           primitive.ObjectID `bson:"_id" json:"id"`
	ElonID       primitive.ObjectID `bson:"elonId" json:"elonId"`
	ElonTitle    string             `bson:"elonTitle" json:"elonTitle"`
	CategoryName string             `bson:"elonCategoryName" json:"categoryName"`
	WorkerID     primitive.ObjectID `bson:"workerId" json:"workerId"`
	WorkerName   string             `bson:"workerName" json:"workerName"`
	WorkerPhone  string             `bson:"workerPhone" json:"workerPhone"`
	Amount       int64              `bson:"amount" json:"amount"`
	IsNegotiable bool               `bson:"isNegotiable" json:"isNegotiable"`
	Status       string             `bson:"status" json:"status"`
	AppliedAt    time.Time          `bson:"appliedAt" json:"appliedAt"`
}

// appRowProjection mirrors adminApplicationRow field by field.
var appRowProjection = bson.M{
	"elonId":           1,
	"elonTitle":        1,
	"elonCategoryName": 1,
	"workerId":         1,
	"workerName":       1,
	"workerPhone":      1,
	"amount":           1,
	"isNegotiable":     1,
	"status":           1,
	"appliedAt":        1,
}

// appStatusCounts returns one count per KNOWN status over the base filter.
//
// # NEGA FILTRGA BOG'LIQ EMAS
//
// Figma 3.6 dagi beshta voronka kartasi bir vaqtda ikki ishni bajaradi:
// umumiy taqsimotni ko'rsatadi va filtr tugmasi bo'lib turadi. Agar
// sanoqlar joriy filtrga bo'ysunsa, «Kutilmoqda» ni bosgan admin qolgan
// to'rt kartada nol ko'rar va taqsimot yo'qolardi.
//
// # NIMA ATAYLAB YO'Q — NOMA'LUM HOLAT KALITI
//
// Javobga faqat tanish beshta kalit tushadi. Bazadagi begona holat satri
// JSON kaliti bo'lib mijozga o'tib ketmasligi kerak.
func (h *Handler) appStatusCounts(ctx context.Context) map[string]int {
	return h.appStatusCountsFor(ctx, appsBase())
}

func (h *Handler) appStatusCountsFor(ctx context.Context, base bson.M) map[string]int {
	out := make(map[string]int, len(appStatuses))
	for _, s := range appStatuses {
		out[s] = 0
	}
	cur, err := h.Apps.Aggregate(ctx, mongo.Pipeline{
		{{Key: "$match", Value: base}},
		{{Key: "$group", Value: bson.M{"_id": "$status", "count": bson.M{"$sum": 1}}}},
	})
	if err != nil {
		return out
	}
	defer cur.Close(ctx)
	for cur.Next(ctx) {
		var row struct {
			ID    string `bson:"_id"`
			Count int    `bson:"count"`
		}
		if cur.Decode(&row) != nil {
			continue
		}
		if _, ok := out[row.ID]; ok {
			out[row.ID] = row.Count
		}
	}
	return out
}

// ListApplications: paginated application feed for the admin panel (Figma 3.6).
// Query params: page, limit, status, stale=1 (pending older than staleDays).
//
// Javob `paged()` ustiga ikki maydon qo'shadi, shuning uchun u qo'lda
// yig'ilgan: `counts` (voronka kartalari) va `overall` (sarlavhadagi «Jami N
// ta ariza»). Ular bitta so'rovda keladi — ilgari bu sahifa voronka uchun
// `GET /admin/stats` ni ham chaqirardi, u esa butunlay boshqa ekran uchun
// yig'ilgan og'ir javob.
func (h *Handler) ListApplications(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	page, limit, skip := pageParams(r)
	filter := appsFilter(r.URL.Query())

	cur, err := h.Apps.Find(ctx, filter,
		options.Find().
			SetProjection(appRowProjection).
			SetSort(bson.D{{Key: "appliedAt", Value: -1}}).
			SetSkip(skip).
			SetLimit(int64(limit)))
	if err != nil {
		httpx.Err(w, err)
		return
	}
	defer cur.Close(ctx)
	out := []adminApplicationRow{}
	for cur.Next(ctx) {
		var a adminApplicationRow
		if err := cur.Decode(&a); err == nil {
			out = append(out, a)
		}
	}
	total, _ := h.Apps.CountDocuments(ctx, filter)
	base := appsScopedBase(r.URL.Query())
	overall, _ := h.Apps.CountDocuments(ctx, base)
	httpx.JSON(w, 200, map[string]any{
		"items":   out,
		"page":    page,
		"limit":   limit,
		"total":   total,
		"counts":  h.appStatusCountsFor(ctx, base),
		"overall": overall,
	})
}

// ---- CSV export ----
