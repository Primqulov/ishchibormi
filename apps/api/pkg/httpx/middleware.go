package httpx

import (
	"context"
	"log"
	"net/http"
	"runtime/debug"
	"strings"
	"sync"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

type ctxKey string

const (
	CtxUserID    ctxKey = "userId"
	CtxAdminID   ctxKey = "adminId"
	CtxAdminRole ctxKey = "adminRole"
	CtxAdminVer  ctxKey = "adminTokenVersion"
	// CtxUserSessionVer — foydalanuvchi tokenidagi sessiya versiyasi (sv).
	// auth.RequireActiveUser uni User.SessionVersion bilan solishtiradi.
	CtxUserSessionVer ctxKey = "userSessionVersion"
	// CtxReviewActor marks a request made by the sandboxed Google Play review
	// account. auth.RequireActiveUser sets it (it already has the user
	// document, so this costs no extra query) and handlers consult it to keep
	// review activity from reaching real users. Never true for a normal user.
	CtxReviewActor ctxKey = "reviewActor"
)

// TrustProxyHeaders controls whether the X-Forwarded-For header is honored when
// determining the client IP (used for rate limiting). It MUST stay false unless
// the service runs behind a trusted reverse proxy; otherwise a client can spoof
// XFF to get an unlimited number of rate-limit buckets and defeat brute-force
// protection. Set from config at startup.
var TrustProxyHeaders = false

// TrustedProxyHops is how many reverse proxies sit between the internet and
// this process. It selects which X-Forwarded-For element is the real client:
// proxies APPEND, so with one hop (Caddy in deploy/Caddyfile) the client
// address is the LAST element, with two (CDN -> Caddy) the second-to-last, and
// so on. Only meaningful when TrustProxyHeaders is true. Set from config at
// startup; values below 1 are treated as 1.
var TrustedProxyHops = 1

// allowedJWTMethods pins token verification to HMAC-SHA256, preventing
// algorithm-confusion / "alg:none" downgrade attacks.
var allowedJWTMethods = []string{"HS256"}

// AccessLog deliberately records URL.Path rather than RequestURI. Query
// parameters include dev OTP lookup tokens and upload URLs; logging the raw URI
// would turn ordinary access logs into a credential/data leak.
func AccessLog(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		started := time.Now()
		next.ServeHTTP(w, r)
		log.Printf("http method=%s path=%s remote=%s duration=%s",
			r.Method, r.URL.Path, clientIP(r), time.Since(started).Round(time.Millisecond))
	})
}

type Claims struct {
	UserID string `json:"uid"`
	// SessionVersion — token chiqarilgan paytdagi User.SessionVersion. Versiya
	// oshirilsa (boshqa qurilmalardan chiqish), eski tokenlarning hammasi
	// kuchini yo'qotadi. omitempty: 0-versiya tokenlar bu funksiyadan oldingi
	// tokenlar bilan bir xil ko'rinadi, ya'ni eski sessiyalar uzilmaydi.
	SessionVersion int `json:"sv,omitempty"`
	jwt.RegisteredClaims
}

type AdminClaims struct {
	AdminID      string `json:"aid"`
	Role         string `json:"role"`
	TokenVersion *int   `json:"ver"`
	jwt.RegisteredClaims
}

