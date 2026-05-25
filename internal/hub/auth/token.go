package auth

import (
	"errors"
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// SessionTTL is how long an issued session token remains valid.
const SessionTTL = 30 * 24 * time.Hour

type Claims struct {
	UserID int64 `json:"uid"`
	jwt.RegisteredClaims
}

// IssueToken signs a session token for userID using HMAC-SHA256.
func IssueToken(userID int64, secret []byte) (string, error) {
	now := time.Now().UTC()
	claims := Claims{
		UserID: userID,
		RegisteredClaims: jwt.RegisteredClaims{
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(SessionTTL)),
			Issuer:    "lumen-hub",
		},
	}
	t := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return t.SignedString(secret)
}

// ParseToken verifies the token signature + expiry and returns the userID.
func ParseToken(raw string, secret []byte) (int64, error) {
	t, err := jwt.ParseWithClaims(raw, &Claims{}, func(token *jwt.Token) (any, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}
		return secret, nil
	})
	if err != nil {
		return 0, err
	}
	claims, ok := t.Claims.(*Claims)
	if !ok || !t.Valid {
		return 0, errors.New("invalid claims")
	}
	if claims.UserID == 0 {
		return 0, errors.New("missing user id")
	}
	return claims.UserID, nil
}
