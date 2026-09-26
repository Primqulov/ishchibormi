package auth

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/ishchibormi/backend/internal/models"
	"github.com/ishchibormi/backend/internal/moderation"
	"github.com/ishchibormi/backend/pkg/httpx"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
)

var botPhone = regexp.MustCompile(`^\+998[0-9]{9}$`)

// BotSession accepts identity assertions from our Telegram poller only. A
// Telegram ID submitted by an ordinary HTTP client is never authentication.
// The short-lived signature covers the exact body, endpoint and timestamp.
// Subsequent writes use the ordinary JWT routes (including bans/moderation).
func (h *Handler) BotSession(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 2048))
	if err != nil || !validBotSignature(h.cfg.BotSharedSecret, r.Header.Get("X-Bot-Timestamp"), r.Header.Get("X-Bot-Signature"), body, time.Now()) {
		httpx.Err(w, httpx.NewError(401, "unauthorized", "bot authentication required"))
		return
	}
	var req struct {
		TelegramID    int64  `json:"telegramId"`
		Phone         string `json:"phone"`
		ContactUserID int64  `json:"contactUserId"`
	}
	if json.Unmarshal(body, &req) != nil || req.TelegramID <= 0 ||
		(req.Phone != "" && (!botPhone.MatchString(req.Phone) || req.ContactUserID != req.TelegramID)) {
		httpx.Err(w, httpx.NewError(400, "bad_contact", "O'zingizning +998 telefon raqamingizni yuboring."))
		return
	}
	h.telegramSession(w, r, req.TelegramID, req.Phone)
}

// Shared account/block checks for identities verified by the poller or Mini App.
func (h *Handler) telegramSession(w http.ResponseWriter, r *http.Request, telegramID int64, phone string) {
	ctx := r.Context()
	var u models.User
	err := h.users.FindOne(ctx, bson.M{"telegramId": telegramID, "isDeleted": bson.M{"$ne": true}}).Decode(&u)
	if err != nil && !errors.Is(err, mongo.ErrNoDocuments) {
		httpx.Err(w, err)
		return
	}
	if err == nil && (u.IsBlocked || u.IsReviewAccount) {
		httpx.Err(w, httpx.NewError(403, "account_blocked", "Hisobingizdan e'lon berish cheklangan."))
		return
	}
	if phone == "" && u.IsPhoneVerified {
		phone = u.Phone
	}
	if phone == "" {
		httpx.Err(w, httpx.NewError(401, "contact_required", "Telefon raqamingizni ulashing."))
		return
	}
	if !u.ID.IsZero() && u.Phone != phone {
		httpx.Err(w, httpx.NewError(409, "phone_conflict", "Telegram hisobingiz boshqa raqamga bog'langan. Qo'llab-quvvatlash xizmatiga murojaat qiling."))
		return
	}
	until, banned, err := h.strikes.BanByPhone(ctx, phone)
	if err != nil {
		httpx.Err(w, httpx.NewError(503, "account_unavailable", "Hisobni tekshirib bo'lmadi. Qayta urinib ko'ring."))
		return
	}
	if banned {
		httpx.Err(w, httpx.NewErrorWithDetails(403, "account_banned", moderation.BanMessage(until), map[string]any{"bannedUntil": until}))
		return
	}
	if u.ModerationBannedUntil != nil && u.ModerationBannedUntil.After(time.Now()) {
		httpx.Err(w, httpx.NewError(403, "account_banned", moderation.BanMessage(*u.ModerationBannedUntil)))
		return
	}
	if u.ID.IsZero() || !u.IsPhoneVerified {
		// Platforma bot yuborgan sarlavhadan olinadi (X-Client-Platform:
		// telegram). Eski bot versiyasi uni yubormaydi -> bo'sh satr, ya'ni
		// mavjud hisobning platformasi o'zgarmaydi.
		user, err := h.upsertUser(ctx, phone, telegramID, httpx.ClientPlatform(r), httpx.ClientDevice(r))
		if errors.Is(err, errAccountBlocked) {
			httpx.Err(w, httpx.NewError(403, "account_blocked", "Hisobingiz bloklangan."))
			return
		}
		if err != nil {
			httpx.Err(w, err)
			return
		}
		u = *user
	}
	// No refresh credentials need to be persisted by the bot.
	access, err := httpx.IssueUserSessionToken(h.cfg.JWTAccessSecret, u.ID.Hex(), u.SessionVersion, h.cfg.JWTAccessTTL)
	if err != nil {
		httpx.Err(w, err)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	httpx.JSON(w, 200, map[string]any{"accessToken": access, "user": u, "botUsername": h.cfg.TelegramBotUsername})
}

func validBotSignature(secret, timestamp, signature string, body []byte, now time.Time) bool {
	if len(secret) < 32 || strings.HasPrefix(secret, "change-me") {
		return false
	}
	seconds, err := strconv.ParseInt(timestamp, 10, 64)
	if err != nil {
		return false
	}
	delta := now.Sub(time.Unix(seconds, 0))
	if delta < -time.Minute || delta > time.Minute {
		return false
	}
	got, err := hex.DecodeString(signature)
	if err != nil {
		return false
	}
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(timestamp + "\nPOST\n/api/auth/bot/session\n"))
	mac.Write(body)
	return hmac.Equal(got, mac.Sum(nil))
}
