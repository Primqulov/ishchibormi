package elon

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/ishchibormi/backend/internal/models"
	"github.com/ishchibormi/backend/pkg/httpx"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

func TestCreationKeysAreScopedToAuthenticatedOwner(t *testing.T) {
	r := httptest.NewRequest("POST", "/api/elons", nil)
	r.Header.Set("Idempotency-Key", primitive.NewObjectID().Hex())
	owner := primitive.NewObjectID()
	a, ok, err := creationID(r, owner)
	b, _, _ := creationID(r, owner)
	other, _, _ := creationID(r, primitive.NewObjectID())
	if err != nil || !ok || a != b || a == other {
		t.Fatal("creation identity is not stable/scoped")
	}
	r.Header.Set("Idempotency-Key", "invalid")
	if _, _, err := creationID(r, owner); err == nil {
		t.Fatal("invalid key accepted")
	}
}
func TestCreateRetriesConcurrentlyAndAfterLostResponse(t *testing.T) {
	db := ownerTestDB(t)
	h := &Handler{Col: db.Collection("elons"), Users: db.Collection("users"), Categories: db.Collection("categories")}
	owner, cat, key := primitive.NewObjectID(), primitive.NewObjectID(), primitive.NewObjectID().Hex()
	_, err := h.Categories.InsertOne(context.Background(), models.Category{ID: cat, Name: "Qurilish", IsActive: true})
	if err != nil {
		t.Fatal(err)
	}
	_, err = h.Users.InsertOne(context.Background(), models.User{ID: owner, FirstName: "Ali"})
	if err != nil {
		t.Fatal(err)
	}
	start := time.Now().In(uzTZ).Add(2 * time.Hour)
	body := `{"title":"Ish","description":"Vazifa","categoryId":"` + cat.Hex() + `","workersNeeded":3,"pricingType":"total","priceAmount":150000,"startDate":"` + start.Format("2006-01-02") + `","workTimeFrom":"` + start.Format("15:04") + `"}`
	call := func(raw string) *httptest.ResponseRecorder {
		r := httptest.NewRequest("POST", "/api/elons", strings.NewReader(raw))
		r.Header.Set("Idempotency-Key", key)
		r = r.WithContext(context.WithValue(r.Context(), httpx.CtxUserID, owner.Hex()))
		w := httptest.NewRecorder()
		h.Create(w, r)
		return w
	}
	var wg sync.WaitGroup
	responses := make([]*httptest.ResponseRecorder, 3)
	for i := range responses {
		wg.Add(1)
		go func(i int) { defer wg.Done(); responses[i] = call(body) }(i)
	}
	wg.Wait()
	var id primitive.ObjectID
	for _, w := range responses {
		if w.Code != 200 && w.Code != 201 {
			t.Fatalf("create failed: %d %s", w.Code, w.Body.String())
		}
		var got models.Elon
		_ = json.Unmarshal(w.Body.Bytes(), &got)
		if !id.IsZero() && got.ID != id {
			t.Fatal("retry produced second listing")
		}
		id = got.ID
	}
	// A retry returns the committed result before revalidating time/content.
	w := call(`{}`)
	if w.Code != 200 {
		t.Fatalf("committed retry did not recover: %s", w.Body.String())
	}
	count, err := h.Col.CountDocuments(context.Background(), bson.M{"ownerId": owner})
	if err != nil || count != 1 {
		t.Fatalf("created %d listings: %v", count, err)
	}
}
