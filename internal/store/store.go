// Package store keeps the draw history in SQLite.
//
// The database is a disposable cache: it is rebuilt from the CSV files of the
// data directory on every start. Nothing in it needs a backup.
package store

import (
	"context"
	"database/sql"
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	_ "modernc.org/sqlite" // pure Go driver, no CGO

	"github.com/kmathelot/loto-stats-webapp/internal/lottery"
)

const schema = `
CREATE TABLE IF NOT EXISTS tirages (
    jeu   TEXT NOT NULL,          -- loto | euro
    date  TEXT NOT NULL,          -- ISO aaaa-mm-jj
    jour  TEXT NOT NULL,
    b1 INT, b2 INT, b3 INT, b4 INT, b5 INT,
    c1 INT, c2 INT,               -- chance / étoiles (c2 NULL au Loto)
    somme INT,
    PRIMARY KEY (jeu, date)
);`

// Store wraps the SQLite database.
type Store struct {
	db *sql.DB
}

// Open opens (or creates) the database. An empty path means in-memory.
func Open(path string) (*Store, error) {
	dsn := path
	if dsn == "" {
		dsn = ":memory:"
	}
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, err
	}
	// A single connection: required for ":memory:" (each connection would get
	// its own database) and plenty for this workload.
	db.SetMaxOpenConns(1)
	if _, err := db.Exec(schema); err != nil {
		db.Close()
		return nil, fmt.Errorf("schema: %w", err)
	}
	return &Store{db: db}, nil
}

func (s *Store) Close() error { return s.db.Close() }

// Ping checks the database is usable.
func (s *Store) Ping(ctx context.Context) error { return s.db.PingContext(ctx) }

// ImportResult summarizes one CSV import.
type ImportResult struct {
	Game    lottery.Game
	Read    int // valid rows read
	New     int // rows actually inserted (duplicates ignored)
	Skipped int // incomplete or malformed rows
}

// ImportCSV imports an FDJ CSV export. The game is detected from the header.
// Rows already present (same game and date) are ignored.
func (s *Store) ImportCSV(ctx context.Context, r io.Reader) (ImportResult, error) {
	cr := csv.NewReader(r)
	cr.Comma = ';'
	cr.FieldsPerRecord = -1
	cr.LazyQuotes = true
	cr.ReuseRecord = true

	header, err := cr.Read()
	if err != nil {
		return ImportResult{}, fmt.Errorf("lecture de l'en-tête : %w", err)
	}
	cols := make(map[string]int, len(header))
	names := make([]string, len(header))
	for i, h := range header {
		h = strings.TrimSpace(strings.TrimPrefix(h, "\ufeff"))
		names[i] = h
		cols[h] = i
	}
	g, ok := lottery.DetectGame(names)
	if !ok {
		return ImportResult{}, errors.New("format de CSV non reconnu (ni Loto ni EuroMillions)")
	}
	for _, c := range []string{"date_de_tirage", "jour_de_tirage", "boule_1"} {
		if _, ok := cols[c]; !ok {
			return ImportResult{}, fmt.Errorf("colonne %q absente", c)
		}
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return ImportResult{}, err
	}
	defer tx.Rollback()
	stmt, err := tx.PrepareContext(ctx, `INSERT OR IGNORE INTO tirages VALUES (?,?,?,?,?,?,?,?,?,?,?)`)
	if err != nil {
		return ImportResult{}, err
	}
	defer stmt.Close()

	res := ImportResult{Game: g}
	for {
		rec, err := cr.Read()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return ImportResult{}, fmt.Errorf("ligne %d : %w", res.Read+res.Skipped+2, err)
		}
		d, ok := parseRow(g, cols, rec)
		if !ok {
			res.Skipped++
			continue
		}
		res.Read++
		var c2 any
		if len(d.Comps) > 1 {
			c2 = d.Comps[1]
		}
		r, err := stmt.ExecContext(ctx, g.Code, d.Date.Format(time.DateOnly), d.Day,
			d.Balls[0], d.Balls[1], d.Balls[2], d.Balls[3], d.Balls[4], d.Comps[0], c2, d.Sum())
		if err != nil {
			return ImportResult{}, err
		}
		n, _ := r.RowsAffected()
		res.New += int(n)
	}
	if err := tx.Commit(); err != nil {
		return ImportResult{}, err
	}
	return res, nil
}

