package admin

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/ishchibormi/backend/pkg/elonquery"
	"github.com/ishchibormi/backend/pkg/httpx"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// «Muddati o'tgan ishlar» kaliti.
//
// Odatda e'lon belgilangan boshlanish vaqtidan elonquery.FeedExpiryGrace
// o'tgach ommaviy ro'yxatlardan o'zi chiqib ketadi. Bu kalit yoqilsa vaqt
// sharti olib tashlanadi va hali `recruiting` holatidagi BARCHA e'lonlar
// ko'rinadi; o'chirilsa — odatdagi xulq qaytadi.
//
// # NIMA O'ZGARMAYDI
//
// Kalit faqat VAQT shartiga tegadi. Yashirilgan, o'chirilgan, bekor qilingan,
// to'lgan yoki egasi bloklangan e'lon kalit yoqilganda ham ko'rinmaydi, va
// e'lonning o'z `status` maydoni bu yerda umuman o'zgartirilmaydi — shuning
// uchun kalitni o'chirish hech narsani "tiklashi" shart emas.

// feedSettingsID — `app_settings` kolleksiyasidagi yagona hujjat kaliti.
const feedSettingsID = "feed"

// feedSettingsSyncEvery — bazadagi qiymat xotiradagiga qanchalik tez yetib
// borishi. Kalitni bosgan nusxa uni darhol qo'llaydi; bu oraliq faqat boshqa
// nusxalar (yoki bazada qo'lda qilingan o'zgarish) uchun.
const feedSettingsSyncEvery = 30 * time.Second

type feedSettingsDoc struct {
	ID            string             `bson:"_id"`
	ShowExpired   bool               `bson:"showExpired"`
	UpdatedAt     time.Time          `bson:"updatedAt"`
	UpdatedBy     primitive.ObjectID `bson:"updatedBy,omitempty"`
	UpdatedByName string             `bson:"updatedByName,omitempty"`
}

type feedSettingsResponse struct {
	ShowExpired bool `json:"showExpired"`
	// ExpiredCount — FAQAT muddati o'tgani uchun yashirilgan (kalit yoqiq
	// bo'lsa — faqat shu kalit tufayli ko'rinib turgan) e'lonlar soni.
	// Admin kalitni bosishdan OLDIN nechta e'lon paydo bo'lishini/yo'qolishini
	// ko'rishi uchun.
	ExpiredCount  int64      `json:"expiredCount"`
	UpdatedAt     *time.Time `json:"updatedAt,omitempty"`
	UpdatedByName string     `json:"updatedByName,omitempty"`
}

func (h *Handler) feedSettingsCol() *mongo.Collection {
	return h.Elons.Database().Collection("app_settings")
}

// LoadFeedSettings — bazadagi qiymatni o'qib, filtrga qo'llaydi. Hujjat hali
// yaratilmagan bo'lsa bu xato emas: kalit hech qachon bosilmagan, ya'ni
// standart xulq (muddati o'tganlar yashirin).
func (h *Handler) LoadFeedSettings(ctx context.Context) error {
	doc, err := h.readFeedSettings(ctx)
	if err != nil {
		return err
	}
	elonquery.SetShowExpired(doc.ShowExpired)
	return nil
}

func (h *Handler) readFeedSettings(ctx context.Context) (feedSettingsDoc, error) {
	var doc feedSettingsDoc
	err := h.feedSettingsCol().FindOne(ctx, bson.M{"_id": feedSettingsID}).Decode(&doc)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return feedSettingsDoc{ID: feedSettingsID}, nil
	}
	return doc, err
}

// RunFeedSettingsSync — qiymatni vaqti-vaqti bilan bazadan qayta o'qiydi.
// O'qib bo'lmasa oxirgi ma'lum qiymat qoladi: bazaning qisqa uzilishi feedni
// to'satdan boshqa rejimga o'tkazib yubormasligi kerak.
func (h *Handler) RunFeedSettingsSync(ctx context.Context) {
	t := time.NewTicker(feedSettingsSyncEvery)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			c, cancel := context.WithTimeout(ctx, 5*time.Second)
			_ = h.LoadFeedSettings(c)
			cancel()
		}
	}
}

