package application

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/ishchibormi/backend/internal/models"
	"github.com/ishchibormi/backend/pkg/httpx"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

func TestApplicationDetailsRestrictsAccessAndPreservesHistory(t *testing.T) {
	h, _, app := completionTestHandler(t)
	for _, status := range []string{"pending", "accepted", "rejected", "cancelled", "completed"} {
		if _, err := h.Apps.UpdateOne(context.Background(), bson.M{"_id": app.ID}, bson.M{"$set": bson.M{"status": status, "cancelReason": "Ish o'rinlari to'ldi"}}); err != nil {
			t.Fatal(err)
		}
		for _, actor := range []primitive.ObjectID{app.WorkerID, app.EmployerID} {
			r := httptest.NewRequest("GET", "/api/applications/"+app.ID.Hex(), nil)
			route := chi.NewRouteContext()
			route.URLParams.Add("id", app.ID.Hex())
			ctx := context.WithValue(r.Context(), chi.RouteCtxKey, route)
			ctx = context.WithValue(ctx, httpx.CtxUserID, actor.Hex())
			w := httptest.NewRecorder()
			h.Get(w, r.WithContext(ctx))
			var got models.Application
			if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &got) != nil || got.ID != app.ID || got.Status != status || got.CancelReason != "Ish o'rinlari to'ldi" {
				t.Fatalf("participant details: %d %s", w.Code, w.Body.String())
			}
		}
	}
	for _, tc := range []struct {
		name, actor, id         string
		reviewData, reviewActor bool
		want                    int
	}{
		{"stranger", primitive.NewObjectID().Hex(), app.ID.Hex(), false, false, 404},
		{"signed out", "", app.ID.Hex(), false, false, 401},
		{"missing", app.WorkerID.Hex(), primitive.NewObjectID().Hex(), false, false, 404},
		{"invalid", app.WorkerID.Hex(), "wrong", false, false, 400},
		{"hidden demo", app.EmployerID.Hex(), app.ID.Hex(), true, false, 404},
		{"review worker", app.WorkerID.Hex(), app.ID.Hex(), true, true, 200},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := h.Apps.UpdateOne(context.Background(), bson.M{"_id": app.ID}, bson.M{"$set": bson.M{"isReviewData": tc.reviewData}}); err != nil {
				t.Fatal(err)
			}
			r := httptest.NewRequest("GET", "/api/applications/"+tc.id, nil)
			route := chi.NewRouteContext()
			route.URLParams.Add("id", tc.id)
			ctx := context.WithValue(r.Context(), chi.RouteCtxKey, route)
			ctx = context.WithValue(ctx, httpx.CtxUserID, tc.actor)
			ctx = context.WithValue(ctx, httpx.CtxReviewActor, tc.reviewActor)
			w := httptest.NewRecorder()
			h.Get(w, r.WithContext(ctx))
			if w.Code != tc.want {
				t.Fatalf("status=%d, want %d: %s", w.Code, tc.want, w.Body.String())
			}
		})
	}
}
