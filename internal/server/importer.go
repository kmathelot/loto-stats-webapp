package server

import (
	"errors"
	"html/template"
	"net/http"
	"net/url"
	"strconv"

	"github.com/kmathelot/loto-stats-webapp/internal/auth"
)

// importCSV handles the upload dialog. Imported draws live in the (in-memory)
// database only: they vanish on restart, by design. Durable history is
// whatever CSV files are present in DATA_DIR at startup.
func (s *Server) importCSV(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, s.maxUpload)
	msg := func(text string) {
		// 200 so that htmx swaps the message into the dialog.
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Write([]byte(`<p class="form-error" role="alert">` + template.HTMLEscapeString(text) + `</p>`))
	}
	file, hdr, err := r.FormFile("fichier")
	if err != nil {
		var tooBig *http.MaxBytesError
		if errors.As(err, &tooBig) {
			msg("Fichier trop volumineux.")
			return
		}
		msg("Choisis un fichier CSV.")
		return
	}
	defer file.Close()

	res, err := s.store.ImportCSV(r.Context(), file)
	if err != nil {
		msg("Import impossible : " + err.Error())
		return
	}
	s.Invalidate()
	u, _ := auth.UserFrom(r.Context())
	s.log.Info("import", "file", hdr.Filename, "game", res.Game.Code, "read", res.Read,
		"new", res.New, "skipped", res.Skipped, "user", u.Email)

	back := auth.SafeNext(r.FormValue("retour"))
	q := url.Values{"jeu": {res.Game.Code}, "importe": {strconv.Itoa(res.New)}}
	w.Header().Set("HX-Redirect", back+"?"+q.Encode())
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) login(w http.ResponseWriter, r *http.Request) {
	if !s.auth.Enabled() {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}
	q := r.URL.Query()
	data := map[string]any{
		"Next":   auth.SafeNext(q.Get("next")),
		"Error":  q.Get("error"),
		"Logout": q.Get("logout") != "",
	}
	s.render(w, "login", "login", data)
}
