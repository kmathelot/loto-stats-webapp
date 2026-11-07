package server

import (
	"fmt"
	"math/rand/v2"
	"net/http"
	"net/url"
	"slices"
	"strconv"

	"github.com/kmathelot/loto-stats-webapp/internal/lottery"
)

const historyPageSize = 12

type pickCell struct {
	N  int
	On bool
}

type drawBall struct {
	N   int
	Hit bool
}

type drawRow struct {
	Day, Date string
	Balls     []drawBall
	Comps     []drawBall
	Sum       int
	Evens     int
	Score     string
}

type rankTile struct {
	Label, Key string
	Count      int
}

type verifyData struct {
	Balls, Comps []int // current selection, sorted
	Cells        []pickCell
	CompCells    []pickCell
	Ready        bool
	CompName     string // "chance" / "étoiles"
	CompHint     string // what is missing to verify
	OOB          bool   // htmx response: add out-of-band swaps

	Verdict string
	Ranks   []rankTile
	Best    []drawRow

	Q          string
	Page       int
	PrevPage   int
	NextPage   int
	HasPrev    bool
	HasNext    bool
	RangeLabel string
	Rows       []drawRow
	Empty      bool
}

func (s *Server) verifyPage(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	g := s.currentGame(w, r)

	// "Grille au hasard": redirect to the canonical URL of a random grid.
	if q.Get("hasard") != "" {
		balls := rand.Perm(g.Balls)[:lottery.BallsPerDraw]
		comps := rand.Perm(g.CompMax)[:g.CompCount]
		v := url.Values{"jeu": {g.Code}}
		for _, b := range balls {
			v.Add("b", strconv.Itoa(b+1))
		}
		for _, c := range comps {
			v.Add("c", strconv.Itoa(c+1))
		}
		http.Redirect(w, r, "/verifier?"+v.Encode(), http.StatusSeeOther)
		return
	}

	st, err := s.stats(r.Context(), g)
	if err != nil {
		s.fail(w, err)
		return
	}
	d := buildVerify(st, q)
	if isHTMX(r) {
		d.OOB = true
		w.Header().Set("HX-Push-Url", canonicalVerifyURL(g, d))
		s.render(w, "verify", "verify-results", d)
		return
	}
	p, err := s.page(w, r, "Vérifier & historique", "verify")
	if err != nil {
		s.fail(w, err)
		return
	}
	p.Data = d
	s.render(w, "verify", "layout", p)
}

func canonicalVerifyURL(g lottery.Game, d verifyData) string {
	v := url.Values{"jeu": {g.Code}}
	for _, b := range d.Balls {
		v.Add("b", strconv.Itoa(b))
	}
	for _, c := range d.Comps {
		v.Add("c", strconv.Itoa(c))
	}
	if d.Q != "" {
		v.Set("q", d.Q)
	}
	if d.Page > 0 {
		v.Set("page", strconv.Itoa(d.Page))
	}
	return "/verifier?" + v.Encode()
}

// parseNums keeps distinct in-range numbers, at most limit of them.
func parseNums(vals []string, max, limit int) []int {
	var out []int
	for _, s := range vals {
		n, err := strconv.Atoi(s)
		if err != nil || n < 1 || n > max || slices.Contains(out, n) {
			continue
		}
		if len(out) == limit {
			break
		}
		out = append(out, n)
	}
	slices.Sort(out)
	return out
}

func buildVerify(st *lottery.Stats, q url.Values) verifyData {
	g := st.Game
	d := verifyData{
		Balls: parseNums(q["b"], g.Balls, lottery.BallsPerDraw),
		Comps: parseNums(q["c"], g.CompMax, g.CompCount),
		Q:     q.Get("q"),
		Empty: st.N() == 0,
	}
	d.CompName, d.CompHint = g.CompLabel, "un numéro chance"
	if g.CompCount > 1 {
		d.CompName, d.CompHint = g.CompLabel+"s", fmt.Sprintf("%d %ss", g.CompCount, g.CompLabel)
	}
	for n := 1; n <= g.Balls; n++ {
		d.Cells = append(d.Cells, pickCell{n, slices.Contains(d.Balls, n)})
	}
	for n := 1; n <= g.CompMax; n++ {
		d.CompCells = append(d.CompCells, pickCell{n, slices.Contains(d.Comps, n)})
	}

	var matches []lottery.Match
	if b, c, err := g.ValidateGrid(d.Balls, d.Comps); err == nil && !d.Empty {
		d.Ready = true
		v := st.Verify(b, c, 5)
		matches = v.Matches
		for _, rc := range v.Ranks {
			d.Ranks = append(d.Ranks, rankTile{rc.Rank.Label, rc.Rank.Key, rc.Count})
		}
		if v.Ranks[0].Count > 0 {
			d.Verdict = "déjà sortie en entier !"
		} else {
			d.Verdict = fmt.Sprintf("jamais sortie en entier · %d tirages gagnants", v.Wins)
		}
		for _, m := range v.Best {
			row := rowOf(st, m.DrawIndex, d.Balls, d.Comps)
			row.Score = plural(m.Balls, "bon")
			if m.Comps > 0 {
				row.Score += " + " + g.CompLabel
				if m.Comps > 1 {
					row.Score = fmt.Sprintf("%s + %d %ss", plural(m.Balls, "bon"), m.Comps, g.CompLabel)
				}
			}
			d.Best = append(d.Best, row)
		}
	}

	// History, newest first, optionally filtered on a number.
	var idx []int
	qn, _ := strconv.Atoi(d.Q)
	for i := st.N() - 1; i >= 0; i-- {
		if qn >= 1 && qn <= g.Balls && !slices.Contains(st.Draws[i].Balls[:], qn) {
			continue
		}
		idx = append(idx, i)
	}
	pages := max(1, (len(idx)+historyPageSize-1)/historyPageSize)
	d.Page, _ = strconv.Atoi(q.Get("page"))
	d.Page = min(max(d.Page, 0), pages-1)
	start := d.Page * historyPageSize
	end := min(start+historyPageSize, len(idx))
	d.HasPrev, d.HasNext = d.Page > 0, d.Page < pages-1
	d.PrevPage, d.NextPage = d.Page-1, d.Page+1
	if len(idx) == 0 {
		d.RangeLabel = "0 sur 0"
	} else {
		d.RangeLabel = fmt.Sprintf("%d–%d sur %d", start+1, end, len(idx))
	}
	for _, i := range idx[start:end] {
		row := rowOf(st, i, d.Balls, d.Comps)
		if len(d.Balls) > 0 || len(d.Comps) > 0 {
			m := lottery.MatchDraw(st.Draws[i], d.Balls, d.Comps)
			if matches != nil {
				m = matches[i]
			}
			row.Score = strconv.Itoa(m.Balls)
			if m.Comps > 0 {
				row.Score += " + " + g.CompShort
				if m.Comps > 1 {
					row.Score = fmt.Sprintf("%d + %d%s", m.Balls, m.Comps, g.CompShort)
				}
			}
		}
		d.Rows = append(d.Rows, row)
	}
	return d
}

func rowOf(st *lottery.Stats, i int, balls, comps []int) drawRow {
	dr := st.Draws[i]
	row := drawRow{Day: shortDay(dr.Day), Date: shortDate(dr.Date), Sum: dr.Sum(), Evens: lottery.Evens(dr.Balls[:])}
	for _, b := range dr.Balls {
		row.Balls = append(row.Balls, drawBall{b, slices.Contains(balls, b)})
	}
	for _, c := range dr.Comps {
		row.Comps = append(row.Comps, drawBall{c, slices.Contains(comps, c)})
	}
	return row
}
