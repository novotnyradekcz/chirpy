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
	"github.com/novotnyradekcz/chirpy/internal/database"
)

func TestHandlerChirpsCreate(t *testing.T) {
	tests := []struct {
		name string
		body string
		want string
	}{
		{
			name: "example",
			body: "This is a kerfuffle opinion I need to share with the world",
			want: "This is a **** opinion I need to share with the world",
		},
		{
			name: "case insensitive matches",
			body: "KERFUFFLE Sharbert fOrNaX kerfuffle",
			want: "**** **** **** ****",
		},
		{
			name: "punctuation and partial matches unchanged",
			body: "Sharbert! kerfuffle, (fornax) sharberts",
			want: "Sharbert! kerfuffle, (fornax) sharberts",
		},
		{
			name: "no matches",
			body: "A perfectly ordinary chirp",
			want: "A perfectly ordinary chirp",
		},
		{
			name: "preserve spaces",
			body: "  kerfuffle  sharbert  ",
			want: "  ****  ****  ",
		},
		{
			name: "only spaces delimit words",
			body: "kerfuffle\tsharbert fornax\nkerfuffle",
			want: "kerfuffle\tsharbert fornax\nkerfuffle",
		},
		{
			name: "empty body",
		},
		{
			name: "length limit",
			body: strings.Repeat("a", 140),
			want: strings.Repeat("a", 140),
		},
		{
			name: "multibyte body at byte limit",
			body: strings.Repeat("é", 70),
			want: strings.Repeat("é", 70),
		},
	}

	chirpID := uuid.MustParse("924b38f4-ec39-43d1-884d-7900813888ee")
	userID := uuid.MustParse("d6b2797e-b0ee-45e6-985f-1c9a57aba969")
	createdAt := time.Date(2026, time.January, 2, 3, 4, 5, 123456000, time.UTC)
	updatedAt := createdAt.Add(time.Minute)

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, "/api/chirps", strings.NewReader(chirpTestPayload(t, tt.body, userID)))
			type contextKey struct{}
			req = req.WithContext(context.WithValue(req.Context(), contextKey{}, "request context"))
			calls := 0
			cfg := apiConfig{dbQueries: newChirpTestQueries(t, func(ctx context.Context, query string, args []driver.NamedValue) (driver.Rows, error) {
				calls++
				if ctx != req.Context() {
					t.Error("database query did not receive the request context")
				}
				assertChirpTestInsert(t, query, args, tt.want, userID)
				return &chirpTestRows{values: []driver.Value{
					chirpID.String(), createdAt, updatedAt, tt.want, userID.String(),
				}}, nil
			})}
			rec := httptest.NewRecorder()

			cfg.handlerChirps(rec, req)

			if calls != 1 {
				t.Errorf("database query calls = %d, want 1", calls)
			}
			assertChirpTestJSON(t, rec, http.StatusCreated, map[string]any{
				"id":         chirpID.String(),
				"created_at": createdAt.Format(time.RFC3339Nano),
				"updated_at": updatedAt.Format(time.RFC3339Nano),
				"body":       tt.want,
				"user_id":    userID.String(),
			})
		})
	}
}

