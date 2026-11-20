package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestHandlerChirpsValidate(t *testing.T) {
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
			name: "empty body",
		},
		{
			name: "length limit",
			body: strings.Repeat("a", 140),
			want: strings.Repeat("a", 140),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			payload, err := json.Marshal(map[string]string{"body": tt.body})
			if err != nil {
				t.Fatal(err)
			}
			req := httptest.NewRequest(http.MethodPost, "/api/validate_chirp", strings.NewReader(string(payload)))
			rec := httptest.NewRecorder()

			handlerChirpsValidate(rec, req)

			if rec.Code != http.StatusOK {
				t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
			}
			var got map[string]string
			if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
				t.Fatal(err)
			}
			if body, ok := got["cleaned_body"]; !ok || body != tt.want {
				t.Errorf("response = %v, want cleaned_body %q", got, tt.want)
			}
			if len(got) != 1 {
				t.Errorf("unexpected response fields: %v", got)
			}
		})
	}
}

func TestHandlerChirpsValidateRejectsLongBodyBeforeCleaning(t *testing.T) {
	payload, err := json.Marshal(map[string]string{"body": strings.Repeat("kerfuffle ", 15)})
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPost, "/api/validate_chirp", strings.NewReader(string(payload)))
	rec := httptest.NewRecorder()

	handlerChirpsValidate(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
	var got map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got["error"] != "Chirp is too long" {
		t.Errorf("unexpected error response: %v", got)
	}
}