func parseRow(g lottery.Game, cols map[string]int, rec []string) (lottery.Draw, bool) {
	field := func(name string) (string, bool) {
		i, ok := cols[name]
		if !ok || i >= len(rec) {
			return "", false
		}
		return strings.TrimSpace(rec[i]), true
	}
	num := func(name string, max int) (int, bool) {
		v, ok := field(name)
		if !ok {
			return 0, false
		}
		n, err := strconv.Atoi(v)
		return n, err == nil && n >= 1 && n <= max
	}

	var d lottery.Draw
	raw, _ := field("date_de_tirage")
	date, ok := parseDate(raw)
	if !ok {
		return d, false
	}
	d.Date = date
	d.Day, _ = field("jour_de_tirage")

	for i := range lottery.BallsPerDraw {
		n, ok := num(fmt.Sprintf("boule_%d", i+1), g.Balls)
		if !ok {
			return d, false
		}
		d.Balls[i] = n
	}
	sort.Ints(d.Balls[:])
	for _, c := range g.CSVColumns {
		n, ok := num(c, g.CompMax)
		if !ok {
			return d, false
		}
		d.Comps = append(d.Comps, n)
	}
	sort.Ints(d.Comps)
	return d, true
}

// parseDate accepts the formats found in FDJ exports over the years.
func parseDate(s string) (time.Time, bool) {
	for _, layout := range []string{"02/01/2006", "2006-01-02", "20060102", "02/01/06"} {
		if t, err := time.Parse(layout, s); err == nil {
			return t, true
		}
	}
	return time.Time{}, false
}

// ImportFile imports one CSV file from disk.
func (s *Store) ImportFile(ctx context.Context, path string) (ImportResult, error) {
	f, err := os.Open(path)
	if err != nil {
		return ImportResult{}, err
	}
	defer f.Close()
	return s.ImportCSV(ctx, f)
}

// ImportDir imports every *.csv file of dir (non recursive), in name order.
func (s *Store) ImportDir(ctx context.Context, dir string) (map[string]ImportResult, error) {
	paths, err := filepath.Glob(filepath.Join(dir, "*.csv"))
	if err != nil {
		return nil, err
	}
	sort.Strings(paths)
	out := make(map[string]ImportResult, len(paths))
	for _, p := range paths {
		r, err := s.ImportFile(ctx, p)
		if err != nil {
			return out, fmt.Errorf("%s : %w", filepath.Base(p), err)
		}
		out[filepath.Base(p)] = r
	}
	return out, nil
}

// Draws returns all draws of a game, oldest first.
func (s *Store) Draws(ctx context.Context, g lottery.Game) ([]lottery.Draw, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT date, jour, b1, b2, b3, b4, b5, c1, c2 FROM tirages WHERE jeu = ? ORDER BY date`, g.Code)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []lottery.Draw
	for rows.Next() {
		var (
			d    lottery.Draw
			date string
			c1   int
			c2   sql.NullInt64
		)
		if err := rows.Scan(&date, &d.Day, &d.Balls[0], &d.Balls[1], &d.Balls[2], &d.Balls[3], &d.Balls[4], &c1, &c2); err != nil {
			return nil, err
		}
		if d.Date, err = time.Parse(time.DateOnly, date); err != nil {
			return nil, err
		}
		d.Comps = []int{c1}
		if c2.Valid {
			d.Comps = append(d.Comps, int(c2.Int64))
		}
		out = append(out, d)
	}
	return out, rows.Err()
}
