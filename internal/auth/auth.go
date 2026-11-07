// Package auth implements "Sign in with Google" (OpenID Connect).
//
// There is no server-side session: the Google ID token (a signed JWT) is kept
// in an HttpOnly cookie and verified on every request (signature, audience,
// issuer, expiry). The session therefore lasts exactly as long as the token
// (~1 h), after which the user goes through Google again (usually silently).
// Any Google account with a verified e-mail is accepted.
package auth

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/coreos/go-oidc/v3/oidc"
	"golang.org/x/oauth2"

	"github.com/kmathelot/loto-stats-webapp/internal/config"
)

const (
	googleIssuer   = "https://accounts.google.com"
	sessionCookie  = "session"
	stateCookie    = "oauth_state"
	nonceCookie    = "oauth_nonce"
	nextCookie     = "oauth_next"
	flowCookieLife = 10 * time.Minute

	LoginPath    = "/login"
	StartPath    = "/auth/google"
	CallbackPath = "/auth/callback"
	LogoutPath   = "/auth/logout"
)

// User is the authenticated user, available from the request context.
type User struct {
	Email   string
	Name    string
	Picture string
	Expires time.Time
	Dev     bool // auth disabled: synthetic user
}

type ctxKey struct{}

// UserFrom returns the user stored in the context by the middleware.
func UserFrom(ctx context.Context) (User, bool) {
	u, ok := ctx.Value(ctxKey{}).(User)
	return u, ok
}

// Authenticator verifies sessions and runs the OIDC flow.
// A nil *Authenticator (auth disabled) lets everything through.
type Authenticator struct {
	oauth    oauth2.Config
	verifier *oidc.IDTokenVerifier
	secure   bool
	log      *slog.Logger
}

// New performs OIDC discovery against Google. It returns (nil, nil) when auth is disabled.
func New(ctx context.Context, c config.Auth, log *slog.Logger) (*Authenticator, error) {
	if !c.Enabled {
		return nil, nil
	}
	provider, err := oidc.NewProvider(ctx, googleIssuer)
	if err != nil {
		return nil, err
	}
	return &Authenticator{
		oauth: oauth2.Config{
			ClientID:     c.ClientID,
			ClientSecret: c.ClientSecret,
			RedirectURL:  c.BaseURL + CallbackPath,
			Endpoint:     provider.Endpoint(),
			Scopes:       []string{oidc.ScopeOpenID, "email", "profile"},
		},
		verifier: provider.Verifier(&oidc.Config{ClientID: c.ClientID}),
		secure:   c.CookieSecure,
		log:      log,
	}, nil
}

// Enabled reports whether authentication is active.
func (a *Authenticator) Enabled() bool { return a != nil }

type claims struct {
	Email         string `json:"email"`
	EmailVerified bool   `json:"email_verified"`
	Name          string `json:"name"`
	Picture       string `json:"picture"`
	Nonce         string `json:"nonce"`
}

func (a *Authenticator) verify(ctx context.Context, raw string) (User, string, error) {
	tok, err := a.verifier.Verify(ctx, raw)
	if err != nil {
		return User{}, "", err
	}
	var c claims
	if err := tok.Claims(&c); err != nil {
		return User{}, "", err
	}
	if c.Email == "" || !c.EmailVerified {
		return User{}, "", errors.New("e-mail absent ou non vérifié")
	}
	return User{Email: c.Email, Name: c.Name, Picture: c.Picture, Expires: tok.Expiry}, c.Nonce, nil
}

// Middleware requires a valid session, except on public paths.
func (a *Authenticator) Middleware(public func(path string) bool) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if !a.Enabled() {
				ctx := context.WithValue(r.Context(), ctxKey{}, User{Email: "dev@localhost", Name: "Mode dev", Dev: true})
				next.ServeHTTP(w, r.WithContext(ctx))
				return
			}
			if public(r.URL.Path) {
				next.ServeHTTP(w, r)
				return
			}
			if c, err := r.Cookie(sessionCookie); err == nil {
				if u, _, err := a.verify(r.Context(), c.Value); err == nil {
					next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), ctxKey{}, u)))
					return
				}
				a.clear(w, sessionCookie)
			}
			login := LoginPath + "?next=" + url.QueryEscape(r.URL.RequestURI())
			if r.Header.Get("HX-Request") == "true" {
				// htmx: a 302 would be followed by XHR and swapped into the page.
				w.Header().Set("HX-Redirect", login)
				w.WriteHeader(http.StatusUnauthorized)
				return
			}
			http.Redirect(w, r, login, http.StatusSeeOther)
		})
	}
}