func TestHandlerGetChirps(t *testing.T) {
	chirps := []database.Chirp{
		{
			ID:        uuid.MustParse("924b38f4-ec39-43d1-884d-7900813888ee"),
			CreatedAt: time.Date(2026, time.January, 2, 3, 4, 5, 123456000, time.UTC),
			UpdatedAt: time.Date(2026, time.January, 2, 3, 5, 5, 123456000, time.UTC),
			Body:      "first chirp",
			UserID:    uuid.MustParse("d6b2797e-b0ee-45e6-985f-1c9a57aba969"),
		},
		{
			ID:        uuid.MustParse("e4b1e4df-03f1-4739-a408-ebd135122d23"),
			CreatedAt: time.Date(2026, time.January, 3, 3, 4, 5, 123456000, time.UTC),
			UpdatedAt: time.Date(2026, time.January, 3, 3, 5, 5, 123456000, time.UTC),
			Body:      "second chirp",
			UserID:    uuid.MustParse("d6b2797e-b0ee-45e6-985f-1c9a57aba969"),
		},
	}

	tests := []struct {
		name string
		rows [][]driver.Value
		want []Chirp
	}{
		{
			name: "returns chirps in query order",
			rows: [][]driver.Value{
				{chirps[0].ID.String(), chirps[0].CreatedAt, chirps[0].UpdatedAt, chirps[0].Body, chirps[0].UserID.String()},
				{chirps[1].ID.String(), chirps[1].CreatedAt, chirps[1].UpdatedAt, chirps[1].Body, chirps[1].UserID.String()},
			},
			want: []Chirp{
				{ID: chirps[0].ID, CreatedAt: chirps[0].CreatedAt, UpdatedAt: chirps[0].UpdatedAt, Body: chirps[0].Body, UserID: chirps[0].UserID},
				{ID: chirps[1].ID, CreatedAt: chirps[1].CreatedAt, UpdatedAt: chirps[1].UpdatedAt, Body: chirps[1].Body, UserID: chirps[1].UserID},
			},
		},
		{
			name: "empty result is an empty array",
			rows: [][]driver.Value{},
			want: []Chirp{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/api/chirps", nil)
			type contextKey struct{}
			req = req.WithContext(context.WithValue(req.Context(), contextKey{}, "request context"))
			calls := 0
			cfg := apiConfig{dbQueries: newChirpTestQueries(t, func(ctx context.Context, query string, args []driver.NamedValue) (driver.Rows, error) {
				calls++
				if ctx != req.Context() {
					t.Error("database query did not receive the request context")
				}
				if normalized := strings.Join(strings.Fields(strings.ToLower(query)), " "); !strings.Contains(normalized, "order by created_at asc") {
					t.Errorf("query does not order chirps by created_at ascending: %q", query)
				}
				if len(args) != 0 {
					t.Errorf("query arguments = %v, want none", args)
				}
				return &chirpTestRows{rows: tt.rows}, nil
			})}
			rec := httptest.NewRecorder()

			cfg.handlerGetChirps(rec, req)

			if calls != 1 {
				t.Errorf("database query calls = %d, want 1", calls)
			}
			if rec.Code != http.StatusOK {
				t.Fatalf("status = %d, want %d; response = %s", rec.Code, http.StatusOK, rec.Body.String())
			}
			if got := rec.Header().Get("Content-Type"); got != "application/json" {
				t.Errorf("Content-Type = %q, want application/json", got)
			}
			var got []Chirp
			if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
				t.Fatalf("decode response: %v; response = %s", err, rec.Body.String())
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("response = %#v, want %#v", got, tt.want)
			}
		})
	}
}

func TestHandlerGetChirp(t *testing.T) {
	chirpID := uuid.MustParse("924b38f4-ec39-43d1-884d-7900813888ee")
	userID := uuid.MustParse("d6b2797e-b0ee-45e6-985f-1c9a57aba969")
	createdAt := time.Date(2026, time.January, 2, 3, 4, 5, 123456000, time.UTC)
	updatedAt := createdAt.Add(time.Minute)

	tests := []struct {
		name       string
		values     []driver.Value
		wantStatus int
		wantBody   map[string]any
	}{
		{
			name:       "found",
			values:     []driver.Value{chirpID.String(), createdAt, updatedAt, "a chirp", userID.String()},
			wantStatus: http.StatusOK,
			wantBody: map[string]any{
				"id":         chirpID.String(),
				"created_at": createdAt.Format(time.RFC3339Nano),
				"updated_at": updatedAt.Format(time.RFC3339Nano),
				"body":       "a chirp",
				"user_id":    userID.String(),
			},
		},
		{
			name:       "not found",
			wantStatus: http.StatusNotFound,
			wantBody:   map[string]any{"error": "Chirp not found"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/api/chirps/"+chirpID.String(), nil)
			req.SetPathValue("chirpID", chirpID.String())
			type contextKey struct{}
			req = req.WithContext(context.WithValue(req.Context(), contextKey{}, "request context"))
			calls := 0
			cfg := apiConfig{dbQueries: newChirpTestQueries(t, func(ctx context.Context, query string, args []driver.NamedValue) (driver.Rows, error) {
				calls++
				if ctx != req.Context() {
					t.Error("database query did not receive the request context")
				}
				assertChirpTestSelectByID(t, query, args, chirpID)
				return &chirpTestRows{values: tt.values}, nil
			})}
			rec := httptest.NewRecorder()

			cfg.handlerGetChirp(rec, req)

			if calls != 1 {
				t.Errorf("database query calls = %d, want 1", calls)
			}
			assertChirpTestJSON(t, rec, tt.wantStatus, tt.wantBody)
		})
	}
}

