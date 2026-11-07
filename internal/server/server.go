// Package server wires HTTP routes, templates and the htmx partials.
package server

import (
	"bytes"
	"context"
	"fmt"
	"html/template"
	"io/fs"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/kmathelot/loto-stats-webapp/internal/auth"
	"github.com/kmathelot/loto-stats-webapp/internal/lottery"
	"github.com/kmathelot/loto-stats-webapp/internal/store"
	"github.com/kmathelot/loto-stats-webapp/web"
)

const gameCookie = "jeu"

type Server struct {
	store     *store.Store
	auth      *auth.Authenticator
	log       *slog.Logger
	maxUpload int64

	pages map[string]*template.Template
	ready atomic.Bool

	mu    sync.Mutex
	cache map[string]*lottery.Stats
}

func New(st *store.Store, a *auth.Authenticator, log *slog.Logger, maxUpload int64) (*Server, error) {
	s := &Server{store: st, auth: a, log: log, maxUpload: maxUpload, cache: map[string]*lottery.Stats{}}
	if err := s.parseTemplates(); err != nil {
		return nil, err
	}
	return s, nil
}

// SetReady flips the readiness probe once the initial data load is done.
func (s *Server) SetReady() { s.ready.Store(true) }

func (s *Server) parseTemplates() error {
	s.pages = map[string]*template.Template{}
	for _, p := range []string{"generate", "stats", "verify", "login"} {
		t, err := template.New("").Funcs(funcs).ParseFS(web.Templates,
			"templates/layout.html", "templates/partials.html", "templates/"+p+".html")
		if err != nil {
			return fmt.Errorf("template %s: %w", p, err)
		}
		s.pages[p] = t
	}
	return nil
}

// Routes returns the full HTTP handler.
func (s *Server) Routes() http.Handler {
	mux := http.NewServeMux()

	static, _ := fs.Sub(web.Static, "static")
	mux.Handle("GET /static/", cacheStatic(http.StripPrefix("/static/", http.FileServerFS(static))))
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) { w.Write([]byte("ok")) })
	mux.HandleFunc("GET /readyz", s.readyz)

	mux.HandleFunc("GET "+auth.LoginPath, s.login)
	if s.auth.Enabled() {
		mux.HandleFunc("GET "+auth.StartPath, s.auth.Start)
		mux.HandleFunc("GET "+auth.CallbackPath, s.auth.Callback)
		mux.HandleFunc("GET "+auth.LogoutPath, s.auth.Logout)
	}

	mux.HandleFunc("GET /{$}", s.generatePage)
	mux.HandleFunc("POST /generer", s.generatePartial)
	mux.HandleFunc("GET /stats", s.statsPage)
	mux.HandleFunc("GET /verifier", s.verifyPage)
	mux.HandleFunc("POST /import", s.importCSV)

	public := func(p string) bool {
		return p == "/healthz" || p == "/readyz" || p == auth.LoginPath ||
			strings.HasPrefix(p, "/static/") || strings.HasPrefix(p, "/auth/")
	}
	var h http.Handler = mux
	h = s.auth.Middleware(public)(h)
	h = securityHeaders(h)
	h = s.logRequests(h)
	h = s.recoverer(h)
	return h
}

func (s *Server) readyz(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancel()
	if !s.ready.Load() || s.store.Ping(ctx) != nil {
		http.Error(w, "not ready", http.StatusServiceUnavailable)
		return
	}
	w.Write([]byte("ok"))
}

// stats returns cached statistics for a game, computing them on first use.
func (s *Server) stats(ctx context.Context, g lottery.Game) (*lottery.Stats, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if st, ok := s.cache[g.Code]; ok {
		return st, nil
	}
	draws, err := s.store.Draws(ctx, g)
	if err != nil {
		return nil, err
	}
	st := lottery.NewStats(g, draws)
	s.cache[g.Code] = st
	return st, nil
}

// Invalidate drops cached statistics (after an import).
func (s *Server) Invalidate() {
	s.mu.Lock()
	s.cache = map[string]*lottery.Stats{}
	s.mu.Unlock()
}