func UserAuth(secret string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			tok := tokenFromReq(r)
			if tok == "" {
				Err(w, NewError(http.StatusUnauthorized, "no_token", "missing token"))
				return
			}
			c := &Claims{}
			_, err := jwt.ParseWithClaims(tok, c,
				func(*jwt.Token) (any, error) { return []byte(secret), nil },
				jwt.WithValidMethods(allowedJWTMethods))
			if err != nil || c.UserID == "" {
				Err(w, NewError(http.StatusUnauthorized, "bad_token", "invalid token"))
				return
			}
			ctx := context.WithValue(r.Context(), CtxUserID, c.UserID)
			ctx = context.WithValue(ctx, CtxUserSessionVer, c.SessionVersion)
			setActor(ctx, c.UserID)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// OptionalUserAuth reads a Bearer token when one is present and never rejects a
// request. Public listing routes use it so they can tailor results to the
// caller while staying open to anonymous visitors.
//
// reviewUserID is the id of the Play review account, or "" when no review
// window is open. Matching the token's subject against it identifies review
// traffic without a database round-trip on this hot path — and it cannot be
// forged, since producing such a token means holding a real session for that
// account. Passing "" disables the check entirely.
func OptionalUserAuth(secret, reviewUserID string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			tok := tokenFromReq(r)
			if tok == "" {
				next.ServeHTTP(w, r)
				return
			}
			c := &Claims{}
			if _, err := jwt.ParseWithClaims(tok, c,
				func(*jwt.Token) (any, error) { return []byte(secret), nil },
				jwt.WithValidMethods(allowedJWTMethods),
			); err != nil || c.UserID == "" {
				// A bad token on a public route is simply an anonymous visitor.
				next.ServeHTTP(w, r)
				return
			}
			ctx := context.WithValue(r.Context(), CtxUserID, c.UserID)
			setActor(ctx, c.UserID)
			if reviewUserID != "" && c.UserID == reviewUserID {
				ctx = context.WithValue(ctx, CtxReviewActor, true)
			}
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

func AdminAuth(secret string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			tok := tokenFromReq(r)
			if tok == "" {
				Err(w, NewError(http.StatusUnauthorized, "no_token", "missing token"))
				return
			}
			c := &AdminClaims{}
			_, err := jwt.ParseWithClaims(tok, c,
				func(*jwt.Token) (any, error) { return []byte(secret), nil },
				jwt.WithValidMethods(allowedJWTMethods))
			// A pointer distinguishes a new token carrying `ver: 0` from a
			// legacy privileged token where the version claim is absent entirely.
			if err != nil || c.AdminID == "" || c.TokenVersion == nil {
				Err(w, NewError(http.StatusUnauthorized, "bad_token", "invalid token"))
				return
			}
			ctx := context.WithValue(r.Context(), CtxAdminID, c.AdminID)
			ctx = context.WithValue(ctx, CtxAdminRole, c.Role)
			ctx = context.WithValue(ctx, CtxAdminVer, *c.TokenVersion)
			setActor(ctx, c.AdminID)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// ── So'rov egasini TASHQI middleware'ga yetkazish ────────────────────────
//
// Muammo: kontekst faqat PASTGA oqadi. Autentifikatsiya middleware'i
// `r.WithContext(...)` bilan YANGI so'rov yasab, uni ichkariga uzatadi —
// undan tashqarida turgan middleware (xatolik jurnali, panic ilgagi) esa
// eski so'rovni ushlab qoladi va u yerda hech qachon foydalanuvchi id'si
// bo'lmaydi.
//
// Yechim: tashqi middleware kontekstga o'zgaruvchan QUTI qo'yadi, ichkaridagi
// auth esa unga id yozadi. Shu tariqa "Ta'sirlangan foydalanuvchi"
// ko'rsatkichi ishlaydi, auth middleware'lari esa xatolik jurnali haqida
// hech narsa bilmaydi.
//
// Quti faqat ID saqlaydi va u hech qayerga XOM holda yozilmaydi:
// internal/errlog uni tuzlangan hash'ga aylantiradi.
type actorBox struct {
	mu sync.Mutex
	id string
}

const ctxActor ctxKey = "actorBox"

// ActorSink so'rovga bo'sh quti biriktiradi. Zanjirda auth'dan TASHQARIDA,
// Recover'dan ham oldin turishi kerak: panic ilgagi ham, xatolik jurnali
// ham aynan shu qutini o'qiydi.
func ActorSink(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), ctxActor, &actorBox{})))
	})
}

func setActor(ctx context.Context, id string) {
	if b, ok := ctx.Value(ctxActor).(*actorBox); ok && id != "" {
		b.mu.Lock()
		b.id = id
		b.mu.Unlock()
	}
}

// Actor — so'rov egasi (foydalanuvchi yoki admin id'si), quti o'rnatilgan
// bo'lsa. Aks holda bo'sh satr.
func Actor(r *http.Request) string {
	if b, ok := r.Context().Value(ctxActor).(*actorBox); ok {
		b.mu.Lock()
		defer b.mu.Unlock()
		return b.id
	}
	return ""
}

// tokenFromReq reads the Bearer token from the Authorization header only.
// A ?token= query fallback used to exist for <a>-tag CSV downloads, but URL
// tokens leak into proxy logs and browser history; the admin UI now downloads
// via fetch with a proper header, so the fallback is gone on purpose.
func tokenFromReq(r *http.Request) string {
	h := r.Header.Get("Authorization")
	if strings.HasPrefix(h, "Bearer ") {
		return strings.TrimPrefix(h, "Bearer ")
	}
	return ""
}

func UserID(r *http.Request) string {
	if v, ok := r.Context().Value(CtxUserID).(string); ok {
		return v
	}
	return ""
}

// UserSessionVersion — so'rov tokenidagi sessiya versiyasi (UserAuth qo'yadi).
func UserSessionVersion(r *http.Request) int {
	v, _ := r.Context().Value(CtxUserSessionVer).(int)
	return v
}

