package httpx

import (
	"time"

	"github.com/golang-jwt/jwt/v5"
)

func IssueUserToken(secret, userID string, ttl time.Duration) (string, error) {
	return IssueUserSessionToken(secret, userID, 0, ttl)
}

// IssueUserSessionToken — foydalanuvchi tokeni, joriy sessiya versiyasi bilan
// (qarang: Claims.SessionVersion).
func IssueUserSessionToken(secret, userID string, sessionVersion int, ttl time.Duration) (string, error) {
	c := Claims{
		UserID:         userID,
		SessionVersion: sessionVersion,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(ttl)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
		},
	}
	t := jwt.NewWithClaims(jwt.SigningMethodHS256, c)
	return t.SignedString([]byte(secret))
}

func IssueAdminToken(secret, adminID, role string, ttl time.Duration) (string, error) {
	return IssueVersionedAdminToken(secret, adminID, role, 0, ttl)
}

func IssueVersionedAdminToken(secret, adminID, role string, tokenVersion int, ttl time.Duration) (string, error) {
	c := AdminClaims{
		AdminID:      adminID,
		Role:         role,
		TokenVersion: &tokenVersion,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(ttl)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
		},
	}
	t := jwt.NewWithClaims(jwt.SigningMethodHS256, c)
	return t.SignedString([]byte(secret))
}

func ParseUserToken(secret, tok string) (string, error) {
	uid, _, err := ParseUserSessionToken(secret, tok)
	return uid, err
}

// ParseUserSessionToken — ParseUserToken, qo'shimcha ravishda sessiya versiyasi.
func ParseUserSessionToken(secret, tok string) (string, int, error) {
	c := &Claims{}
	_, err := jwt.ParseWithClaims(tok, c,
		func(*jwt.Token) (any, error) { return []byte(secret), nil },
		jwt.WithValidMethods(allowedJWTMethods))
	if err != nil {
		return "", 0, err
	}
	if c.UserID == "" {
		return "", 0, jwt.ErrTokenInvalidClaims
	}
	return c.UserID, c.SessionVersion, nil
}
