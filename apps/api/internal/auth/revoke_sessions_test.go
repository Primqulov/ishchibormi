package auth

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ishchibormi/backend/pkg/httpx"
)

type sessionPair struct {
	AccessToken  string `json:"accessToken"`
	RefreshToken string `json:"refreshToken"`
}

func loginPair(t *testing.T, h *Handler, phone string, tgID int64) sessionPair {
	t.Helper()
	token, code := issueCode(t, h, phone, tgID)
	body := `{"token":"` + token + `","code":"` + code + `"}`
	rec := httptest.NewRecorder()
	h.VerifyOTP(rec, httptest.NewRequest(http.MethodPost, "/api/auth/otp/verify", strings.NewReader(body)))
	if rec.Code != 200 {
		t.Fatalf("login failed: %d %s", rec.Code, rec.Body.String())
	}
	var p sessionPair
	_ = json.Unmarshal(rec.Body.Bytes(), &p)
	return p
}

func refreshStatus(h *Handler, refreshToken string) int {
	rec := httptest.NewRecorder()
	h.Refresh(rec, httptest.NewRequest(http.MethodPost, "/api/auth/refresh",
		strings.NewReader(`{"refreshToken":"`+refreshToken+`"}`)))
	return rec.Code
}

func revoke(t *testing.T, h *Handler, access string) sessionPair {
	t.Helper()
	var chain http.Handler = http.HandlerFunc(h.RevokeSessions)
	chain = RequireActiveUser(h.Users())(chain)
	chain = httpx.UserAuth(loginTestConfig().JWTAccessSecret)(chain)
	req := httptest.NewRequest(http.MethodPost, "/api/me/sessions/revoke", nil)
	req.Header.Set("Authorization", "Bearer "+access)
	rec := httptest.NewRecorder()
	chain.ServeHTTP(rec, req)
	if rec.Code != 200 {
		t.Fatalf("revoke failed: %d %s", rec.Code, rec.Body.String())
	}
	var p sessionPair
	_ = json.Unmarshal(rec.Body.Bytes(), &p)
	return p
}

// "Barcha qurilmalardan chiqish" — boshqa qurilmadagi (masalan o'g'irlangan)
// access ham, refresh ham darhol o'lishi, chaqirgan qurilma esa ishlashda
// davom etishi kerak.
func TestRevokeSessionsKillsOtherDevices(t *testing.T) {
	h := NewHandler(loginTestConfig(), testDB(t))
	stolen := loginPair(t, h, "+998900000091", 777091)
	mine := loginPair(t, h, "+998900000091", 777091)

	if s, _ := callProtected(t, h, stolen.AccessToken); s != 200 {
		t.Fatalf("precondition: stolen access should work before revoke, got %d", s)
	}
	fresh := revoke(t, h, mine.AccessToken)

	if s, code := callProtected(t, h, stolen.AccessToken); s != 401 || code != "session_revoked" {
		t.Fatalf("old access token still accepted: %d %s", s, code)
	}
	if s := refreshStatus(h, stolen.RefreshToken); s != 401 {
		t.Fatalf("old refresh token still accepted: %d", s)
	}
	if s, code := callProtected(t, h, fresh.AccessToken); s != 200 {
		t.Fatalf("caller's new access rejected: %d %s", s, code)
	}
	if s := refreshStatus(h, fresh.RefreshToken); s != 200 {
		t.Fatalf("caller's new refresh rejected: %d", s)
	}
}

// Bu funksiyadan oldin chiqarilgan (sv'siz) tokenlar ishlashda davom etadi —
// deploy hech kimni hisobdan chiqarib yubormasligi kerak.
func TestLegacyTokenWithoutVersionStillWorks(t *testing.T) {
	h := NewHandler(loginTestConfig(), testDB(t))
	p := loginPair(t, h, "+998900000092", 777092)
	uid, _, err := httpx.ParseUserSessionToken(loginTestConfig().JWTAccessSecret, p.AccessToken)
	if err != nil {
		t.Fatal(err)
	}
	cfg := loginTestConfig()
	legacyAccess, _ := httpx.IssueUserToken(cfg.JWTAccessSecret, uid, cfg.JWTAccessTTL)
	legacyRefresh, _ := httpx.IssueUserToken(cfg.JWTRefreshSecret, uid, cfg.JWTRefreshTTL)
	if s, code := callProtected(t, h, legacyAccess); s != 200 {
		t.Fatalf("legacy access rejected: %d %s", s, code)
	}
	if s := refreshStatus(h, legacyRefresh); s != 200 {
		t.Fatalf("legacy refresh rejected: %d", s)
	}
}
