package auth

import (
	"errors"
	"testing"
	"time"
)

const testSecret = "test-secret-at-least-32-bytes-long-ok" // gitleaks:allow — test fixture, not a real secret

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
	noneToken := "eyJhbGciOiJub25lIiwidHlwIjoiSldUIn0.eyJ1c2VyX2lkIjoieCJ9." // gitleaks:allow — not a real secret, a negative-test fixture
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

	// Corrupt one byte well inside the signature segment. Must not use a
	// fixed replacement character: since the token (and so the byte at this
	// offset) differs on every run, a fixed 'X' has a ~1/64 chance of
	// already being the original character, making the "tamper" a no-op
	// and the test flaky (observed failing in CI: "error = <nil>, want
	// ErrInvalidToken"). Picking a replacement guaranteed to differ from
	// the original byte removes that flake entirely.
	idx := len(token) - 20
	replacement := byte('X')
	if token[idx] == replacement {
		replacement = 'Y'
	}
	tampered := token[:idx] + string(replacement) + token[idx+1:]
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
