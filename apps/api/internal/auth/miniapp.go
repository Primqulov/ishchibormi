package auth

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/ishchibormi/backend/pkg/httpx"
)

const miniAppDataTTL = 10 * time.Minute

// Telegram signs the raw query string. Never authenticate initDataUnsafe,
// a client-supplied Telegram ID, or an unverified phone number.
func verifiedMiniAppData(raw, token string, now time.Time) (url.Values, error) {
	invalid := errors.New("invalid Telegram Mini App data")
	if token == "" || len(raw) == 0 || len(raw) > 16384 {
		return nil, invalid
	}
	values, err := url.ParseQuery(raw)
	if err != nil {
		return nil, invalid
	}
	fields := make([]string, 0, len(values))
	for key, list := range values {
		if len(list) != 1 || strings.ContainsAny(key, "\r\n=") {
			return nil, invalid
		}
		if key != "hash" {
			fields = append(fields, key+"="+list[0])
		}
	}
	got, err := hex.DecodeString(values.Get("hash"))
	if err != nil || len(got) != sha256.Size {
		return nil, invalid
	}
	sort.Strings(fields)
	secret := hmac.New(sha256.New, []byte("WebAppData"))
	secret.Write([]byte(token))
	mac := hmac.New(sha256.New, secret.Sum(nil))
	mac.Write([]byte(strings.Join(fields, "\n")))
	if !hmac.Equal(got, mac.Sum(nil)) {
		return nil, invalid
	}
	stamp, err := strconv.ParseInt(values.Get("auth_date"), 10, 64)
	if err != nil || now.Sub(time.Unix(stamp, 0)) > miniAppDataTTL || time.Unix(stamp, 0).After(now.Add(time.Minute)) {
		return nil, invalid
	}
	return values, nil
}

func (h *Handler) MiniAppSession(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	r.Body = http.MaxBytesReader(w, r.Body, 40<<10)
	var req struct {
		InitData    string `json:"initData"`
		ContactData string `json:"contactData"`
		Consent     bool   `json:"consent"`
	}
	if err := httpx.Decode(r, &req); err != nil {
		httpx.Err(w, err)
		return
	}
	data, err := verifiedMiniAppData(req.InitData, h.cfg.TelegramBotToken, time.Now())
	var user struct {
		ID    int64 `json:"id"`
		IsBot bool  `json:"is_bot"`
	}
	if err != nil || json.Unmarshal([]byte(data.Get("user")), &user) != nil || user.ID <= 0 || user.IsBot {
		httpx.Err(w, httpx.NewError(401, "invalid_miniapp_data", "Telegram sessiyasi tasdiqlanmadi. Mini App'ni botdan qayta oching."))
		return
	}
	phone := ""
	if req.ContactData != "" {
		contactData, err := verifiedMiniAppData(req.ContactData, h.cfg.TelegramBotToken, time.Now())
		var contact struct {
			UserID int64  `json:"user_id"`
			Phone  string `json:"phone_number"`
		}
		if err != nil || json.Unmarshal([]byte(contactData.Get("contact")), &contact) != nil || contact.UserID != user.ID {
			httpx.Err(w, httpx.NewError(400, "bad_contact", "Telegram orqali o'zingizning telefon raqamingizni ulashing."))
			return
		}
		phone = "+" + strings.TrimPrefix(strings.TrimSpace(contact.Phone), "+")
		if !botPhone.MatchString(phone) || !req.Consent {
			httpx.Err(w, httpx.NewError(400, "consent_required", "Shartlarga rozilik bildiring va o'zingizning +998 raqamingizni ulashing."))
			return
		}
	}
	h.telegramSession(w, r, user.ID, phone)
}
