package elon

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/ishchibormi/backend/pkg/httpx"
)

type botSourceKey struct{}

// Verify the exact request body when bot signature headers are supplied.
// All clients still require normal JWT authentication and their new listings
// enter the channel queue independently of this optional source signature.
func BotPublicationSource(secret string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			signature, stamp := r.Header.Get("X-Bot-Signature"), r.Header.Get("X-Bot-Timestamp")
			if signature == "" && stamp == "" {
				next.ServeHTTP(w, r)
				return
			}
			body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 1<<20))
			if err != nil || !validPublicationSignature(secret, stamp, signature, r.Method, r.URL.RequestURI(), body, time.Now()) {
				httpx.Err(w, httpx.NewError(401, "invalid_bot_signature", "Bot so'rovini tasdiqlab bo'lmadi."))
				return
			}
			r.Body = io.NopCloser(bytes.NewReader(body))
			next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), botSourceKey{}, true)))
		})
	}
}
func validPublicationSignature(secret, stamp, signature, method, path string, body []byte, now time.Time) bool {
	if len(secret) < 32 || strings.HasPrefix(secret, "change-me") {
		return false
	}
	seconds, err := strconv.ParseInt(stamp, 10, 64)
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
	mac.Write([]byte(stamp + "\n" + method + "\n" + path + "\n"))
	mac.Write(body)
	return hmac.Equal(got, mac.Sum(nil))
}
