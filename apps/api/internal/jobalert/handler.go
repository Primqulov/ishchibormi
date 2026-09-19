package jobalert

import (
	"net/http"
	"strings"

	"github.com/ishchibormi/backend/internal/models"
	"github.com/ishchibormi/backend/pkg/httpx"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

type saveReq struct {
	Lat        float64 `json:"lat"`
	Lng        float64 `json:"lng"`
	RadiusM    int     `json:"radiusM"`
	CategoryID string  `json:"categoryId"`
	PlaceName  string  `json:"placeName"`
}

// Get — joriy signal yoki null. Signal yo'qligi xato emas: klient shu javob
// bo'yicha «yoqish» yoki «o'chirish» tugmasini ko'rsatadi.
func (s *Service) Get(w http.ResponseWriter, r *http.Request) {
	uid, err := primitive.ObjectIDFromHex(httpx.UserID(r))
	if err != nil {
		httpx.Err(w, httpx.NewError(401, "unauthorized", "unauthorized"))
		return
	}
	alert, err := s.load(r.Context(), uid)
	if err != nil {
		httpx.Err(w, err)
		return
	}
	httpx.JSON(w, 200, map[string]any{"alert": alert})
}

func (s *Service) Save(w http.ResponseWriter, r *http.Request) {
	uid, err := primitive.ObjectIDFromHex(httpx.UserID(r))
	if err != nil {
		httpx.Err(w, httpx.NewError(401, "unauthorized", "unauthorized"))
		return
	}
	var req saveReq
	if err := httpx.Decode(r, &req); err != nil {
		httpx.Err(w, err)
		return
	}
	if !validCoordinates(req.Lat, req.Lng) || (req.Lat == 0 && req.Lng == 0) {
		httpx.Err(w, httpx.NewError(400, "bad_coordinates", "To'g'ri joylashuv koordinatalarini yuboring."))
		return
	}
	radius := req.RadiusM
	if radius == 0 {
		radius = DefaultRadiusM
	}
	if radius < MinRadiusM || radius > MaxRadiusM {
		httpx.Err(w, httpx.NewError(400, "bad_radius", "Radius 1 km dan 50 km gacha bo'lishi kerak."))
		return
	}
	alert := Alert{UserID: uid, Lat: req.Lat, Lng: req.Lng, RadiusM: radius, PlaceName: trimText(req.PlaceName, 120)}
	// Turkum nomi ATAYLAB klientdan olinmaydi: bazadagi nom yagona haqiqat,
	// aks holda bildirishnomada foydalanuvchi yozgan matn paydo bo'lardi.
	if strings.TrimSpace(req.CategoryID) != "" {
		id, err := primitive.ObjectIDFromHex(strings.TrimSpace(req.CategoryID))
		if err != nil {
			httpx.Err(w, httpx.NewError(400, "bad_category", "Ish turi noto'g'ri."))
			return
		}
		var cat models.Category
		if s.Categories == nil || s.Categories.FindOne(r.Context(), bson.M{"_id": id, "isActive": true}).Decode(&cat) != nil {
			httpx.Err(w, httpx.NewError(404, "not_found", "Ish turi topilmadi."))
			return
		}
		alert.CategoryID, alert.CategoryName = &id, cat.Name
	}
	saved, err := s.store(r.Context(), alert)
	if err != nil {
		httpx.Err(w, err)
		return
	}
	httpx.JSON(w, 200, map[string]any{"alert": saved})
}

func (s *Service) Delete(w http.ResponseWriter, r *http.Request) {
	uid, err := primitive.ObjectIDFromHex(httpx.UserID(r))
	if err != nil {
		httpx.Err(w, httpx.NewError(401, "unauthorized", "unauthorized"))
		return
	}
	if err := s.remove(r.Context(), uid); err != nil {
		httpx.Err(w, err)
		return
	}
	httpx.JSON(w, 200, map[string]any{"alert": nil})
}

func trimText(v string, max int) string {
	v = strings.TrimSpace(v)
	r := []rune(v)
	if len(r) > max {
		return string(r[:max])
	}
	return v
}