// expiredOpenCount — boshqa hamma shartdan o'tadigan, lekin vaqti o'tgan
// e'lonlar soni: «vaqt shartisiz» va «vaqt sharti bilan» sanoqlar ayirmasi.
func (h *Handler) expiredOpenCount(ctx context.Context) int64 {
	now := time.Now()
	fresh := elonquery.StrictActiveFilter(now, false)
	all := elonquery.StrictActiveFilter(now, false)
	delete(all, "$expr")
	nAll, err := h.Elons.CountDocuments(ctx, all)
	if err != nil {
		return 0
	}
	nFresh, err := h.Elons.CountDocuments(ctx, fresh)
	if err != nil || nFresh > nAll {
		return 0
	}
	return nAll - nFresh
}

func (h *Handler) feedSettingsJSON(ctx context.Context, doc feedSettingsDoc) feedSettingsResponse {
	out := feedSettingsResponse{
		ShowExpired:   doc.ShowExpired,
		ExpiredCount:  h.expiredOpenCount(ctx),
		UpdatedByName: doc.UpdatedByName,
	}
	if !doc.UpdatedAt.IsZero() {
		at := doc.UpdatedAt
		out.UpdatedAt = &at
	}
	return out
}

// GetFeedSettings — kalitning joriy holati. Javob xotiradagi emas, BAZADAGI
// qiymatni beradi: panel doim haqiqat manbasini ko'rsatsin.
func (h *Handler) GetFeedSettings(w http.ResponseWriter, r *http.Request) {
	doc, err := h.readFeedSettings(r.Context())
	if err != nil {
		httpx.Err(w, err)
		return
	}
	httpx.JSON(w, 200, h.feedSettingsJSON(r.Context(), doc))
}

type feedSettingsRequest struct {
	// Ko'rsatkich: maydon umuman yuborilmagan so'rov `false` deb o'qilib,
	// kalitni jimgina o'chirib qo'ymasligi kerak.
	ShowExpired *bool `json:"showExpired"`
}

// SetFeedSettings — kalitni almashtiradi (faqat superadmin: bu bitta e'lon
// emas, butun platformaning ko'rinishi).
func (h *Handler) SetFeedSettings(w http.ResponseWriter, r *http.Request) {
	var req feedSettingsRequest
	if err := httpx.Decode(r, &req); err != nil {
		httpx.Err(w, err)
		return
	}
	if req.ShowExpired == nil {
		httpx.Err(w, httpx.NewError(400, "bad_request", "showExpired majburiy"))
		return
	}
	a, err := h.currentAdmin(r)
	if err != nil {
		httpx.Err(w, err)
		return
	}
	doc := feedSettingsDoc{
		ID: feedSettingsID, ShowExpired: *req.ShowExpired,
		UpdatedAt: time.Now().UTC().Truncate(time.Millisecond), UpdatedBy: a.ID, UpdatedByName: a.Username,
	}
	if _, err := h.feedSettingsCol().ReplaceOne(r.Context(), bson.M{"_id": feedSettingsID}, doc,
		options.Replace().SetUpsert(true)); err != nil {
		httpx.Err(w, err)
		return
	}
	// Bazaga yozilgandan KEYIN qo'llanadi: yozuv yiqilsa feed panel
	// ko'rsatayotgan holatdan farq qilib qolmaydi.
	elonquery.SetShowExpired(doc.ShowExpired)
	detail := "muddati o'tgan ishlar yashirildi"
	if doc.ShowExpired {
		detail = "muddati o'tgan ishlar ko'rsatiladi"
	}
	h.audit(r, "feed_show_expired", feedSettingsID, detail)
	httpx.JSON(w, 200, h.feedSettingsJSON(r.Context(), doc))
}
