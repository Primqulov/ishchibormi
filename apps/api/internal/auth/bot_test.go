package auth

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/ishchibormi/backend/internal/models"
	"go.mongodb.org/mongo-driver/bson"
)

const testBotSecret = "a-long-test-only-bot-shared-secret-123456"

func signBot(secret, ts string, body []byte) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(ts + "\nPOST\n/api/auth/bot/session\n"))
	mac.Write(body)
	return hex.EncodeToString(mac.Sum(nil))
}
func botRequest(body []byte) *http.Request {
	r := httptest.NewRequest("POST", "/api/auth/bot/session", bytes.NewReader(body))
	ts := strconv.FormatInt(time.Now().Unix(), 10)
	r.Header.Set("X-Bot-Timestamp", ts)
	r.Header.Set("X-Bot-Signature", signBot(testBotSecret, ts, body))
	return r
}
func TestBotSignatureRejectsTamperingAndExpiredAssertions(t *testing.T) {
	now := time.Now()
	ts := strconv.FormatInt(now.Unix(), 10)
	body := []byte(`{"telegramId":42}`)
	sig := signBot(testBotSecret, ts, body)
	if !validBotSignature(testBotSecret, ts, sig, body, now) {
		t.Fatal("valid assertion refused")
	}
	for _, tc := range []struct {
		name, secret, ts, sig string
		body                  []byte
		now                   time.Time
	}{
		{"tampered body", testBotSecret, ts, sig, []byte(`{"telegramId":43}`), now},
		{"expired", testBotSecret, ts, sig, body, now.Add(2 * time.Minute)},
		{"future", testBotSecret, ts, sig, body, now.Add(-2 * time.Minute)},
		{"no signature", testBotSecret, ts, "", body, now},
		{"weak default", "dev-shared", ts, signBot("dev-shared", ts, body), body, now},
		{"other secret", strings.Repeat("b", 32), ts, sig, body, now},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if validBotSignature(tc.secret, tc.ts, tc.sig, tc.body, tc.now) {
				t.Fatal("unsafe assertion accepted")
			}
		})
	}
}
func TestBotSessionRejectsUnauthenticatedAndForeignContactsBeforeDB(t *testing.T) {
	cfg := loginTestConfig()
	cfg.BotSharedSecret = testBotSecret
	h := &Handler{cfg: cfg}
	for _, tc := range []struct {
		body   string
		signed bool
		status int
	}{
		{`{"telegramId":42}`, false, 401},
		{`{"telegramId":42,"phone":"+998901234567","contactUserId":99}`, true, 400},
		{`{"telegramId":0}`, true, 400},
		{`{"telegramId":42,"phone":"+1901234567","contactUserId":42}`, true, 400},
	} {
		r := botRequest([]byte(tc.body))
		if !tc.signed {
			r.Header.Del("X-Bot-Signature")
		}
		w := httptest.NewRecorder()
		h.BotSession(w, r)
		if w.Code != tc.status {
			t.Fatalf("%s: %d %s", tc.body, w.Code, w.Body.String())
		}
	}
}
func TestBotSessionSharesPlatformAccountAndEnforcesBlocks(t *testing.T) {
	db := testDB(t)
	cfg := loginTestConfig()
	cfg.BotSharedSecret = testBotSecret
	h := NewHandler(cfg, db)
	call := func(body string) (int, models.User, string) {
		t.Helper()
		w := httptest.NewRecorder()
		h.BotSession(w, botRequest([]byte(body)))
		var out struct {
			User        models.User `json:"user"`
			AccessToken string      `json:"accessToken"`
		}
		_ = json.Unmarshal(w.Body.Bytes(), &out)
		return w.Code, out.User, out.AccessToken
	}
	status, _, _ := call(`{"telegramId":912345}`)
	if status != 401 {
		t.Fatal("unknown Telegram ID authenticated without contact")
	}
	status, u, token := call(`{"telegramId":912345,"phone":"+998901239991","contactUserId":912345}`)
	if status != 200 || u.ID.IsZero() || token == "" {
		t.Fatalf("new user failed: %d", status)
	}
	if status, _ := callProtected(t, h, token); status != 200 {
		t.Fatal("bot session cannot use ordinary API")
	}
	status, known, _ := call(`{"telegramId":912345}`)
	if status != 200 || known.ID != u.ID {
		t.Fatal("bot created a separate platform identity")
	}
	_, err := h.users.UpdateOne(context.Background(), bson.M{"_id": u.ID}, bson.M{"$set": bson.M{"isBlocked": true}})
	if err != nil {
		t.Fatal(err)
	}
	status, _, _ = call(`{"telegramId":912345}`)
	if status != 403 {
		t.Fatal("blocked account authenticated")
	}
}
