package elon

import (
	"math"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/ishchibormi/backend/internal/models"
	"github.com/ishchibormi/backend/pkg/elonquery"
	"github.com/ishchibormi/backend/pkg/httpx"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

type nearbyElon struct {
	models.Elon    `bson:",inline"`
	DistanceMeters float64 `bson:"distanceMeters" json:"distanceMeters"`
}

// Nearby sorts ALL eligible listings before pagination. It uses the existing
// lat/lng fields, so legacy listings and coordinate edits are immediately
// searchable without maintaining a second, potentially stale location field.
func (h *Handler) Nearby(w http.ResponseWriter, r *http.Request) {
	lat, latErr := strconv.ParseFloat(strings.TrimSpace(r.URL.Query().Get("lat")), 64)
	lng, lngErr := strconv.ParseFloat(strings.TrimSpace(r.URL.Query().Get("lng")), 64)
	if latErr != nil || lngErr != nil || math.IsNaN(lat) || math.IsNaN(lng) || math.IsInf(lat, 0) || math.IsInf(lng, 0) || lat < -90 || lat > 90 || lng < -180 || lng > 180 {
		httpx.Err(w, httpx.NewError(400, "bad_coordinates", "To'g'ri joylashuv koordinatalarini yuboring."))
		return
	}
	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	if page < 1 {
		page = 1
	}
	if page > 1000 {
		httpx.Err(w, httpx.NewError(400, "bad_page", "Qidiruvni yangilang."))
		return
	}
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	if limit < 1 || limit > 20 {
		limit = 5
	}
	filter := nearbyFilter(time.Now(), httpx.IsReviewActor(r.Context()))
	if day := r.URL.Query().Get("day"); day != "" {
		if _, err := time.Parse("2006-01-02", day); err != nil {
			httpx.Err(w, httpx.NewError(400, "bad_day", "Sana noto'g'ri."))
			return
		}
		filter["startDate"] = bson.M{"$regex": "^" + day + "($|T)"}
	}
	if category := r.URL.Query().Get("categoryId"); category != "" {
		id, err := primitive.ObjectIDFromHex(category)
		if err != nil {
			httpx.Err(w, httpx.NewError(400, "bad_category", "Ish turi noto'g'ri."))
			return
		}
		filter["categoryId"] = id
	}
	cur, err := h.Col.Aggregate(r.Context(), nearbyPipeline(filter, lat, lng, page, limit),
		options.Aggregate().SetAllowDiskUse(true).SetMaxTime(5*time.Second))
	if err != nil {
		httpx.Err(w, err)
		return
	}
	defer cur.Close(r.Context())
	items := []nearbyElon{}
	if err := cur.All(r.Context(), &items); err != nil {
		httpx.Err(w, err)
		return
	}
	total, err := h.Col.CountDocuments(r.Context(), filter, options.Count().SetMaxTime(5*time.Second))
	if err != nil {
		httpx.Err(w, err)
		return
	}
	// Distances are a query result, never written back into listing documents.
	w.Header().Set("Cache-Control", "no-store")
	httpx.JSON(w, 200, map[string]any{"items": items, "page": page, "limit": limit, "total": total})
}

func nearbyFilter(now time.Time, includeReview bool) bson.M {
	f := elonquery.ActiveFilter(now, includeReview)
	f["lat"] = bson.M{"$gte": -90, "$lte": 90}
	f["lng"] = bson.M{"$gte": -180, "$lte": 180}
	f["$expr"] = bson.M{"$and": bson.A{
		f["$expr"],
		bson.M{"$isNumber": "$lat"}, bson.M{"$isNumber": "$lng"},
		// 0,0 is the legacy application's sentinel for a missing map pin.
		bson.M{"$or": bson.A{bson.M{"$ne": bson.A{"$lat", 0}}, bson.M{"$ne": bson.A{"$lng", 0}}}},
		bson.M{"$gt": bson.A{bson.M{"$ifNull": bson.A{"$workersNeeded", 0}}, bson.M{"$ifNull": bson.A{"$acceptedCount", 0}}}},
	}}
	return f
}

func nearbyPipeline(filter bson.M, lat, lng float64, page, limit int) mongo.Pipeline {
	radLat := lat * math.Pi / 180
	// Haversine remains accurate for coincident, nearby and antipodal points.
	// Clamp roundoff to [0,1] before sqrt/asin to avoid invalid distances.
	halfDelta := func(field string, origin float64) bson.M {
		return bson.M{"$divide": bson.A{bson.M{"$subtract": bson.A{bson.M{"$degreesToRadians": field}, origin * math.Pi / 180}}, 2}}
	}
	a := bson.M{"$add": bson.A{
		bson.M{"$pow": bson.A{bson.M{"$sin": halfDelta("$lat", lat)}, 2}},
		bson.M{"$multiply": bson.A{math.Cos(radLat), bson.M{"$cos": bson.M{"$degreesToRadians": "$lat"}}, bson.M{"$pow": bson.A{bson.M{"$sin": halfDelta("$lng", lng)}, 2}}}},
	}}
	distance := bson.M{"$multiply": bson.A{2 * 6371008.8, bson.M{"$asin": bson.M{"$sqrt": bson.M{"$min": bson.A{1, bson.M{"$max": bson.A{0, a}}}}}}}}
	return mongo.Pipeline{
		{{Key: "$match", Value: filter}},
		{{Key: "$set", Value: bson.M{"distanceMeters": distance}}},
		{{Key: "$sort", Value: bson.D{{Key: "distanceMeters", Value: 1}, {Key: "publishedAt", Value: -1}, {Key: "_id", Value: 1}}}},
		{{Key: "$skip", Value: int64((page - 1) * limit)}},
		{{Key: "$limit", Value: int64(limit)}},
	}
}