// IsReviewActor reports whether this request comes from the sandboxed Play
// review account. It defaults to false everywhere the flag was never set —
// unauthenticated routes, background jobs, tests — so forgetting to plumb it
// through can only ever fail towards "treat as a normal user", never towards
// silently granting a real user review-account behaviour.
func IsReviewActor(ctx context.Context) bool {
	v, _ := ctx.Value(CtxReviewActor).(bool)
	return v
}
func AdminID(r *http.Request) string {
	if v, ok := r.Context().Value(CtxAdminID).(string); ok {
		return v
	}
	return ""
}

// AdminRole returns the role stored in the admin JWT (superadmin|moderator|
// support). Empty when the request wasn't authenticated as an admin.
func AdminRole(r *http.Request) string {
	if v, ok := r.Context().Value(CtxAdminRole).(string); ok {
		return v
	}
	return ""
}

func AdminTokenVersion(r *http.Request) int {
	if v, ok := r.Context().Value(CtxAdminVer).(int); ok {
		return v
	}
	return 0
}

// WithAdminRole replaces the JWT snapshot with the current database role.
// The admin session middleware calls this before endpoint RBAC runs, so role
// changes take effect immediately instead of waiting for token expiry.
func WithAdminRole(ctx context.Context, role string) context.Context {
	return context.WithValue(ctx, CtxAdminRole, role)
}

// RequireRole authorizes an admin request by role. "superadmin" is ALWAYS
// allowed (full access), regardless of the passed list. Any other role passes
// only if it is in `allowed`. Otherwise a 403 is returned.
//
// MUST be mounted AFTER AdminAuth so the role is present in the context. This is
// the RBAC gap the admin panel had: previously every authenticated admin —
// including a plain "moderator" — could hit every endpoint (delete users, send
// broadcasts, manage other admins). Now each route declares who may call it.
func RequireRole(allowed ...string) func(http.Handler) http.Handler {
	set := map[string]bool{"superadmin": true}
	for _, a := range allowed {
		set[a] = true
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if !set[AdminRole(r)] {
				Err(w, NewError(http.StatusForbidden, "forbidden", "insufficient role"))
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// PanicHook, agar o'rnatilgan bo'lsa, Recover ushlagan har bir panic uchun
// chaqiriladi (main.go uni internal/errlog ga ulaydi).
//
// # NEGA ILGAK, TO'G'RIDAN-TO'G'RI CHAQIRUV EMAS
//
// httpx — quyi qatlam: uni internal/errlog import qiladi, teskarisi emas.
// Ilgak shu yo'nalishni saqlaydi va middleware tartibini o'zgartirmaydi:
// Recover javobni qaytarish mas'uliyatini o'zida ushlab qoladi, xatolikni
// yozish esa ixtiyoriy qo'shimcha bo'lib qoladi. Ilgak nil bo'lsa yoki
// o'zi panic qilsa ham, mijoz baribir 500 oladi.
var PanicHook func(r *http.Request, rec any, stack []byte)

func Recover(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if rec := recover(); rec != nil {
				if h := PanicHook; h != nil {
					// Ilgakdagi nosozlik javobni to'sib qo'ymasin.
					func() {
						defer func() { _ = recover() }()
						h(r, rec, debug.Stack())
					}()
				}
				JSON(w, http.StatusInternalServerError, errBody{Error: APIError{Code: "panic", Message: "internal server error"}})
			}
		}()
		next.ServeHTTP(w, r)
	})
}

// SecurityHeaders sets conservative response headers. The API serves JSON only,
// so a strict CSP plus nosniff/frame-deny costs nothing and blocks a range of
// content-type confusion and clickjacking issues.
func SecurityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("X-Frame-Options", "DENY")
		h.Set("Referrer-Policy", "no-referrer")
		h.Set("Content-Security-Policy", "default-src 'none'; frame-ancestors 'none'")
		h.Set("Permissions-Policy", "camera=(), microphone=(), geolocation=(), payment=(), usb=()")
		h.Add("Vary", "Origin")
		h.Add("Vary", "Authorization")
		if r.Header.Get("Authorization") != "" || strings.HasPrefix(r.URL.Path, "/api/auth/") ||
			strings.HasPrefix(r.URL.Path, "/api/admin/") || r.URL.Path == "/api/me" {
			h.Set("Cache-Control", "no-store")
			h.Set("Pragma", "no-cache")
		}
		next.ServeHTTP(w, r)
	})
}

// RateLimit: simple per-IP token bucket (in-memory).
type bucket struct {
	tokens   float64
	last     time.Time
	capacity float64
	refill   float64 // tokens / sec
}

type Limiter struct {
	mu      sync.Mutex
	buckets map[string]*bucket
	cap     float64
	refill  float64
}

