// Command loto-stats serves the Loto / EuroMillions statistics web app.
//
// On startup the draw history is (re)built from the CSV files of DATA_DIR into
// an in-memory SQLite database: restarting the process simply reloads it.
package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/kmathelot/loto-stats-webapp/internal/auth"
	"github.com/kmathelot/loto-stats-webapp/internal/config"
	"github.com/kmathelot/loto-stats-webapp/internal/server"
	"github.com/kmathelot/loto-stats-webapp/internal/store"
)

func main() {
	log := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	if err := run(log); err != nil {
		log.Error("fatal", "err", err)
		os.Exit(1)
	}
}

func run(log *slog.Logger) error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	st, err := store.Open(cfg.DBPath)
	if err != nil {
		return err
	}
	defer st.Close()

	authn, err := auth.New(ctx, cfg.Auth, log)
	if err != nil {
		return err
	}
	if !authn.Enabled() {
		log.Warn("authentification désactivée (AUTH_ENABLED=false) : ne pas exposer publiquement")
	}

	srv, err := server.New(st, authn, log, cfg.MaxUpload)
	if err != nil {
		return err
	}

	res, err := st.ImportDir(ctx, cfg.DataDir)
	for file, r := range res {
		log.Info("bootstrap", "file", file, "game", r.Game.Code, "read", r.Read, "new", r.New, "skipped", r.Skipped)
	}
	if err != nil {
		return err
	}
	if len(res) == 0 {
		log.Warn("aucun CSV trouvé dans DATA_DIR", "dir", cfg.DataDir)
	}
	srv.SetReady()

	httpSrv := &http.Server{
		Addr:              cfg.Addr,
		Handler:           srv.Routes(),
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       60 * time.Second,
		WriteTimeout:      60 * time.Second,
		IdleTimeout:       120 * time.Second,
	}
	errc := make(chan error, 1)
	go func() {
		log.Info("listening", "addr", cfg.Addr, "auth", authn.Enabled())
		errc <- httpSrv.ListenAndServe()
	}()

	select {
	case err := <-errc:
		if !errors.Is(err, http.ErrServerClosed) {
			return err
		}
	case <-ctx.Done():
		log.Info("shutting down")
		sctx, cancel := context.WithTimeout(context.Background(), config.ShutdownGrace)
		defer cancel()
		return httpSrv.Shutdown(sctx)
	}
	return nil
}