func TestHandlerGetChirpInvalidID(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/api/chirps/not-a-uuid", nil)
	req.SetPathValue("chirpID", "not-a-uuid")
	rec := httptest.NewRecorder()

	(&apiConfig{}).handlerGetChirp(rec, req)

	assertChirpTestJSON(t, rec, http.StatusNotFound, map[string]any{"error": "Chirp not found"})
}

func TestHandlerGetChirpDatabaseError(t *testing.T) {
	chirpID := uuid.MustParse("924b38f4-ec39-43d1-884d-7900813888ee")
	calls := 0
	cfg := apiConfig{dbQueries: newChirpTestQueries(t, func(_ context.Context, query string, args []driver.NamedValue) (driver.Rows, error) {
		calls++
		assertChirpTestSelectByID(t, query, args, chirpID)
		return nil, errors.New("database unavailable: private connection details")
	})}
	req := httptest.NewRequest(http.MethodGet, "/api/chirps/"+chirpID.String(), nil)
	req.SetPathValue("chirpID", chirpID.String())
	rec := httptest.NewRecorder()

	cfg.handlerGetChirp(rec, req)

	if calls != 1 {
		t.Errorf("database query calls = %d, want 1", calls)
	}
	assertChirpTestJSON(t, rec, http.StatusInternalServerError, map[string]any{"error": "Could not retrieve chirp"})
}

func TestHandlerChirpsRejectsInvalidInputWithoutDatabase(t *testing.T) {
	userID := uuid.MustParse("d6b2797e-b0ee-45e6-985f-1c9a57aba969")
	tests := []struct {
		name    string
		payload string
		want    string
	}{
		{
			name: "empty request",
			want: "Invalid request body",
		},
		{
			name:    "malformed JSON",
			payload: `{"body":`,
			want:    "Invalid request body",
		},
		{
			name:    "invalid body type",
			payload: `{"body":42}`,
			want:    "Invalid request body",
		},
		{
			name:    "invalid UUID",
			payload: `{"body":"hello","user_id":"not-a-uuid"}`,
			want:    "Invalid request body",
		},
		{
			name:    "invalid UUID type",
			payload: `{"body":"hello","user_id":123}`,
			want:    "Invalid request body",
		},
		{
			name:    "body one byte over limit",
			payload: chirpTestPayload(t, strings.Repeat("a", 141), userID),
			want:    "Chirp is too long",
		},
		{
			name:    "limit counts bytes not runes",
			payload: chirpTestPayload(t, strings.Repeat("é", 71), userID),
			want:    "Chirp is too long",
		},
		{
			name:    "rejects long body before cleaning",
			payload: chirpTestPayload(t, strings.Repeat("kerfuffle ", 15), userID),
			want:    "Chirp is too long",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, "/api/chirps", strings.NewReader(tt.payload))
			rec := httptest.NewRecorder()
			cfg := apiConfig{}

			cfg.handlerChirps(rec, req)

			assertChirpTestJSON(t, rec, http.StatusBadRequest, map[string]any{"error": tt.want})
		})
	}
}

func TestHandlerChirpsDatabaseError(t *testing.T) {
	userID := uuid.MustParse("d6b2797e-b0ee-45e6-985f-1c9a57aba969")
	calls := 0
	cfg := apiConfig{dbQueries: newChirpTestQueries(t, func(_ context.Context, query string, args []driver.NamedValue) (driver.Rows, error) {
		calls++
		assertChirpTestInsert(t, query, args, "a **** chirp", userID)
		return nil, errors.New("database unavailable: private connection details")
	})}
	req := httptest.NewRequest(http.MethodPost, "/api/chirps", strings.NewReader(chirpTestPayload(t, "a kerfuffle chirp", userID)))
	rec := httptest.NewRecorder()

	cfg.handlerChirps(rec, req)

	if calls != 1 {
		t.Errorf("database query calls = %d, want 1", calls)
	}
	assertChirpTestJSON(t, rec, http.StatusInternalServerError, map[string]any{"error": "Could not create chirp"})
}

