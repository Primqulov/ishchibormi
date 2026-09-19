package auth

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/ishchibormi/backend/internal/channelpost"
	"github.com/ishchibormi/backend/internal/elon"
	"github.com/ishchibormi/backend/internal/models"
	"github.com/ishchibormi/backend/pkg/httpx"
	"github.com/ishchibormi/backend/pkg/tgsend"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

const miniTestToken = "123456789:test-miniapp-token"

func signedMiniData(values url.Values, token string) string {
	var lines []string
	for key := range values {
		if key != "hash" {
			lines = append(lines, key+"="+values.Get(key))
		}
	}
	sort.Strings(lines)
	secret := hmac.New(sha256.New, []byte("WebAppData"))
	secret.Write([]byte(token))
	mac := hmac.New(sha256.New, secret.Sum(nil))
	mac.Write([]byte(strings.Join(lines, "\n")))
	values.Set("hash", hex.EncodeToString(mac.Sum(nil)))
	return values.Encode()
}

func TestMiniAppSignatureRejectsAlteredExpiredDuplicateAndWrongBotData(t *testing.T) {
	now := time.Now()
	values := url.Values{"auth_date": {strconv.FormatInt(now.Unix(), 10)}, "query_id": {"query+id"}, "user": {`{"id":42,"first_name":"Ali & Vali"}`}, "signature": {"telegram-extra-signature"}}
	valid := signedMiniData(values, miniTestToken)
	if _, err := verifiedMiniAppData(valid, miniTestToken, now); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		raw, token string
		now        time.Time
	}{
		{strings.Replace(valid, "42", "43", 1), miniTestToken, now},
		{valid, "different-bot", now},
		{valid, "", now},
		{valid, miniTestToken, now.Add(11 * time.Minute)},
		{valid, miniTestToken, now.Add(-2 * time.Minute)},
		{valid + "&user=%7B%22id%22%3A43%7D", miniTestToken, now},
		{valid + "&hash=duplicate", miniTestToken, now},
		{"auth_date=1&user=%7B%22id%22%3A42%7D", miniTestToken, now},
	} {
		if _, err := verifiedMiniAppData(tc.raw, tc.token, tc.now); err == nil {
			t.Fatal("invalid Mini App signature accepted")
		}
	}
}

func TestMiniAppSessionUsesVerifiedContactAndSameAccountWithBlockChecks(t *testing.T) {
	cfg := loginTestConfig()
	cfg.TelegramBotToken = miniTestToken
	h := NewHandler(cfg, testDB(t))
	stamp := strconv.FormatInt(time.Now().Unix(), 10)
	initData := signedMiniData(url.Values{"auth_date": {stamp}, "user": {`{"id":7542001,"first_name":"Ali"}`}}, miniTestToken)
	contact := func(id string) string {
		return signedMiniData(url.Values{"auth_date": {stamp}, "contact": {`{"user_id":` + id + `,"phone_number":"998901237411"}`}}, miniTestToken)
	}
	call := func(init, proof string, consent bool) (int, models.User, string) {
		t.Helper()
		body, _ := json.Marshal(map[string]any{"initData": init, "contactData": proof, "consent": consent})
		w := httptest.NewRecorder()
		h.MiniAppSession(w, httptest.NewRequest("POST", "/api/auth/miniapp/session", bytes.NewReader(body)))
		var out struct {
			User  models.User `json:"user"`
			Token string      `json:"accessToken"`
		}
		_ = json.Unmarshal(w.Body.Bytes(), &out)
		if w.Header().Get("Cache-Control") != "no-store" {
			t.Fatal("session response is cacheable")
		}
		return w.Code, out.User, out.Token
	}
	if status, _, _ := call("", "", false); status != 401 {
		t.Fatal("unsigned Mini App authenticated")
	}
	if status, _, _ := call(initData, "", false); status != 401 {
		t.Fatal("new user skipped phone verification")
	}
	if status, _, _ := call(initData, contact("7542002"), true); status != 400 {
		t.Fatal("foreign contact accepted")
	}
	if status, _, _ := call(initData, contact("7542001"), false); status != 400 {
		t.Fatal("registration skipped consent")
	}
	status, user, token := call(initData, contact("7542001"), true)
	if status != 200 || user.ID.IsZero() || user.TelegramID != 7542001 || !user.IsPhoneVerified {
		t.Fatalf("registration failed: %d %+v", status, user)
	}
	if status, _ := callProtected(t, h, token); status != 200 {
		t.Fatal("Mini App token cannot use ordinary API")
	}
	assertMiniAppPosting(t, h, token, user.ID)
	status, known, _ := call(initData, "", false)
	if status != 200 || known.ID != user.ID {
		t.Fatal("existing user had to register again")
	}
	if _, err := h.users.UpdateOne(context.Background(), bson.M{"_id": user.ID}, bson.M{"$set": bson.M{"isBlocked": true}}); err != nil {
		t.Fatal(err)
	}
	if status, _, _ := call(initData, "", false); status != 403 {
		t.Fatal("blocked Mini App user authenticated")
	}
}

