package auth

import (
	"fmt"

	"golang.org/x/crypto/bcrypt"
)

type PasswordHasher interface {
	Hash(password string) (string, error)
	Verify(password, hash string) bool
}

type BcryptHasher struct {
	cost int
}

func NewBcryptHasher() *BcryptHasher {
	return &BcryptHasher{cost: bcrypt.DefaultCost}
}

func (h *BcryptHasher) Hash(password string) (string, error) {
	bytes, err := bcrypt.GenerateFromPassword([]byte(password), h.cost)
	if err != nil {
		return "", fmt.Errorf("bcrypt hash: %w", err)
	}
	return string(bytes), nil
}

func (h *BcryptHasher) Verify(password, hash string) bool {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)) == nil
}
