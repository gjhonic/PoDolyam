package httpapi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestHealth(t *testing.T) {
	handler := NewHandler()
	for _, tc := range []struct {
		name, method, path string
		status             int
	}{
		{"health", http.MethodGet, "/healthz", http.StatusOK},
		{"method", http.MethodPost, "/healthz", http.StatusMethodNotAllowed},
		{"unknown", http.MethodGet, "/api/unknown", http.StatusNotFound},
	} {
		t.Run(tc.name, func(t *testing.T) {
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, httptest.NewRequest(tc.method, tc.path, nil))
			if response.Code != tc.status {
				t.Fatalf("status = %d, want %d", response.Code, tc.status)
			}
			if tc.status != http.StatusOK {
				return
			}
			if response.Header().Get("Content-Type") != "application/json; charset=utf-8" {
				t.Fatal("ответ должен быть JSON")
			}
			var body struct {
				Status string `json:"status"`
			}
			if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
				t.Fatal(err)
			}
			if body.Status != "ok" {
				t.Fatalf("status = %q", body.Status)
			}
		})
	}
}
