package server

import (
	"net/http"

	"github.com/kmathelot/loto-stats-webapp/internal/auth"
)

// login renders the "Se connecter avec Google" page.
func (s *Server) login(w http.ResponseWriter, r *http.Request) {
	if !s.auth.Enabled() {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}
	q := r.URL.Query()
	s.render(w, "login", "login", map[string]any{
		"Next":   auth.SafeNext(q.Get("next")),
		"Error":  q.Get("error"),
		"Logout": q.Get("logout") != "",
	})
}
