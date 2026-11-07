package server

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/kmathelot/loto-stats-webapp/internal/store"
)

func newTestServer(t *testing.T) http.Handler {
	t.Helper()
	st, err := store.Open("")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	if _, err := st.ImportDir(context.Background(), "../../data"); err != nil {
		t.Fatal(err)
	}
	s, err := New(st, nil, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatal(err)
	}
	s.SetReady()
	return s.Routes()
}

func TestPages(t *testing.T) {
	h := newTestServer(t)
	for _, u := range []string{"/", "/stats", "/verifier?b=1&b=2&b=3&b=4&b=5&c=1", "/?jeu=euro", "/stats?jeu=euro",
		"/verifier?jeu=euro&b=1&b=2&b=3&b=4&b=5&c=1&c=2&q=7&page=3", "/readyz"} {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest("GET", u, nil))
		if rec.Code != 200 {
			t.Errorf("%s: %d %s", u, rec.Code, rec.Body.String())
		}
	}
}

func TestGeneratePartial(t *testing.T) {
	h := newTestServer(t)
	req := httptest.NewRequest("POST", "/generer", strings.NewReader("envoye=1&strategie=rand&nb=3"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if n := strings.Count(rec.Body.String(), "grid-card"); n != 3 {
		t.Fatalf("want 3 grids, got %d", n)
	}
}
