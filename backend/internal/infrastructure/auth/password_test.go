package auth

import "testing"

func TestBcryptHasher_HashAndVerify(t *testing.T) {
	h := NewBcryptHasher()

	hash, err := h.Hash("correct-horse-battery-staple")
	if err != nil {
		t.Fatalf("Hash() error = %v", err)
	}
	if hash == "correct-horse-battery-staple" {
		t.Fatal("Hash() returned the plaintext password unchanged")
	}
	if !h.Verify("correct-horse-battery-staple", hash) {
		t.Error("Verify() with the correct password = false, want true")
	}
	if h.Verify("wrong-password", hash) {
		t.Error("Verify() with a wrong password = true, want false")
	}
}

func TestBcryptHasher_DifferentHashesForSamePassword(t *testing.T) {
	h := NewBcryptHasher()
	a, err := h.Hash("same-password")
	if err != nil {
		t.Fatalf("Hash() error = %v", err)
	}
	b, err := h.Hash("same-password")
	if err != nil {
		t.Fatalf("Hash() error = %v", err)
	}
	if a == b {
		t.Error("two hashes of the same password are identical — salt is not being applied")
	}
}
