package elon

import (
	"context"
	"encoding/json"
	"math"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/ishchibormi/backend/internal/models"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

func TestNearbyRejectsInvalidCoordinatesBeforeDatabase(t *testing.T) {
	h := &Handler{}
	for _, query := range []string{"", "lat=41", "lat=abc&lng=69", "lat=NaN&lng=69", "lat=41&lng=Inf", "lat=91&lng=69", "lat=41&lng=-181", "lat=41&lng=69&page=1001", "lat=41&lng=69&day=2026-99-99", "lat=41&lng=69&categoryId=bad"} {
		w := httptest.NewRecorder()
		h.Nearby(w, httptest.NewRequest("GET", "/api/elons/nearby?"+query, nil))
		if w.Code != 400 {
			t.Fatalf("%s: status=%d", query, w.Code)
		}
	}
}

func TestNearbyDayAndCategoryFilterBeforeSortAndCount(t *testing.T) {
	db := ownerTestDB(t)
	h := &Handler{Col: db.Collection("elons")}
	day := time.Now().In(uzTZ).AddDate(0, 0, 1).Format("2006-01-02")
	category := primitive.NewObjectID()
	for i := 0; i < 4; i++ {
		e := models.Elon{ID: primitive.NewObjectID(), Title: "Ish", CategoryID: category, Status: "recruiting", WorkersNeeded: 1, Lat: 41.3 + float64(i)/1000, Lng: 69.2, StartDate: day, WorkTimeFrom: "10:00"}
		if i == 0 {
			e.CategoryID = primitive.NewObjectID()
		}
		if i == 1 {
			e.StartDate = time.Now().In(uzTZ).AddDate(0, 0, 2).Format("2006-01-02")
		}
		if i == 3 {
			e.StartDate += "T10:00:00"
		}
		if _, err := h.Col.InsertOne(context.Background(), e); err != nil {
			t.Fatal(err)
		}
	}
	w := httptest.NewRecorder()
	h.Nearby(w, httptest.NewRequest("GET", "/api/elons/nearby?lat=41.3&lng=69.2&limit=1&page=2&day="+day+"&categoryId="+category.Hex(), nil))
	var result struct {
		Items []nearbyElon `json:"items"`
		Total int          `json:"total"`
	}
	if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &result) != nil {
		t.Fatal(w.Body.String())
	}
	if result.Total != 2 || len(result.Items) != 1 || result.Items[0].Lat != 41.303 {
		t.Fatalf("filtered order/count: %+v", result)
	}
}