func chirpTestPayload(t *testing.T, body string, userID uuid.UUID) string {
	t.Helper()
	payload, err := json.Marshal(map[string]string{"body": body, "user_id": userID.String()})
	if err != nil {
		t.Fatal(err)
	}
	return string(payload)
}

func assertChirpTestJSON(t *testing.T, rec *httptest.ResponseRecorder, status int, want map[string]any) {
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

func assertChirpTestSelectByID(t *testing.T, query string, args []driver.NamedValue, chirpID uuid.UUID) {
	t.Helper()
	if normalized := strings.Join(strings.Fields(strings.ToLower(query)), " "); !strings.Contains(normalized, "from chirps where id = $1") {
		t.Errorf("expected a chirp lookup by ID, got query %q", query)
	}
	if len(args) != 1 {
		t.Fatalf("query arguments = %v, want chirp ID", args)
	}
	if args[0].Ordinal != 1 || args[0].Value != chirpID.String() {
		t.Errorf("query argument = %+v, want chirp ID %q", args[0], chirpID)
	}
}

func assertChirpTestInsert(t *testing.T, query string, args []driver.NamedValue, body string, userID uuid.UUID) {
	t.Helper()
	if normalized := strings.Join(strings.Fields(strings.ToLower(query)), " "); !strings.Contains(normalized, "insert into chirps") {
		t.Errorf("expected a chirps insert, got query %q", query)
	}
	if len(args) != 2 {
		t.Fatalf("query arguments = %v, want body and user_id", args)
	}
	if args[0].Ordinal != 1 || args[0].Value != body {
		t.Errorf("first query argument = %+v, want body %q", args[0], body)
	}
	if args[1].Ordinal != 2 || args[1].Value != userID.String() {
		t.Errorf("second query argument = %+v, want user_id %q", args[1], userID.String())
	}
}

type chirpTestQuery func(context.Context, string, []driver.NamedValue) (driver.Rows, error)

func newChirpTestQueries(t *testing.T, query chirpTestQuery) *database.Queries {
	t.Helper()
	db := sql.OpenDB(&chirpTestConnector{query: query})
	t.Cleanup(func() {
		if err := db.Close(); err != nil {
			t.Errorf("close test database: %v", err)
		}
	})
	return database.New(db)
}

type chirpTestConnector struct {
	query chirpTestQuery
}

func (c *chirpTestConnector) Connect(context.Context) (driver.Conn, error) {
	return &chirpTestConn{query: c.query}, nil
}

func (c *chirpTestConnector) Driver() driver.Driver { return c }

func (*chirpTestConnector) Open(string) (driver.Conn, error) {
	return nil, errors.New("unexpected driver.Open call")
}

type chirpTestConn struct {
	query chirpTestQuery
}

func (*chirpTestConn) Prepare(string) (driver.Stmt, error) {
	return nil, errors.New("unexpected Prepare call")
}

func (*chirpTestConn) Close() error { return nil }

func (*chirpTestConn) Begin() (driver.Tx, error) {
	return nil, errors.New("unexpected Begin call")
}

func (c *chirpTestConn) QueryContext(ctx context.Context, query string, args []driver.NamedValue) (driver.Rows, error) {
	return c.query(ctx, query, args)
}

type chirpTestRows struct {
	values []driver.Value
	rows   [][]driver.Value
	index  int
}

func (*chirpTestRows) Columns() []string {
	return []string{"id", "created_at", "updated_at", "body", "user_id"}
}

func (*chirpTestRows) Close() error { return nil }

func (r *chirpTestRows) Next(dest []driver.Value) error {
	if r.rows != nil {
		if r.index >= len(r.rows) {
			return io.EOF
		}
		copy(dest, r.rows[r.index])
		r.index++
		return nil
	}
	if r.values == nil {
		return io.EOF
	}
	copy(dest, r.values)
	r.values = nil
	return nil
}
