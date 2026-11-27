package main

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/novotnyradekcz/chirpy/internal/auth"
	"github.com/novotnyradekcz/chirpy/internal/database"
)

func TestHandlerUsersCreate(t *testing.T) {
	userID := uuid.MustParse("d6b2797e-b0ee-45e6-985f-1c9a57aba969")
	createdAt := time.Date(2026, time.January, 2, 3, 4, 5, 123456000, time.UTC)
	updatedAt := createdAt.Add(time.Minute)
	const (
		email    = "user@example.com"
		password = "test password"
	)

	req := httptest.NewRequest(http.MethodPost, "/api/users", strings.NewReader(`{"email":"user@example.com","password":"test password"}`))
	type contextKey struct{}
	req = req.WithContext(context.WithValue(req.Context(), contextKey{}, "request context"))
	calls := 0
	cfg := apiConfig{dbQueries: newUserTestQueries(t, func(ctx context.Context, query string, args []driver.NamedValue) (driver.Rows, error) {
		calls++
		if ctx != req.Context() {
			t.Error("database query did not receive the request context")
		}
		if normalized := strings.Join(strings.Fields(strings.ToLower(query)), " "); !strings.Contains(normalized, "insert into users") {
			t.Errorf("expected a users insert, got query %q", query)
		}
		if len(args) != 2 {
			t.Fatalf("query arguments = %v, want email and hashed password", args)
		}
		if args[0].Ordinal != 1 || args[0].Value != email {
			t.Errorf("query argument = %+v, want email %q", args[0], email)
		}
		hashedPassword, ok := args[1].Value.(string)
		if !ok || hashedPassword == password {
			t.Fatalf("password argument = %v, want a password hash", args[1].Value)
		}
		if matches, err := auth.CheckPasswordHash(password, hashedPassword); err != nil || !matches {
			t.Errorf("stored password hash check = (%v, %v), want (true, nil)", matches, err)
		}
		return &userTestRows{values: []driver.Value{userID.String(), createdAt, updatedAt, email, hashedPassword}}, nil
	})}
	rec := httptest.NewRecorder()

	cfg.handlerUsers(rec, req)

	if calls != 1 {
		t.Errorf("database query calls = %d, want 1", calls)
	}
	assertUserTestJSON(t, rec, http.StatusCreated, map[string]any{
		"id":         userID.String(),
		"created_at": createdAt.Format(time.RFC3339Nano),
		"updated_at": updatedAt.Format(time.RFC3339Nano),
		"email":      email,
	})
}

func TestHandlerUsersRejectsInvalidInputWithoutDatabase(t *testing.T) {
	tests := []struct {
		name    string
		payload string
	}{
		{name: "empty request"},
		{name: "missing password", payload: `{"email":"user@example.com"}`},
		{name: "malformed JSON", payload: `{"email":`},
		{name: "invalid email type", payload: `{"email":42}`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, "/api/users", strings.NewReader(tt.payload))
			rec := httptest.NewRecorder()
			cfg := apiConfig{}

			cfg.handlerUsers(rec, req)

			assertUserTestJSON(t, rec, http.StatusBadRequest, map[string]any{"error": "Invalid request body"})
		})
	}
}

func TestHandlerUsersDatabaseError(t *testing.T) {
	calls := 0
	cfg := apiConfig{dbQueries: newUserTestQueries(t, func(_ context.Context, _ string, _ []driver.NamedValue) (driver.Rows, error) {
		calls++
		return nil, errors.New("database unavailable: private connection details")
	})}
	req := httptest.NewRequest(http.MethodPost, "/api/users", strings.NewReader(`{"email":"user@example.com","password":"test password"}`))
	rec := httptest.NewRecorder()

	cfg.handlerUsers(rec, req)

	if calls != 1 {
		t.Errorf("database query calls = %d, want 1", calls)
	}
	assertUserTestJSON(t, rec, http.StatusInternalServerError, map[string]any{"error": "Could not create user"})
}

func assertUserTestJSON(t *testing.T, rec *httptest.ResponseRecorder, status int, want map[string]any) {
	t.Helper()
	if rec.Code != status {
		t.Fatalf("status = %d, want %d; response = %s", rec.Code, status, rec.Body.String())
	}
	if got := rec.Header().Get("Content-Type"); got != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", got)
	}
	var got map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode response: %v; response = %s", err, rec.Body.String())
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("response = %#v, want %#v", got, want)
	}
}

type userTestQuery func(context.Context, string, []driver.NamedValue) (driver.Rows, error)

func newUserTestQueries(t *testing.T, query userTestQuery) *database.Queries {
	t.Helper()
	db := sql.OpenDB(&userTestConnector{query: query})
	t.Cleanup(func() {
		if err := db.Close(); err != nil {
			t.Errorf("close test database: %v", err)
		}
	})
	return database.New(db)
}

type userTestConnector struct {
	query userTestQuery
}

func (c *userTestConnector) Connect(context.Context) (driver.Conn, error) {
	return &userTestConn{query: c.query}, nil
}

func (c *userTestConnector) Driver() driver.Driver { return c }

func (*userTestConnector) Open(string) (driver.Conn, error) {
	return nil, errors.New("unexpected driver.Open call")
}

type userTestConn struct {
	query userTestQuery
}

func (*userTestConn) Prepare(string) (driver.Stmt, error) {
	return nil, errors.New("unexpected Prepare call")
}

func (*userTestConn) Close() error { return nil }

func (*userTestConn) Begin() (driver.Tx, error) {
	return nil, errors.New("unexpected Begin call")
}

func (c *userTestConn) QueryContext(ctx context.Context, query string, args []driver.NamedValue) (driver.Rows, error) {
	return c.query(ctx, query, args)
}

type userTestRows struct {
	values []driver.Value
}

func (*userTestRows) Columns() []string {
	return []string{"id", "created_at", "updated_at", "email", "hashed_password"}
}

func (*userTestRows) Close() error { return nil }

func (r *userTestRows) Next(dest []driver.Value) error {
	if r.values == nil {
		return io.EOF
	}
	copy(dest, r.values)
	r.values = nil
	return nil
}