// Exercise the real creation handler with the Mini App's JWT, including the
// ordinary active-account guard and channel queue. No Telegram sender is run.
func assertMiniAppPosting(t *testing.T, auth *Handler, token string, owner primitive.ObjectID) {
	t.Helper()
	ctx := context.Background()
	db := auth.Users().Database()
	jobs := elon.NewHandler(db, nil, nil)
	channel, err := channelpost.New(db, tgsend.New("test-no-network"), "@miniapp_test", "miniappbot", slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatal(err)
	}
	jobs.Channel = channel
	cat := primitive.NewObjectID()
	if _, err := db.Collection("categories").InsertOne(ctx, models.Category{ID: cat, Name: "Tozalash", IsActive: true}); err != nil {
		t.Fatal(err)
	}
	start := time.Now().In(time.FixedZone("Asia/Tashkent", 5*3600)).Add(2 * time.Hour)
	body, _ := json.Marshal(map[string]any{
		"title": "Hovlini tozalash", "description": "Mini App orqali ish",
		"categoryId": cat.Hex(), "workersNeeded": 3, "pricingType": "per_worker", "priceAmount": 250000,
		"startDate": start.Format("2006-01-02"), "workTimeFrom": start.Format("15:04"), "contactPhone": "+998000000001",
	})
	handler := httpx.UserAuth(auth.cfg.JWTAccessSecret)(RequireActiveUser(auth.Users())(http.HandlerFunc(jobs.Create)))
	key := primitive.NewObjectID().Hex()
	var first models.Elon
	for attempt := 0; attempt < 2; attempt++ {
		r := httptest.NewRequest("POST", "/api/elons", bytes.NewReader(body))
		r.Header.Set("Authorization", "Bearer "+token)
		r.Header.Set("Idempotency-Key", key)
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)
		wantStatus := 201
		if attempt > 0 {
			wantStatus = 200
		}
		if w.Code != wantStatus {
			t.Fatalf("Mini App create: %d %s", w.Code, w.Body.String())
		}
		var job models.Elon
		if err := json.Unmarshal(w.Body.Bytes(), &job); err != nil {
			t.Fatal(err)
		}
		if job.OwnerID != owner || job.WorkersNeeded != 3 || job.PerWorkerAmount != 250000 {
			t.Fatal("Mini App listing ownership or pricing lost")
		}
		if attempt == 0 {
			first = job
		} else if job.ID != first.ID {
			t.Fatal("Mini App retry created another listing")
		}
	}
	var saved models.Elon
	if err := jobs.Col.FindOne(ctx, bson.M{"_id": first.ID}).Decode(&saved); err != nil {
		t.Fatal(err)
	}
	if saved.TelegramChannel == nil || saved.TelegramChannel.Status != "pending" || saved.TelegramChannel.Reference != "@miniapp_test" {
		t.Fatal("Mini App listing did not enter the shared channel queue")
	}
}
