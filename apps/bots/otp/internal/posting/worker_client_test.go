package posting

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestWorkerClientUsesPlatformRoutesAndVerifiedSession(t *testing.T) {
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.Header.Get("Authorization") != "Bearer worker-jwt" {
			t.Error("missing worker authentication")
		}
		switch r.URL.Path {
		case "/api/elons/" + workerJobID:
			if r.Method != "GET" {
				t.Error("wrong job method")
			}
			_, _ = io.WriteString(w, `{"id":"`+workerJobID+`","status":"recruiting","title":"Ish","images":["https://files.test/first.jpg","https://files.test/second.webp"]}`)
		case "/api/elons/" + workerJobID + "/apply":
			var body map[string]any
			_ = json.NewDecoder(r.Body).Decode(&body)
			if r.Method != "POST" || body["peopleCount"] != float64(2) || body["phone"] != nil || body["workerId"] != nil {
				t.Error("apply must use verified account and group size")
			}
			_, _ = io.WriteString(w, `{"id":"`+workerAppID+`","status":"pending"}`)
		case "/api/my/applications":
			if r.URL.Query().Get("page") != "2" || r.URL.Query().Get("limit") != "5" || r.URL.Query().Get("status") != "accepted" {
				t.Error("list filters missing")
			}
			_, _ = io.WriteString(w, `[]`)
		case "/api/my/elons/applications":
			_, _ = io.WriteString(w, `{"first":[{"id":"a","appliedAt":"2026-09-01"}],"second":[{"id":"b","appliedAt":"2026-09-02"}]}`)
		case "/api/applications/" + workerAppID:
			_, _ = io.WriteString(w, `{"id":"`+workerAppID+`","status":"accepted"}`)
		case "/api/applications/" + workerAppID + "/cancel":
			var body map[string]string
			_ = json.NewDecoder(r.Body).Decode(&body)
			if r.Method != "POST" || body["reason"] != "Bora olmayman" {
				t.Error("cancellation reason missing")
			}
			_, _ = io.WriteString(w, `{"status":"cancelled"}`)
		case "/api/applications/" + workerAppID + "/confirm-done":
			if r.Method != "POST" {
				t.Error("wrong completion method")
			}
			_, _ = io.WriteString(w, `{"status":"awaiting_other"}`)
		default:
			t.Errorf("unexpected worker route: %s", r.URL.Path)
		}
	}))
	defer server.Close()
	c, _ := NewClient(server.URL, strings.Repeat("s", 32))
	ctx := context.Background()
	s := Session{AccessToken: "worker-jwt"}
	j, err := c.Job(ctx, s, workerJobID)
	if err != nil || j.ID != workerJobID || len(j.Images) != 2 || j.Images[1] != "https://files.test/second.webp" {
		t.Fatal("job decoding failed", err)
	}
	a, err := c.Apply(ctx, s, workerJobID, 2)
	if err != nil || a.Status != "pending" {
		t.Fatal("apply failed", err)
	}
	if _, err := c.Applications(ctx, s, "accepted", 2, false); err != nil {
		t.Fatal(err)
	}
	apps, err := c.Applications(ctx, s, "pending", 1, true)
	if err != nil || len(apps) != 2 || apps[0].ID != "b" {
		t.Fatal("group flatten order", err)
	}
	if _, err := c.Application(ctx, s, workerAppID); err != nil {
		t.Fatal(err)
	}
	if _, err := c.ApplicationAction(ctx, s, workerAppID, "cancel", "Bora olmayman"); err != nil {
		t.Fatal(err)
	}
	if status, err := c.ApplicationAction(ctx, s, workerAppID, "confirm-done", ""); err != nil || status != "awaiting_other" {
		t.Fatal("dual confirmation response", err)
	}
	if calls != 7 {
		t.Fatal("incomplete worker contract")
	}
}
