// Package auth provides JWT issuance/validation and password hashing.
package auth

import (
	"errors"
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

var ErrInvalidToken = errors.New("invalid or expired token")

// Claims embeds the user id and role in the JWT payload, alongside the
// standard registered claims (expiry, issued-at).
type Claims struct {
	UserID string `json:"user_id"`
	Role   string `json:"role"`
	jwt.RegisteredClaims
}

// JWTManager issues and validates HS256 tokens with a single shared secret.
// HS256 (not RS256) is a deliberate choice: this is a single-service API,
// not a multi-party trust setup where a public/private keypair would let
// other services verify tokens without holding the signing secret.
type JWTManager struct {
	secret   []byte
	duration time.Duration
}

func NewJWTManager(secret string, duration time.Duration) *JWTManager {
	return &JWTManager{secret: []byte(secret), duration: duration}
}

func (m *JWTManager) Generate(userID, role string) (string, error) {
	claims := Claims{
		UserID: userID,
		Role:   role,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(m.duration)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
		},
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signed, err := token.SignedString(m.secret)
	if err != nil {
		return "", fmt.Errorf("sign token: %w", err)
	}
	return signed, nil
}

// Validate parses and verifies a token, rejecting anything not signed with
// this manager's secret via HMAC — including a crafted "alg: none" token,
// which jwt.ParseWithClaims already refuses because none is not an *HMAC
// method, but we double-check the concrete signing method defensively.
func (m *JWTManager) Validate(tokenStr string) (*Claims, error) {
	claims := &Claims{}
	token, err := jwt.ParseWithClaims(tokenStr, claims, func(t *jwt.Token) (interface{}, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", t.Method.Alg())
		}
		return m.secret, nil
	})
	if err != nil || !token.Valid {
		return nil, ErrInvalidToken
	}
	return claims, nil
}
