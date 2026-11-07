// Package config reads the runtime configuration from environment variables
// (12-factor style, easy to feed from a Kubernetes ConfigMap / Secret).
package config

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	Addr      string // LISTEN_ADDR, default ":8080"
	DataDir   string // DATA_DIR: CSV files loaded at startup, default "./data"
	DBPath    string // DB_PATH: SQLite file; empty = in-memory (default)
	MaxUpload int64  // MAX_UPLOAD_MB, default 10

	Auth Auth
}

type Auth struct {
	Enabled      bool   // AUTH_ENABLED, default false (local dev)
	ClientID     string // GOOGLE_CLIENT_ID
	ClientSecret string // GOOGLE_CLIENT_SECRET
	BaseURL      string // BASE_URL, public URL, e.g. https://loto.example.com
	CookieSecure bool   // COOKIE_SECURE, default: true when BASE_URL is https
}

// ShutdownGrace is the time given to in-flight requests on SIGTERM.
const ShutdownGrace = 10 * time.Second

func Load() (Config, error) {
	c := Config{
		Addr:    env("LISTEN_ADDR", ":8080"),
		DataDir: env("DATA_DIR", "./data"),
		DBPath:  os.Getenv("DB_PATH"),
	}
	mb, err := strconv.Atoi(env("MAX_UPLOAD_MB", "10"))
	if err != nil || mb <= 0 {
		return c, fmt.Errorf("MAX_UPLOAD_MB invalide : %q", os.Getenv("MAX_UPLOAD_MB"))
	}
	c.MaxUpload = int64(mb) << 20

	c.Auth.Enabled, err = boolEnv("AUTH_ENABLED", false)
	if err != nil {
		return c, err
	}
	c.Auth.ClientID = os.Getenv("GOOGLE_CLIENT_ID")
	c.Auth.ClientSecret = os.Getenv("GOOGLE_CLIENT_SECRET")
	c.Auth.BaseURL = strings.TrimRight(os.Getenv("BASE_URL"), "/")
	c.Auth.CookieSecure, err = boolEnv("COOKIE_SECURE", strings.HasPrefix(c.Auth.BaseURL, "https://"))
	if err != nil {
		return c, err
	}

	if c.Auth.Enabled {
		var missing []string
		for k, v := range map[string]string{
			"GOOGLE_CLIENT_ID": c.Auth.ClientID, "GOOGLE_CLIENT_SECRET": c.Auth.ClientSecret, "BASE_URL": c.Auth.BaseURL,
		} {
			if v == "" {
				missing = append(missing, k)
			}
		}
		if len(missing) > 0 {
			return c, errors.New("AUTH_ENABLED=true mais variables manquantes : " + strings.Join(missing, ", "))
		}
	}
	return c, nil
}

func env(k, def string) string {
	if v, ok := os.LookupEnv(k); ok && v != "" {
		return v
	}
	return def
}

func boolEnv(k string, def bool) (bool, error) {
	v := os.Getenv(k)
	if v == "" {
		return def, nil
	}
	b, err := strconv.ParseBool(v)
	if err != nil {
		return false, fmt.Errorf("%s invalide : %q", k, v)
	}
	return b, nil
}
