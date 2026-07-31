package auth

import (
	"errors"
	"testing"
	"time"
)

const testSecret = "test-secret-at-least-32-bytes-long-ok"

func TestJWTManager_GenerateAndValidate(t *testing.T) {
	m := NewJWTManager(testSecret, time.Hour)

	token, err := m.Generate("u1", "user")
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}

	claims, err := m.Validate(token)
	if err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
	if claims.UserID != "u1" || claims.Role != "user" {
		t.Errorf("claims = %+v, want UserID=u1 Role=user", claims)
	}
}

func TestJWTManager_RejectsExpiredToken(t *testing.T) {
	m := NewJWTManager(testSecret, -time.Hour) // already expired
	token, err := m.Generate("u1", "user")
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}

	if _, err := m.Validate(token); !errors.Is(err, ErrInvalidToken) {
		t.Fatalf("Validate() error = %v, want ErrInvalidToken", err)
	}
}

func TestJWTManager_RejectsTokenSignedWithDifferentSecret(t *testing.T) {
	m1 := NewJWTManager(testSecret, time.Hour)
	m2 := NewJWTManager("a-completely-different-secret-value", time.Hour)

	token, err := m1.Generate("u1", "user")
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}
	if _, err := m2.Validate(token); !errors.Is(err, ErrInvalidToken) {
		t.Fatalf("Validate() with wrong secret error = %v, want ErrInvalidToken", err)
	}
}

func TestJWTManager_RejectsNoneAlgToken(t *testing.T) {
	m := NewJWTManager(testSecret, time.Hour)
	// Hand-crafted "none" token (header.payload.signature, alg=none, empty sig).
	// gitleaks:allow — not a real secret, a negative-test fixture.
	noneToken := "eyJhbGciOiJub25lIiwidHlwIjoiSldUIn0.eyJ1c2VyX2lkIjoieCJ9."
	if _, err := m.Validate(noneToken); !errors.Is(err, ErrInvalidToken) {
		t.Fatalf("Validate() on alg=none token error = %v, want ErrInvalidToken", err)
	}
}

func TestJWTManager_RejectsTamperedToken(t *testing.T) {
	m := NewJWTManager(testSecret, time.Hour)
	token, err := m.Generate("u1", "user")
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}

	// Corrupt the payload segment (index 1), not just the last character of
	// the signature — flipping a payload byte always changes the decoded
	// claims and must fail signature verification, unlike flipping a
	// signature-tail bit which can occasionally decode to an equivalent value.
	tampered := token[:len(token)-20] + "X" + token[len(token)-19:]
	if _, err := m.Validate(tampered); !errors.Is(err, ErrInvalidToken) {
		t.Fatalf("Validate() on tampered token error = %v, want ErrInvalidToken", err)
	}
}

func TestJWTManager_RejectsMalformedToken(t *testing.T) {
	m := NewJWTManager(testSecret, time.Hour)
	if _, err := m.Validate("not-a-jwt-at-all"); !errors.Is(err, ErrInvalidToken) {
		t.Fatalf("Validate() on malformed token error = %v, want ErrInvalidToken", err)
	}
}
