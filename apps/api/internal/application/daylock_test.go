package application

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/ishchibormi/backend/internal/models"
	"github.com/ishchibormi/backend/internal/notification"
	"github.com/ishchibormi/backend/pkg/httpx"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

func dayLockTestHandler(t *testing.T) *Handler {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	client, err := mongo.Connect(ctx, options.Client().ApplyURI("mongodb://127.0.0.1:27017"))
	if err != nil {
		t.Fatal(err)
	}
	if err := client.Ping(ctx, nil); err != nil {
		_ = client.Disconnect(context.Background())
		t.Skipf("local Mongo unavailable: %v", err)
	}
	db := client.Database("ib_daylock_test_" + primitive.NewObjectID().Hex())
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = db.Drop(ctx)
		_ = client.Disconnect(ctx)
	})
	return NewHandler(db, notification.New(db))
}

// seedPendingOnDay — alohida ish beruvchining shu kundagi e'loni va unga
// ishchining kutilayotgan arizasi.
func seedPendingOnDay(t *testing.T, h *Handler, worker primitive.ObjectID, day string) models.Application {
	t.Helper()
	e := models.Elon{
		ID: primitive.NewObjectID(), OwnerID: primitive.NewObjectID(), Status: "recruiting",
		StartDate: day, WorkTimeFrom: "09:00", WorkersNeeded: 1,
	}
	a := models.Application{
		ID: primitive.NewObjectID(), ElonID: e.ID, EmployerID: e.OwnerID,
		WorkerID: worker, Status: "pending", PeopleCount: 1,
	}
	if _, err := h.Elons.InsertOne(context.Background(), e); err != nil {
		t.Fatal(err)
	}
	if _, err := h.Apps.InsertOne(context.Background(), a); err != nil {
		t.Fatal(err)
	}
	if _, err := h.Users.InsertOne(context.Background(), models.User{ID: e.OwnerID}); err != nil {
		t.Fatal(err)
	}
	return a
}

func acceptRequest(a models.Application) *http.Request {
	r := httptest.NewRequest(http.MethodPost, "/api/applications/"+a.ID.Hex()+"/accept", nil)
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", a.ID.Hex())
	ctx := context.WithValue(r.Context(), chi.RouteCtxKey, rctx)
	ctx = context.WithValue(ctx, httpx.CtxUserID, a.EmployerID.Hex())
	return r.WithContext(ctx)
}

func TestConcurrentAcceptsBookWorkerOncePerDay(t *testing.T) {
	h := dayLockTestHandler(t)
	worker := primitive.NewObjectID()
	if _, err := h.Users.InsertOne(context.Background(), models.User{ID: worker}); err != nil {
		t.Fatal(err)
	}
	const n = 8
	apps := make([]models.Application, n)
	for i := range apps {
		apps[i] = seedPendingOnDay(t, h, worker, "2030-05-10")
	}

	codes := make([]int, n)
	var wg sync.WaitGroup
	start := make(chan struct{})
	for i := range apps {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			<-start
			rec := httptest.NewRecorder()
			h.Accept(rec, acceptRequest(apps[i]))
			codes[i] = rec.Code
		}(i)
	}
	close(start)
	wg.Wait()

	ok := 0
	for _, c := range codes {
		if c == http.StatusOK {
			ok++
		} else if c != http.StatusConflict {
			t.Fatalf("unexpected status %d (all: %v)", c, codes)
		}
	}
	if ok != 1 {
		t.Fatalf("want exactly 1 accepted, got %d (codes %v)", ok, codes)
	}
	accepted, err := h.Apps.CountDocuments(context.Background(), bson.M{"workerId": worker, "status": "accepted"})
	if err != nil {
		t.Fatal(err)
	}
	if accepted != 1 {
		t.Fatalf("want 1 accepted application in DB, got %d", accepted)
	}
}

func TestStaleDayLockIsReclaimed(t *testing.T) {
	h := dayLockTestHandler(t)
	worker := primitive.NewObjectID()
	first := seedPendingOnDay(t, h, worker, "2030-05-11")
	if rec := httptest.NewRecorder(); func() int { h.Accept(rec, acceptRequest(first)); return rec.Code }() != http.StatusOK {
		t.Fatal("first accept failed")
	}
	// Birinchi ish bekor qilindi — qulf o'z joyida qoladi, lekin eskirgan.
	if _, err := h.Apps.UpdateOne(context.Background(), bson.M{"_id": first.ID}, bson.M{"$set": bson.M{"status": "cancelled"}}); err != nil {
		t.Fatal(err)
	}
	second := seedPendingOnDay(t, h, worker, "2030-05-11")
	rec := httptest.NewRecorder()
	h.Accept(rec, acceptRequest(second))
	if rec.Code != http.StatusOK {
		t.Fatalf("second accept after cancellation: want 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var lock struct {
		ApplicationID primitive.ObjectID `bson:"applicationId"`
	}
	if err := h.dayLocks().FindOne(context.Background(), bson.M{"_id": dayLockID(worker, "2030-05-11")}).Decode(&lock); err != nil {
		t.Fatal(err)
	}
	if lock.ApplicationID != second.ID {
		t.Fatalf("lock should now belong to the second application")
	}
}
