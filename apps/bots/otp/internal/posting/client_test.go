package posting

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestAPIClientUsesSignedLoginAndOrdinaryPublicationRoutes(t *testing.T) {
	secret := strings.Repeat("s", 32)
	var posted Form
	var uploaded bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/auth/bot/session":
			body, _ := io.ReadAll(r.Body)
			mac := hmac.New(sha256.New, []byte(secret))
			mac.Write([]byte(r.Header.Get("X-Bot-Timestamp") + "\nPOST\n/api/auth/bot/session\n"))
			mac.Write(body)
			if hex.EncodeToString(mac.Sum(nil)) != r.Header.Get("X-Bot-Signature") {
				t.Error("incorrect signature")
			}
			if !strings.Contains(string(body), `"contactUserId":42`) {
				t.Error("own contact missing")
			}
			_, _ = io.WriteString(w, `{"accessToken":"jwt","user":{"id":"owner"}}`)
		case "/api/elons":
			if r.Header.Get("Authorization") != "Bearer jwt" || r.Header.Get("Idempotency-Key") != "draft-key" {
				t.Error("missing authenticated/idempotent request")
			}
			body, _ := io.ReadAll(r.Body)
			mac := hmac.New(sha256.New, []byte(secret))
			mac.Write([]byte(r.Header.Get("X-Bot-Timestamp") + "\nPOST\n/api/elons\n"))
			mac.Write(body)
			if hex.EncodeToString(mac.Sum(nil)) != r.Header.Get("X-Bot-Signature") {
				t.Error("publication source signature missing")
			}
			_ = json.Unmarshal(body, &posted)
			_, _ = io.WriteString(w, `{"id":"listing"}`)
		case "/api/uploads":
			if r.URL.Query().Get("kind") != "elon" {
				t.Error("wrong upload kind")
			}
			f, _, err := r.FormFile("file")
			if err != nil {
				t.Error(err)
			} else {
				defer f.Close()
				data, _ := io.ReadAll(f)
				uploaded = string(data) == "photo bytes"
			}
			_, _ = io.WriteString(w, `{"url":"https://files.test/photo.jpg"}`)
		default:
			t.Errorf("unexpected API route %s", r.URL.Path)
		}
	}))
	defer server.Close()
	c, err := NewClient(server.URL, secret)
	if err != nil {
		t.Fatal(err)
	}
	s, err := c.Login(context.Background(), 42, "+998901234567")
	if err != nil {
		t.Fatal(err)
	}
	_, err = c.Upload(context.Background(), s, []byte("photo bytes"))
	if err != nil || !uploaded {
		t.Fatalf("upload failed %v", err)
	}
	_, err = c.Publish(context.Background(), s, "draft-key", Form{Title: "Ish", PricingType: "total", WorkersNeeded: 3, Images: []string{"https://files.test/photo.jpg"}})
	if err != nil || posted.WorkersNeeded != 3 || posted.PricingType != "total" || len(posted.Images) != 1 {
		t.Fatalf("wrong publication: %+v %v", posted, err)
	}
}
func TestAPIClientPreservesModerationWarning(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(422)
		_, _ = io.WriteString(w, `{"error":{"code":"content_rejected","message":"Tavsif rad etildi","details":{"warning":"Yana buzilish bo'lsa bloklanadi"}}}`)
	}))
	defer server.Close()
	c, _ := NewClient(server.URL, strings.Repeat("x", 32))
	_, err := c.Publish(context.Background(), Session{}, "key", Form{})
	if !definiteRejection(err) || !strings.Contains(userError(err), "bloklanadi") {
		t.Fatalf("moderation reason/warning lost: %v", err)
	}
}

func TestNearbyClientSendsLocationAndPageWithoutAuthentication(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		if r.URL.Path != "/api/elons/nearby" || q.Get("lat") != "41.3" || q.Get("lng") != "69.2" || q.Get("page") != "2" || q.Get("limit") != "5" || r.Header.Get("Authorization") != "" {
			t.Errorf("wrong nearby request: %s", r.URL)
		}
		_, _ = io.WriteString(w, `{"items":[{"id":"listing","distanceMeters":123.4}],"page":2,"limit":5,"total":8}`)
	}))
	defer server.Close()
	c, _ := NewClient(server.URL, strings.Repeat("s", 32))
	p, err := c.Nearby(context.Background(), 41.3, 69.2, 2)
	if err != nil || p.Total != 8 || len(p.Items) != 1 || p.Items[0].DistanceMeters != 123.4 {
		t.Fatalf("distance response lost: %+v %v", p, err)
	}
}
