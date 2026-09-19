package application

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/ishchibormi/backend/internal/models"
	"github.com/ishchibormi/backend/pkg/httpx"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

func TestInboxStatusFiltersBeforePagingAndKeepsIdentity(t *testing.T) {
	h, _, original := completionTestHandler(t)
	now := time.Now()
	for i, status := range []string{"pending", "accepted", "completed", "cancelled", "rejected"} {
		a := original
		a.ID = primitive.NewObjectID()
		a.Status = status
		a.AppliedAt = now.Add(time.Duration(i) * time.Minute)
		if _, err := h.Apps.InsertOne(context.Background(), a); err != nil {
			t.Fatal(err)
		}
		a.ID = primitive.NewObjectID()
		a.WorkerID = primitive.NewObjectID()
		a.EmployerID = primitive.NewObjectID()
		if _, err := h.Apps.InsertOne(context.Background(), a); err != nil {
			t.Fatal(err)
		}
	}
	for _, employer := range []bool{false, true} {
		for _, filter := range []string{"pending", "accepted", "history", "nonsense"} {
			r := httptest.NewRequest("GET", "/api/my/applications?status="+filter+"&page=1&limit=1", nil)
			uid := original.WorkerID
			if employer {
				uid = original.EmployerID
			}
			r = r.WithContext(context.WithValue(r.Context(), httpx.CtxUserID, uid.Hex()))
			w := httptest.NewRecorder()
			if employer {
				h.MyElonsApplications(w, r)
			} else {
				h.MyApplications(w, r)
			}
			if filter == "nonsense" {
				if w.Code != 400 {
					t.Fatal("invalid status accepted")
				}
				continue
			}
			if w.Code != 200 {
				t.Fatal(w.Body.String())
			}
			apps := []models.Application{}
			if employer {
				var groups map[string][]models.Application
				if err := json.Unmarshal(w.Body.Bytes(), &groups); err != nil {
					t.Fatal(err)
				}
				for _, group := range groups {
					apps = append(apps, group...)
				}
			} else if err := json.Unmarshal(w.Body.Bytes(), &apps); err != nil {
				t.Fatal(err)
			}
			if len(apps) != 1 {
				t.Fatalf("page: %+v", apps)
			}
			want := filter
			if filter == "history" {
				want = "rejected"
			}
			if apps[0].Status != want || apps[0].WorkerID != original.WorkerID || apps[0].EmployerID != original.EmployerID {
				t.Fatalf("incorrect filtered page: %+v", apps)
			}
		}
	}
}