func TestNearbyOrdersAllEligibleListingsBeforePagination(t *testing.T) {
	db := ownerTestDB(t)
	h := &Handler{Col: db.Collection("elons")}
	now := time.Now()
	seed := func(title string, lat, lng float64, edit func(*models.Elon)) primitive.ObjectID {
		t.Helper()
		e := models.Elon{ID: primitive.NewObjectID(), OwnerID: primitive.NewObjectID(), Title: title, Description: "Vazifa", Status: "recruiting", WorkersNeeded: 3, Lat: lat, Lng: lng, PublishedAt: &now, StartDate: now.In(uzTZ).AddDate(0, 0, 1).Format("2006-01-02"), WorkTimeFrom: "09:00"}
		if edit != nil {
			edit(&e)
		}
		if _, err := h.Col.InsertOne(context.Background(), e); err != nil {
			t.Fatal(err)
		}
		return e.ID
	}
	// The nearest listing is older than all the others; limiting a normal feed
	// first would lose it. These positions differ by about 111, 222, 333 metres.
	nearest := seed("Eng yaqin", 41.301, 69.2, func(e *models.Elon) { old := now.Add(-time.Hour); e.PublishedAt = &old })
	second := seed("Ikkinchi", 41.302, 69.2, nil)
	third := seed("Uchinchi", 41.303, 69.2, nil)
	seed("To'lgan", 41.30001, 69.2, func(e *models.Elon) { e.Status = "filled" })
	seed("Noto'g'ri recruiting", 41.30001, 69.2, func(e *models.Elon) { e.AcceptedCount = e.WorkersNeeded })
	seed("Yashirilgan", 41.30001, 69.2, func(e *models.Elon) { e.Status = "hidden" })
	seed("O'chirilgan", 41.30001, 69.2, func(e *models.Elon) { e.IsDeleted = true })
	seed("Bloklangan", 41.30001, 69.2, func(e *models.Elon) { e.OwnerBlocked = true })
	seed("Demo", 41.30001, 69.2, func(e *models.Elon) { e.IsReviewData = true })
	seed("Eskirgan", 41.30001, 69.2, func(e *models.Elon) { e.StartDate = now.In(uzTZ).AddDate(0, 0, -2).Format("2006-01-02") })
	seed("Lokatsiyasiz", 0, 0, nil)
	seed("Noto'g'ri lokatsiya", 91, 69, nil)
	_, err := h.Col.InsertMany(context.Background(), []any{
		bson.M{"_id": primitive.NewObjectID(), "title": "text coordinate", "status": "recruiting", "workersNeeded": 1, "lat": "41.3", "lng": 69.2},
		bson.M{"_id": primitive.NewObjectID(), "title": "array coordinate", "status": "recruiting", "workersNeeded": 1, "lat": bson.A{41.3}, "lng": 69.2},
		bson.M{"_id": primitive.NewObjectID(), "title": "missing longitude", "status": "recruiting", "workersNeeded": 1, "lat": 41.3},
	})
	if err != nil {
		t.Fatal(err)
	}
	read := func(query string) struct {
		Items []nearbyElon `json:"items"`
		Total int          `json:"total"`
	} {
		t.Helper()
		w := httptest.NewRecorder()
		h.Nearby(w, httptest.NewRequest("GET", "/api/elons/nearby?"+query, nil))
		if w.Code != 200 {
			t.Fatalf("nearby: %d %s", w.Code, w.Body.String())
		}
		var page struct {
			Items []nearbyElon `json:"items"`
			Total int          `json:"total"`
		}
		if err := json.Unmarshal(w.Body.Bytes(), &page); err != nil {
			t.Fatal(err)
		}
		return page
	}
	p := read("lat=41.3&lng=69.2&limit=2&page=1")
	if p.Total != 3 || len(p.Items) != 2 || p.Items[0].ID != nearest || p.Items[1].ID != second {
		t.Fatalf("incorrect nearest results: %+v", p)
	}
	if math.Abs(p.Items[0].DistanceMeters-111.195) > 0.5 {
		t.Fatalf("bad metres: %f", p.Items[0].DistanceMeters)
	}
	p = read("lat=41.3&lng=69.2&limit=2&page=2")
	if len(p.Items) != 1 || p.Items[0].ID != third {
		t.Fatalf("bad second page: %+v", p)
	}
	// A moved pin takes effect on the next search without a geo backfill.
	_, err = h.Col.UpdateOne(context.Background(), bson.M{"_id": third}, bson.M{"$set": bson.M{"lat": 41.3001}})
	if err != nil {
		t.Fatal(err)
	}
	p = read("lat=41.3&lng=69.2")
	if p.Items[0].ID != third {
		t.Fatal("coordinate edit was not reflected")
	}
	count, err := h.Col.CountDocuments(context.Background(), bson.M{"distanceMeters": bson.M{"$exists": true}})
	if err != nil || count != 0 {
		t.Fatal("query wrote distances to listings")
	}
}

func TestNearbyHandlesCoincidentAntipodalAndEqualDistances(t *testing.T) {
	db := ownerTestDB(t)
	h := &Handler{Col: db.Collection("elons")}
	now := time.Now()
	ids := []primitive.ObjectID{primitive.NewObjectID(), primitive.NewObjectID(), primitive.NewObjectID()}
	for i, id := range ids {
		lat, lng := 10.0, 20.0
		if i == 2 {
			lat, lng = -10, -160
		}
		_, err := h.Col.InsertOne(context.Background(), models.Elon{ID: id, Title: "Ish", Status: "recruiting", WorkersNeeded: 1, Lat: lat, Lng: lng, PublishedAt: &now})
		if err != nil {
			t.Fatal(err)
		}
	}
	w := httptest.NewRecorder()
	h.Nearby(w, httptest.NewRequest("GET", "/api/elons/nearby?lat=10&lng=20", nil))
	if w.Code != 200 {
		t.Fatal(w.Body.String())
	}
	var result struct {
		Items []nearbyElon `json:"items"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &result)
	if len(result.Items) != 3 || result.Items[0].ID != ids[0] || result.Items[1].ID != ids[1] || result.Items[0].DistanceMeters != 0 || math.Abs(result.Items[2].DistanceMeters-math.Pi*6371008.8) > 1 {
		t.Fatalf("unstable/invalid distances: %+v", result)
	}
}
