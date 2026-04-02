package service

import (
	"errors"
	"testing"
	"time"

	"diplom/internal/domain"
	"diplom/internal/repository"

	"github.com/golang-jwt/jwt/v5"
)

func TestAuthServiceCreateAndParseToken(t *testing.T) {
	store := repository.NewMemoryStore()
	service := NewAuthService(store, "test-secret")

	token, err := service.createToken(domain.User{ID: 42, Role: domain.RoleAdmin})
	if err != nil {
		t.Fatalf("create token: %v", err)
	}

	claims, err := service.parseToken(token)
	if err != nil {
		t.Fatalf("parse token: %v", err)
	}
	if claims.UserID != 42 {
		t.Fatalf("expected user_id 42, got %d", claims.UserID)
	}
	if claims.Role != domain.RoleAdmin {
		t.Fatalf("expected role %s, got %s", domain.RoleAdmin, claims.Role)
	}
}

func TestAuthServiceRejectsInvalidSignature(t *testing.T) {
	store := repository.NewMemoryStore()
	service := NewAuthService(store, "test-secret")

	token, err := jwt.NewWithClaims(jwt.SigningMethodHS256, tokenClaims{
		UserID: 7,
		Role:   domain.RoleEmployee,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
		},
	}).SignedString([]byte("wrong-secret"))
	if err != nil {
		t.Fatalf("signed string: %v", err)
	}

	if _, err := service.parseToken(token); err == nil {
		t.Fatal("expected invalid signature error")
	}
}

func TestAuthServiceRejectsExpiredToken(t *testing.T) {
	store := repository.NewMemoryStore()
	service := NewAuthService(store, "test-secret")

	token, err := jwt.NewWithClaims(jwt.SigningMethodHS256, tokenClaims{
		UserID: 7,
		Role:   domain.RoleEmployee,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(-time.Hour)),
		},
	}).SignedString([]byte("test-secret"))
	if err != nil {
		t.Fatalf("signed string: %v", err)
	}

	_, err = service.parseToken(token)
	if !errors.Is(err, errors.New("token expired")) && (err == nil || err.Error() != "token expired") {
		t.Fatalf("expected token expired error, got %v", err)
	}
}
