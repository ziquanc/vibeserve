package auth

import (
	"testing"
	"time"
)

func TestHashAndVerifyPassword(t *testing.T) {
	hash, err := HashPassword("mypassword123")
	if err != nil {
		t.Fatal(err)
	}
	if !VerifyPassword("mypassword123", hash) {
		t.Error("should verify correct password")
	}
	if VerifyPassword("wrongpassword", hash) {
		t.Error("should reject wrong password")
	}
}

func TestGenerateAndVerifyAccessToken(t *testing.T) {
	secret := "test-secret-key"
	token, err := GenerateAccessToken(secret, 42, "admin", "test@example.com", 15*time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if token == "" {
		t.Fatal("token should not be empty")
	}

	claims, err := VerifyAccessToken(secret, token)
	if err != nil {
		t.Fatal(err)
	}
	if claims.UserID != 42 {
		t.Errorf("expected user_id 42, got %d", claims.UserID)
	}
	if claims.Role != "admin" {
		t.Errorf("expected role admin, got %s", claims.Role)
	}
	if claims.Email != "test@example.com" {
		t.Errorf("expected email test@example.com, got %s", claims.Email)
	}
}

func TestVerifyExpiredToken(t *testing.T) {
	secret := "test-secret"
	token, _ := GenerateAccessToken(secret, 1, "user", "a@b.com", -1*time.Hour)
	_, err := VerifyAccessToken(secret, token)
	if err == nil {
		t.Error("should reject expired token")
	}
}

func TestVerifyWrongSecret(t *testing.T) {
	token, _ := GenerateAccessToken("secret1", 1, "user", "a@b.com", time.Hour)
	_, err := VerifyAccessToken("secret2", token)
	if err == nil {
		t.Error("should reject token with wrong secret")
	}
}

func TestGenerateRefreshToken(t *testing.T) {
	t1 := GenerateRefreshToken()
	t2 := GenerateRefreshToken()
	if t1 == "" || t2 == "" {
		t.Error("refresh tokens should not be empty")
	}
	if t1 == t2 {
		t.Error("refresh tokens should be unique")
	}
}

func TestGenerateRandomSecret(t *testing.T) {
	s := GenerateRandomSecret()
	if len(s) < 32 {
		t.Error("secret should be at least 32 chars")
	}
}
