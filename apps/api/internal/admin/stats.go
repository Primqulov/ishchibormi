package admin

import (
	"context"
	"net/http"
	"time"

	"github.com/ishchibormi/backend/pkg/httpx"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// notDeletedNotReview is the base filter for every admin metric: live records
// only, and never the Google Play review sandbox.
//
// One filter serves both collections. A user document carries isReviewAccount
// and no isReviewData; an elon carries the reverse. Since "$ne: true" also
// matches a missing field, each condition is a no-op on the collection it does
// not apply to — so real records match both and sandbox records match neither.
func notDeletedNotReview() bson.M {
	return bson.M{
		"isDeleted":       bson.M{"$ne": true},
		"isReviewAccount": bson.M{"$ne": true},
		"isReviewData":    bson.M{"$ne": true},
	}
}

// mergeFilter returns a new filter combining a and b; b wins on conflicts.
func mergeFilter(a, b bson.M) bson.M {
	m := bson.M{}
	for k, v := range a {
		m[k] = v
	}
	for k, v := range b {
		m[k] = v
	}
	return m
}

// dashboardAuditWindow — bitta admin uchun "dashboard_viewed" yozuvlari
// orasidagi eng kichik oraliq.
//
// Dashboard o'zini bir necha sababdan qayta so'raydi: pastga tortib
// yangilash, aloqa tiklanishi, ekranga qaytish. Har so'rovga yozuv qo'ysak,
// jurnal shu bitta amaldan iborat bo'lib qolardi va haqiqiy signal — kim
// qachon panelga kirgani — ko'milib ketardi. Soatiga bitta yozuv "bu admin
// bugun panelni ochganmi" degan savolga javob berish uchun yetarli.
const dashboardAuditWindow = time.Hour

// auditDashboardView har oynada bir marta "dashboard_viewed" yozadi.
//
// XAVFSIZLIK: yozuvni SERVER yozadi, klient emas. Klient yuboradigan audit
// yozuvini soxtalashtirish mumkin — jurnalning butun qiymati esa aynan
// soxtalashtirib bo'lmasligida.
//
// Xatolar jimgina yutiladi: audit yozilmagani uchun admin panelining bosh
// sahifasi ochilmay qolishi mumkin emas.
func (h *Handler) auditDashboardView(r *http.Request) {
	aid, err := primitive.ObjectIDFromHex(httpx.AdminID(r))
	if err != nil {
		return
	}
	ctx := r.Context()
	// {adminId, createdAt} indeksi bor (pkg/db/indexes.go) — bu so'rov
	// bitta adminning oxirgi bir soatlik yozuvlarigina ko'rib chiqadi.
	n, err := h.AuditCol.CountDocuments(ctx, bson.M{
		"adminId":   aid,
		"action":    "dashboard_viewed",
		"createdAt": bson.M{"$gte": time.Now().Add(-dashboardAuditWindow)},
	}, options.Count().SetLimit(1))
	if err != nil || n > 0 {
		return
	}
	// Izohda platforma: "kim" ustunidan tashqari "qayerdan" ham kerak —
	// mobil paneldan kirish veb paneldan kirishdan boshqa iz qoldiradi.
	h.auditRaw(ctx, aid, "dashboard_viewed", "", httpx.PlatformOrUnknown(httpx.ClientPlatform(r)))
}

// Dashboard returns the KPI cards for the overview screen. Each metric is a
// cheap CountDocuments; heavier time-series live under Stats.
func (h *Handler) Dashboard(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	h.auditDashboardView(r)
	notDeleted := notDeletedNotReview()
	today := bson.M{"createdAt": bson.M{"$gte": startOfToday()}}

	count := func(col *mongo.Collection, filter bson.M) int64 {
		n, _ := col.CountDocuments(ctx, filter)
		return n
	}
	merge := mergeFilter

	httpx.JSON(w, 200, map[string]any{
		"users":           count(h.Users, notDeleted),
		"activeUsers":     count(h.Users, merge(notDeleted, bson.M{"isBlocked": bson.M{"$ne": true}})),
		"blockedUsers":    count(h.Users, bson.M{"isBlocked": true}),
		"todayUsers":      count(h.Users, merge(notDeleted, today)),
		"elons":           count(h.Elons, notDeleted),
		"recruitingElons": count(h.Elons, merge(notDeleted, bson.M{"status": "recruiting"})),
		"filledElons":     count(h.Elons, merge(notDeleted, bson.M{"status": "filled"})),
		"todayElons":      count(h.Elons, merge(notDeleted, today)),
		"completed":       count(h.Apps, bson.M{"status": "completed"}),
		"openReports":     count(h.Reports, bson.M{"status": "open"}),
		"openFeedback":    count(h.Feedback, bson.M{"status": "open"}),
		// Platforma — oxirgi ishlatilgan klient bo'yicha. "Ro'yxatdan
		// o'tgan" emas, "hozir foydalanadigan": panelning bosh sahifasida
		// kerak bo'ladigan javob "qaysi klientni rivojlantiray" degan
		// savolga tegishli. Ro'yxatdan o'tish taqsimoti /stats da.
		"webUsers":     count(h.Users, merge(notDeleted, bson.M{"lastPlatform": "web"})),
		"androidUsers": count(h.Users, merge(notDeleted, bson.M{"lastPlatform": "android"})),
		"iosUsers":     count(h.Users, merge(notDeleted, bson.M{"lastPlatform": "ios"})),
		// Telegram — bot suhbati va Mini App. Bu odamlar saytga ham,
		// ilovaga ham kirmagan bo'lishi mumkin, shuning uchun alohida ustun.
		"telegramUsers": count(h.Users, merge(notDeleted, bson.M{"lastPlatform": "telegram"})),
		// Sarlavha yubormaydigan eski klientlar va bu funksiyadan oldin
		// ro'yxatdan o'tganlar. ATAYLAB ko'rsatiladi: yashirilsa ustunlar
		// yig'indisi jami foydalanuvchiga teng kelmay, sanoq buzuqdek
		// ko'rinardi.
		"unknownPlatformUsers": count(h.Users, merge(notDeleted, bson.M{
			"lastPlatform": bson.M{"$in": bson.A{nil, ""}},
		})),
	})
}

// nameCount — "nom -> son" juftligi. Turkumlar, viloyatlar va platformalar
// hisobotlari bir xil shaklda qaytadi, shuning uchun ikkala admin klienti
// ham bitta ro'yxat vidjetini qayta ishlatadi.
type nameCount struct {
	Name  string `json:"name" bson:"_id"`
	Count int    `json:"count" bson:"count"`
	// ID — manba yozuvining id'si; hozircha faqat turkumlar to'ldiradi.
	//
	// Panel qatori shu id bilan e'lonlar ro'yxatini `?categoryId=` qilib
	// ochadi. Nom bo'yicha filtrlash yaramaydi: turkum nomi o'zgarishi
	// mumkin va e'lonlar ro'yxati faqat id bo'yicha filtrlaydi.
	//
	// `bson:"-"` — agregatsiya natijasi bu maydonni to'ldirmaydi, uni
	// chaqiruvchi qo'lda qo'yadi (viloyat/platforma qatorlarida id yo'q).
	ID string `json:"id,omitempty" bson:"-"`
}

type dayPoint struct {
	Date  string `json:"date"`
	Count int    `json:"count"`
}

// dailySeries returns one point per day for the last `days` days (gaps filled
// with 0) counting documents by their createdAt date. Used for growth charts.
func dailySeries(ctx context.Context, col *mongo.Collection, days int) []dayPoint {
	since := startOfToday().AddDate(0, 0, -(days - 1))
	pipeline := mongo.Pipeline{
		{{Key: "$match", Value: mergeFilter(notDeletedNotReview(), bson.M{"createdAt": bson.M{"$gte": since}})}},
		{{Key: "$group", Value: bson.M{
			"_id":   bson.M{"$dateToString": bson.M{"format": "%Y-%m-%d", "date": "$createdAt"}},
			"count": bson.M{"$sum": 1},
		}}},
	}
	counts := map[string]int{}
	if cur, err := col.Aggregate(ctx, pipeline); err == nil {
		defer cur.Close(ctx)
		for cur.Next(ctx) {
			var row struct {
				ID    string `bson:"_id"`
				Count int    `bson:"count"`
			}
			if cur.Decode(&row) == nil {
				counts[row.ID] = row.Count
			}
		}
	}
	out := make([]dayPoint, 0, days)
	for i := 0; i < days; i++ {
		d := since.AddDate(0, 0, i).Format("2006-01-02")
		out = append(out, dayPoint{Date: d, Count: counts[d]})
	}
	return out
}

// activeWindowDays — "faol foydalanuvchi" deb sanash oynasi.
//
// 30 kun tanlandi: kunlik ish bozorida odam har kuni kirmaydi — mavsumiy
// ishchi haftada bir marta ham qarashi mumkin. 7 kun bo'lsa haqiqiy faol
// auditoriyaning katta qismi hisobdan tushib qolardi.
const activeWindowDays = 30

// platformCounts bitta maydon bo'yicha foydalanuvchilarni platformaga
// ajratadi va HAR DOIM to'liq ro'yxat qaytaradi (web, android, ios,
// noma'lum) — nol bo'lganda ham.
//
// Nega to'liq: grafik ustunlari yo'qolib-paydo bo'lmasligi kerak. Android
// sanog'i nolga tushganda ustun umuman ko'rinmay qolsa, admin buni "hech
// kim ishlatmayapti" emas, "ma'lumot kelmadi" deb o'qiydi.
func platformCounts(ctx context.Context, col *mongo.Collection, field string, extra bson.M) []nameCount {
	match := mergeFilter(notDeletedNotReview(), extra)
	pipeline := mongo.Pipeline{
		{{Key: "$match", Value: match}},
		{{Key: "$group", Value: bson.M{"_id": "$" + field, "count": bson.M{"$sum": 1}}}},
	}
	raw := map[string]int{}
	if cur, err := col.Aggregate(ctx, pipeline); err == nil {
		defer cur.Close(ctx)
		for cur.Next(ctx) {
			var row struct {
				ID    string `bson:"_id"`
				Count int    `bson:"count"`
			}
			// Maydon yo'q hujjatda _id null bo'ladi va string'ga decode
			// bo'lmaydi — bunday qatorlar ataylab "noma'lum" ga yig'iladi.
			if cur.Decode(&row) != nil {
				continue
			}
			raw[httpx.PlatformOrUnknown(row.ID)] += row.Count
		}
	}
	// Yig'indi jami bilan mos kelishi uchun: yuqoridagi decode xatolari
	// (null _id) hisobdan tushib qolmasin.
	total, _ := col.CountDocuments(ctx, match)
	known := 0
	for _, p := range httpx.Platforms {
		known += raw[p]
	}
	if rest := int(total) - known; rest > 0 {
		raw[httpx.PlatformUnknown] = rest
	}

	out := make([]nameCount, 0, len(httpx.Platforms)+1)
	for _, p := range httpx.Platforms {
		out = append(out, nameCount{Name: p, Count: raw[p]})
	}
	out = append(out, nameCount{Name: httpx.PlatformUnknown, Count: raw[httpx.PlatformUnknown]})
	return out
}

// Stats powers the analytics widgets: 30-day growth curves, the application
// funnel, top categories and regional distribution — all via aggregation.
func (h *Handler) Stats(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	// Time range for the growth curves: 7 | 30 | 90 days (default 30).
	days := 30
	switch r.URL.Query().Get("days") {
	case "7":
		days = 7
	case "90":
		days = 90
	}

	// Application funnel — counts per status.
	funnel := map[string]int{}
	if cur, err := h.Apps.Aggregate(ctx, mongo.Pipeline{
		{{Key: "$group", Value: bson.M{"_id": "$status", "count": bson.M{"$sum": 1}}}},
	}); err == nil {
		defer cur.Close(ctx)
		for cur.Next(ctx) {
			var row struct {
				ID    string `bson:"_id"`
				Count int    `bson:"count"`
			}
			if cur.Decode(&row) == nil {
				funnel[row.ID] = row.Count
			}
		}
	}

	// Top categories by number of (non-deleted) elons.
	//
	// Guruhlash `categoryId` bo'yicha, nom bo'yicha emas: id barqaror, nom
	// esa tahrirlanishi mumkin — nomi o'zgargan turkum ikkita qatorga
	// bo'linib ketardi. Nom ko'rsatish uchun `$first` bilan olinadi.
	topCats := []nameCount{}
	if cur, err := h.Elons.Aggregate(ctx, mongo.Pipeline{
		{{Key: "$match", Value: notDeletedNotReview()}},
		{{Key: "$group", Value: bson.M{
			"_id":   "$categoryId",
			"name":  bson.M{"$first": "$categoryName"},
			"count": bson.M{"$sum": 1},
		}}},
		{{Key: "$sort", Value: bson.M{"count": -1}}},
		{{Key: "$limit", Value: 5}},
	}); err == nil {
		defer cur.Close(ctx)
		for cur.Next(ctx) {
			var row struct {
				ID    primitive.ObjectID `bson:"_id"`
				Name  string             `bson:"name"`
				Count int                `bson:"count"`
			}
			if cur.Decode(&row) != nil {
				continue
			}
			// categoryId yo'q eski yozuvda id bo'sh qoladi. Klient bunday
			// qatorni bosiladigan qilmaydi — filtrsiz ro'yxatga olib
			// borish "shu turkumni ko'rsataman" degan va'dani buzardi.
			id := ""
			if !row.ID.IsZero() {
				id = row.ID.Hex()
			}
			topCats = append(topCats, nameCount{Name: row.Name, Count: row.Count, ID: id})
		}
	}

	// Users per region (top 10).
	regions := []nameCount{}
	if cur, err := h.Users.Aggregate(ctx, mongo.Pipeline{
		{{Key: "$match", Value: mergeFilter(notDeletedNotReview(), bson.M{"region": bson.M{"$nin": bson.A{"", nil}}})}},
		{{Key: "$group", Value: bson.M{"_id": "$region", "count": bson.M{"$sum": 1}}}},
		{{Key: "$sort", Value: bson.M{"count": -1}}},
		{{Key: "$limit", Value: 10}},
	}); err == nil {
		defer cur.Close(ctx)
		for cur.Next(ctx) {
			var row nameCount
			if cur.Decode(&row) == nil {
				regions = append(regions, row)
			}
		}
	}

	// Platformalar — ikkita alohida kesim:
	//   signup — qaysi klientdan ro'yxatdan o'tishgan (o'sish kanali);
	//   active — oxirgi 30 kunda qaysi klientdan foydalanishgan.
	// Ikkalasi bir xil bo'lishi shart emas va aynan farqi qiziq: vebdan
	// kelib ilovaga ko'chgan oqim faqat shu ikki qator solishtirilganda
	// ko'rinadi.
	activeSince := startOfToday().AddDate(0, 0, -activeWindowDays)
	platforms := map[string]any{
		"signup": platformCounts(ctx, h.Users, "signupPlatform", nil),
		"active": platformCounts(ctx, h.Users, "lastPlatform", bson.M{
			"lastSeenAt": bson.M{"$gte": activeSince},
		}),
		"activeWindowDays": activeWindowDays,
	}

	httpx.JSON(w, 200, map[string]any{
		"userGrowth":    dailySeries(ctx, h.Users, days),
		"elonGrowth":    dailySeries(ctx, h.Elons, days),
		"funnel":        funnel,
		"topCategories": topCats,
		"regions":       regions,
		"platforms":     platforms,
	})
}
