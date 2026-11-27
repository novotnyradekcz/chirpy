package auth

import "testing"

func TestHashPasswordAndCheckPasswordHash(t *testing.T) {
	const password = "correct horse battery staple"

	hash, err := HashPassword(password)
	if err != nil {
		t.Fatalf("HashPassword() error = %v", err)
	}
	if hash == password {
		t.Fatal("HashPassword() returned the plaintext password")
	}

	matches, err := CheckPasswordHash(password, hash)
	if err != nil {
		t.Fatalf("CheckPasswordHash() error = %v", err)
	}
	if !matches {
		t.Fatal("CheckPasswordHash() = false for the original password")
	}

	matches, err = CheckPasswordHash("incorrect password", hash)
	if err != nil {
		t.Fatalf("CheckPasswordHash() error for incorrect password = %v", err)
	}
	if matches {
		t.Fatal("CheckPasswordHash() = true for an incorrect password")
	}
}
