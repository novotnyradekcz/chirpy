package main

import (
	"context"
	"database/sql/driver"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/novotnyradekcz/chirpy/internal/auth"
)

func TestHandlerLogin(t *testing.T) {
	const (
		email    = "user@example.com"
		password = "test password"
	)
	userID := uuid.MustParse("d6b2797e-b0ee-45e6-985f-1c9a57aba969")
	createdAt := time.Date(2026, time.January, 2, 3, 4, 5, 123456000, time.UTC)
	updatedAt := createdAt.Add(time.Minute)
	hashedPassword, err := auth.HashPassword(password)
	if err != nil {
		t.Fatalf("HashPassword() error = %v", err)
	}

	t.Run("valid credentials", func(t *testing.T) {
		calls := 0
		cfg := apiConfig{dbQueries: newUserTestQueries(t, func(_ context.Context, query string, args []driver.NamedValue) (driver.Rows, error) {
			calls++
			if !strings.Contains(strings.ToLower(query), "where email = $1") {
				t.Errorf("expected lookup by email, got query %q", query)
			}
			if len(args) != 1 || args[0].Value != email {
				t.Errorf("query arguments = %v, want email %q", args, email)
			}
			return &userTestRows{values: []driver.Value{userID.String(), createdAt, updatedAt, email, hashedPassword}}, nil
		})}
		req := httptest.NewRequest(http.MethodPost, "/api/login", strings.NewReader(`{"email":"user@example.com","password":"test password"}`))
		rec := httptest.NewRecorder()

		cfg.handlerLogin(rec, req)

		if calls != 1 {
			t.Errorf("database query calls = %d, want 1", calls)
		}
		assertUserTestJSON(t, rec, http.StatusOK, map[string]any{
			"id":         userID.String(),
			"created_at": createdAt.Format(time.RFC3339Nano),
			"updated_at": updatedAt.Format(time.RFC3339Nano),
			"email":      email,
		})
	})

	tests := []struct {
		name        string
		requestPass string
		storedHash  string
		queryErr    error
	}{
		{name: "incorrect password", requestPass: "wrong password", storedHash: hashedPassword},
		{name: "invalid stored hash", requestPass: password, storedHash: "unset"},
		{name: "user lookup error", requestPass: password, queryErr: errors.New("user not found")},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := apiConfig{dbQueries: newUserTestQueries(t, func(_ context.Context, _ string, _ []driver.NamedValue) (driver.Rows, error) {
				if tt.queryErr != nil {
					return nil, tt.queryErr
				}
				return &userTestRows{values: []driver.Value{userID.String(), createdAt, updatedAt, email, tt.storedHash}}, nil
			})}
			req := httptest.NewRequest(http.MethodPost, "/api/login", strings.NewReader(`{"email":"user@example.com","password":"`+tt.requestPass+`"}`))
			rec := httptest.NewRecorder()

			cfg.handlerLogin(rec, req)

			assertUserTestJSON(t, rec, http.StatusUnauthorized, map[string]any{"error": "Incorrect email or password"})
		})
	}
}

func TestHandlerLoginRejectsMissingCredentials(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/api/login", strings.NewReader(`{"email":"user@example.com"}`))
	rec := httptest.NewRecorder()

	(&apiConfig{}).handlerLogin(rec, req)

	assertUserTestJSON(t, rec, http.StatusBadRequest, map[string]any{"error": "Invalid request body"})
}