// Start redirects to Google's consent screen.
func (a *Authenticator) Start(w http.ResponseWriter, r *http.Request) {
	state, nonce := randomString(), randomString()
	a.setFlow(w, stateCookie, state)
	a.setFlow(w, nonceCookie, nonce)
	a.setFlow(w, nextCookie, SafeNext(r.URL.Query().Get("next")))
	http.Redirect(w, r, a.oauth.AuthCodeURL(state, oidc.Nonce(nonce)), http.StatusFound)
}

// Callback handles Google's redirect: checks state, exchanges the code,
// verifies the ID token and stores it as the session cookie.
func (a *Authenticator) Callback(w http.ResponseWriter, r *http.Request) {
	fail := func(msg string, err error) {
		a.log.Warn("oidc callback", "reason", msg, "err", err)
		http.Redirect(w, r, LoginPath+"?error="+url.QueryEscape(msg), http.StatusSeeOther)
	}
	if e := r.URL.Query().Get("error"); e != "" {
		fail("connexion refusée", errors.New(e))
		return
	}
	st, err := r.Cookie(stateCookie)
	if err != nil || st.Value == "" || st.Value != r.URL.Query().Get("state") {
		fail("session de connexion expirée, réessaie", err)
		return
	}
	tok, err := a.oauth.Exchange(r.Context(), r.URL.Query().Get("code"))
	if err != nil {
		fail("échange du code impossible", err)
		return
	}
	raw, ok := tok.Extra("id_token").(string)
	if !ok {
		fail("pas d'ID token", nil)
		return
	}
	u, nonce, err := a.verify(r.Context(), raw)
	if err != nil {
		fail("jeton invalide", err)
		return
	}
	if nc, err := r.Cookie(nonceCookie); err != nil || nc.Value != nonce {
		fail("nonce invalide", err)
		return
	}
	next := "/"
	if c, err := r.Cookie(nextCookie); err == nil {
		next = SafeNext(c.Value)
	}
	for _, n := range []string{stateCookie, nonceCookie, nextCookie} {
		a.clear(w, n)
	}
	http.SetCookie(w, &http.Cookie{
		Name: sessionCookie, Value: raw, Path: "/",
		Expires: u.Expires, MaxAge: int(time.Until(u.Expires).Seconds()),
		HttpOnly: true, Secure: a.secure, SameSite: http.SameSiteLaxMode,
	})
	a.log.Info("login", "email", u.Email, "expires", u.Expires)
	http.Redirect(w, r, next, http.StatusSeeOther)
}

// Logout drops the session cookie.
func (a *Authenticator) Logout(w http.ResponseWriter, r *http.Request) {
	a.clear(w, sessionCookie)
	http.Redirect(w, r, LoginPath+"?logout=1", http.StatusSeeOther)
}

func (a *Authenticator) setFlow(w http.ResponseWriter, name, value string) {
	http.SetCookie(w, &http.Cookie{
		Name: name, Value: value, Path: "/", MaxAge: int(flowCookieLife.Seconds()),
		HttpOnly: true, Secure: a.secure, SameSite: http.SameSiteLaxMode,
	})
}

func (a *Authenticator) clear(w http.ResponseWriter, name string) {
	http.SetCookie(w, &http.Cookie{Name: name, Path: "/", MaxAge: -1, HttpOnly: true, Secure: a.secure, SameSite: http.SameSiteLaxMode})
}

// SafeNext only allows local absolute paths (no open redirect).
func SafeNext(next string) string {
	if !strings.HasPrefix(next, "/") || strings.HasPrefix(next, "//") || strings.HasPrefix(next, "/\\") {
		return "/"
	}
	return next
}

func randomString() string {
	b := make([]byte, 24)
	_, _ = rand.Read(b)
	return base64.RawURLEncoding.EncodeToString(b)
}