func NewLimiter(cap float64, refillPerSec float64) *Limiter {
	return &Limiter{buckets: map[string]*bucket{}, cap: cap, refill: refillPerSec}
}

func (l *Limiter) allow(key string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	b, ok := l.buckets[key]
	now := time.Now()
	if !ok {
		l.buckets[key] = &bucket{tokens: l.cap - 1, last: now, capacity: l.cap, refill: l.refill}
		return true
	}
	elapsed := now.Sub(b.last).Seconds()
	b.tokens += elapsed * b.refill
	if b.tokens > b.capacity {
		b.tokens = b.capacity
	}
	b.last = now
	if b.tokens < 1 {
		return false
	}
	b.tokens--
	return true
}

func (l *Limiter) Middleware(prefix string) func(http.Handler) http.Handler {
	return l.MiddlewareKey(prefix, clientIP)
}

// MiddlewareKey rate-limits by a caller-selected stable identity. Authenticated
// expensive operations (uploads) use user id rather than spoofable/request-
// distribution-prone IP addresses. An empty key falls back to client IP.
func (l *Limiter) MiddlewareKey(prefix string, keyFn func(*http.Request) string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			identity := keyFn(r)
			if identity == "" {
				identity = clientIP(r)
			}
			key := prefix + ":" + identity
			if !l.allow(key) {
				Err(w, NewError(http.StatusTooManyRequests, "rate_limited", "too many requests"))
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// StartCleanup periodically evicts idle buckets so the per-IP map can't grow
// without bound — otherwise every unique client IP leaves a permanent entry (a
// slow memory leak). A bucket idle longer than `idle` is safe to drop: by then
// it has fully refilled to capacity, so a returning client is not handed any
// extra allowance versus keeping the stale bucket. Pick `idle` comfortably
// above the bucket's full-refill time (capacity / refillPerSec seconds).
// Runs in its own goroutine and stops when ctx is cancelled.
func (l *Limiter) StartCleanup(ctx context.Context, every, idle time.Duration) {
	go func() {
		t := time.NewTicker(every)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case now := <-t.C:
				l.evictIdle(now, idle)
			}
		}
	}()
}

// evictIdle removes every bucket untouched for longer than `idle` as of `now`.
// It is the deterministic core of StartCleanup, split out so the eviction rule
// can be unit-tested without spinning up a ticker and goroutine.
func (l *Limiter) evictIdle(now time.Time, idle time.Duration) {
	l.mu.Lock()
	defer l.mu.Unlock()
	for k, b := range l.buckets {
		if now.Sub(b.last) > idle {
			delete(l.buckets, k)
		}
	}
}

// ClientIP exposes the same trusted-proxy-aware client address the rate limiter
// buckets on, for callers that keep their own per-IP budgets.
func ClientIP(r *http.Request) string { return clientIP(r) }

func clientIP(r *http.Request) string {
	// Only trust the forwarded header when explicitly configured to run behind a
	// trusted proxy. Otherwise an attacker can rotate XFF to mint a fresh
	// rate-limit bucket per request and bypass brute-force protection.
	if TrustProxyHeaders {
		if ip := forwardedClientIP(r.Header.Get("X-Forwarded-For"), TrustedProxyHops); ip != "" {
			return ip
		}
	}
	host := r.RemoteAddr
	if i := strings.LastIndexByte(host, ':'); i > 0 {
		return host[:i]
	}
	return host
}

// forwardedClientIP picks the client address out of an X-Forwarded-For chain,
// counting from the RIGHT.
//
// This direction is the whole point. Reverse proxies APPEND the peer address to
// whatever the client already sent — Caddy's reverse_proxy and nginx's
// $proxy_add_x_forwarded_for both do — so in
//
//	X-Forwarded-For: 9.9.9.9, <real client>
//
// the leftmost element is simply a string the attacker typed. Reading it (as
// this function used to) hands the rate-limit key to the caller: a new value
// per request means a new bucket per request, and every brute-force budget in
// this file — admin login, OTP verify, the Play review-code guess counter —
// becomes unlimited. Only the last `hops` elements were written by
// infrastructure we control, so the client is the one just before them.
//
// A chain shorter than the configured hop count means the header did not come
// from the expected topology; "" is returned so the caller falls back to
// RemoteAddr rather than trusting an attacker-supplied element.
func forwardedClientIP(header string, hops int) string {
	if header == "" {
		return ""
	}
	if hops < 1 {
		hops = 1
	}
	parts := strings.Split(header, ",")
	i := len(parts) - hops
	if i < 0 {
		return ""
	}
	return strings.TrimSpace(parts[i])
}
