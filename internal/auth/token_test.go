package auth

import (
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestMakeJWTAndValidateJWT(t *testing.T) {
	userID := uuid.New()
	const secret = "test-secret"

	token, err := MakeJWT(userID, secret, time.Hour)
	if err != nil {
		t.Fatalf("MakeJWT() error = %v", err)
	}

	gotUserID, err := ValidateJWT(token, secret)
	if err != nil {
		t.Fatalf("ValidateJWT() error = %v", err)
	}
	if gotUserID != userID {
		t.Errorf("ValidateJWT() user ID = %v, want %v", gotUserID, userID)
	}
}

func TestValidateJWTRejectsExpiredToken(t *testing.T) {
	token, err := MakeJWT(uuid.New(), "test-secret", -time.Second)
	if err != nil {
		t.Fatalf("MakeJWT() error = %v", err)
	}

	if _, err := ValidateJWT(token, "test-secret"); err == nil {
		t.Fatal("ValidateJWT() error = nil for an expired token")
	}
}

func TestValidateJWTRejectsWrongSecret(t *testing.T) {
	token, err := MakeJWT(uuid.New(), "correct-secret", time.Hour)
	if err != nil {
		t.Fatalf("MakeJWT() error = %v", err)
	}

	if _, err := ValidateJWT(token, "wrong-secret"); err == nil {
		t.Fatal("ValidateJWT() error = nil for a token signed with a different secret")
	}
}