// currentGame reads ?jeu= (and remembers it in a cookie) or falls back to the cookie.
func (s *Server) currentGame(w http.ResponseWriter, r *http.Request) lottery.Game {
	if code := r.URL.Query().Get("jeu"); code != "" {
		if g, ok := lottery.GameByCode(code); ok {
			http.SetCookie(w, &http.Cookie{Name: gameCookie, Value: g.Code, Path: "/", MaxAge: 365 * 24 * 3600, SameSite: http.SameSiteLaxMode})
			return g
		}
	}
	if c, err := r.Cookie(gameCookie); err == nil {
		if g, ok := lottery.GameByCode(c.Value); ok {
			return g
		}
	}
	return lottery.Loto
}

// Page is the data shared by every full page (header, nav, flash).
type Page struct {
	Title       string
	Active      string // nav entry: generate | stats | verify
	Path        string
	Game        lottery.Game
	Games       []lottery.Game
	Stats       *lottery.Stats
	From, To    string
	User        auth.User
	AuthEnabled bool
	Flash       string
	Data        any
}

func (s *Server) page(w http.ResponseWriter, r *http.Request, title, active string) (Page, error) {
	g := s.currentGame(w, r)
	st, err := s.stats(r.Context(), g)
	if err != nil {
		return Page{}, err
	}
	p := Page{
		Title: title, Active: active, Path: r.URL.Path,
		Game: g, Games: lottery.Games, Stats: st,
		AuthEnabled: s.auth.Enabled(),
	}
	if st.N() > 0 {
		p.From, p.To = shortDate(st.Draws[0].Date), shortDate(st.Draws[st.N()-1].Date)
	}
	p.User, _ = auth.UserFrom(r.Context())
	if n := r.URL.Query().Get("importe"); n != "" {
		p.Flash = fmt.Sprintf("Import terminé : %s nouveau(x) tirage(s) %s.", n, g.Name)
	}
	return p, nil
}

// render executes a named template into a buffer first, so that a template
// error yields a clean 500 instead of a half-written page.
func (s *Server) render(w http.ResponseWriter, page, name string, data any) {
	var buf bytes.Buffer
	if err := s.pages[page].ExecuteTemplate(&buf, name, data); err != nil {
		s.log.Error("render", "page", page, "template", name, "err", err)
		http.Error(w, "erreur interne", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	buf.WriteTo(w)
}

func (s *Server) fail(w http.ResponseWriter, err error) {
	s.log.Error("request", "err", err)
	http.Error(w, "erreur interne", http.StatusInternalServerError)
}

func isHTMX(r *http.Request) bool { return r.Header.Get("HX-Request") == "true" }

// ---------------------------------------------------------------------------
// Middlewares
// ---------------------------------------------------------------------------

func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("X-Frame-Options", "DENY")
		h.Set("Referrer-Policy", "same-origin")
		next.ServeHTTP(w, r)
	})
}

func cacheStatic(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "public, max-age=3600")
		next.ServeHTTP(w, r)
	})
}

type statusWriter struct {
	http.ResponseWriter
	status int
}

func (w *statusWriter) WriteHeader(code int) {
	w.status = code
	w.ResponseWriter.WriteHeader(code)
}

func (s *Server) logRequests(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		sw := &statusWriter{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(sw, r)
		if r.URL.Path == "/healthz" || r.URL.Path == "/readyz" || strings.HasPrefix(r.URL.Path, "/static/") {
			return
		}
		s.log.Info("http", "method", r.Method, "path", r.URL.Path, "status", sw.status,
			"dur_ms", time.Since(start).Milliseconds(), "htmx", isHTMX(r))
	})
}

func (s *Server) recoverer(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if v := recover(); v != nil {
				s.log.Error("panic", "path", r.URL.Path, "value", v)
				http.Error(w, "erreur interne", http.StatusInternalServerError)
			}
		}()
		next.ServeHTTP(w, r)
	})
}
