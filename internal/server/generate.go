package server

import (
	"fmt"
	"math"
	"math/rand/v2"
	"net/http"
	"net/url"
	"strconv"

	"github.com/kmathelot/loto-stats-webapp/internal/lottery"
)

const maxGridsPerStrategy = 5

// generateForm is the state of the "Générer" sidebar.
type generateForm struct {
	Strategy string
	Count    int
	Filters  lottery.Filters
}

func parseGenerateForm(v url.Values) generateForm {
	f := generateForm{Strategy: lottery.StrategyAll, Count: 1, Filters: lottery.Filters{Sum: true, Parity: true, Low: true}}
	if _, ok := lottery.StrategyByID(v.Get("strategie")); ok {
		f.Strategy = v.Get("strategie")
	}
	if n, err := strconv.Atoi(v.Get("nb")); err == nil {
		f.Count = min(max(n, 1), maxGridsPerStrategy)
	}
	// Checkboxes are absent when unchecked: only trust them once the form was submitted.
	if v.Get("envoye") == "1" {
		f.Filters = lottery.Filters{Sum: v.Get("f_somme") != "", Parity: v.Get("f_pairs") != "", Low: v.Get("f_bas") != ""}
	}
	return f
}

type strategyOption struct {
	lottery.Strategy
	Checked bool
}

type filterOption struct {
	Name, Label string
	Checked     bool
}

type generateData struct {
	Form       generateForm
	Strategies []strategyOption
	Filters    []filterOption
	MaxCount   int
	Low        int // low/high threshold
	Odds       string
	Grids      []gridView
	Empty      bool
}

type ballView struct {
	N    int
	Kind string // hot | late | neutral
}

type binBar struct {
	H  int // px
	On bool
}

type gridView struct {
	Tag, Label string
	Balls      []ballView
	Comps      []int
	Copy       string
	VerifyURL  string

	Sum     int
	Bins    []binBar
	SumNote string

	Evens, Odds int
	EvenW       int
	EvenNote    string

	Lows, Highs int
	LowW        int
	LowNote     string

	FreqDelta string
	FreqNote  string
}

func (s *Server) generatePage(w http.ResponseWriter, r *http.Request) {
	p, err := s.page(w, r, "Générer", "generate")
	if err != nil {
		s.fail(w, err)
		return
	}
	p.Data = s.buildGenerate(p.Stats, parseGenerateForm(r.URL.Query()))
	s.render(w, "generate", "layout", p)
}

func (s *Server) generatePartial(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "formulaire invalide", http.StatusBadRequest)
		return
	}
	g := s.currentGame(w, r)
	st, err := s.stats(r.Context(), g)
	if err != nil {
		s.fail(w, err)
		return
	}
	s.render(w, "generate", "grids", s.buildGenerate(st, parseGenerateForm(r.PostForm)))
}

func (s *Server) buildGenerate(st *lottery.Stats, f generateForm) generateData {
	g := st.Game
	d := generateData{
		Form:     f,
		MaxCount: maxGridsPerStrategy,
		Low:      g.LowThreshold(),
		Odds:     thousands(g.JackpotOdds()),
		Filters: []filterOption{
			{"f_somme", fmt.Sprintf("Somme %d–%d", g.SumMin, g.SumMax), f.Filters.Sum},
			{"f_pairs", "1–4 pairs", f.Filters.Parity},
			{"f_bas", fmt.Sprintf("1–4 numéros ≤ %d", g.LowThreshold()), f.Filters.Low},
		},
	}
	for _, s := range lottery.Strategies {
		d.Strategies = append(d.Strategies, strategyOption{s, f.Strategy == s.ID})
	}
	d.Strategies = append(d.Strategies, strategyOption{
		lottery.Strategy{ID: lottery.StrategyAll, Label: "Comparer les 4", Desc: "Une grille par stratégie, côte à côte"},
		f.Strategy == lottery.StrategyAll,
	})
	if st.N() == 0 {
		d.Empty = true
		return d
	}

	ids := []string{f.Strategy}
	if f.Strategy == lottery.StrategyAll {
		ids = ids[:0]
		for _, s := range lottery.Strategies {
			ids = append(ids, s.ID)
		}
	}
	rng := rand.New(rand.NewPCG(rand.Uint64(), rand.Uint64()))
	for _, id := range ids {
		for i := range f.Count {
			grid := st.Generate(id, f.Filters, rng)
			tag := fmt.Sprintf("Grille %d", i+1)
			if f.Strategy == lottery.StrategyAll {
				tag = "Stratégie"
			}
			d.Grids = append(d.Grids, gridViewOf(st, grid, tag))
		}
	}
	return d
}

func gridViewOf(st *lottery.Stats, grid lottery.Grid, tag string) gridView {
	g := st.Game
	strat, _ := lottery.StrategyByID(grid.Strategy)
	v := gridView{
		Tag: tag, Label: strat.Label, Comps: grid.Comps,
		Copy: joinInts(grid.Balls, " ") + " + " + joinInts(grid.Comps, " "),
	}
	q := url.Values{"jeu": {g.Code}}
	for _, b := range grid.Balls {
		q.Add("b", strconv.Itoa(b))
		kind := "neutral"
		switch {
		case st.IsHot(b):
			kind = "hot"
		case st.IsLate(b, false):
			kind = "late"
		}
		v.Balls = append(v.Balls, ballView{b, kind})
	}
	for _, c := range grid.Comps {
		q.Add("c", strconv.Itoa(c))
	}
	v.VerifyURL = "/verifier?" + q.Encode()

	// Sum vs histogram.
	v.Sum = grid.Sum()
	bins := st.SumBins()
	maxPct := 0.0
	for _, b := range bins {
		maxPct = math.Max(maxPct, b.Pct)
	}
	for _, b := range bins {
		v.Bins = append(v.Bins, binBar{H: 3 + int(math.Round(b.Pct/maxPct*38)), On: v.Sum >= b.Lo && v.Sum <= b.Hi})
	}
	bin := st.SumBinOf(v.Sum)
	v.SumNote = fmt.Sprintf("Tranche %d–%d : %s des tirages", bin.Lo, bin.Hi, pct(bin.Pct))

	// Parity and low/high vs observed distribution.
	ev, lo := st.EvenDist(), st.LowDist()
	v.Evens = lottery.Evens(grid.Balls)
	v.Odds = lottery.BallsPerDraw - v.Evens
	v.EvenW = widthPct(ev.Observed[v.Evens], maxOf(ev.Observed[:]))
	v.EvenNote = fmt.Sprintf("%s des tirages ont %s", pct(ev.Observed[v.Evens]), plural(v.Evens, "pair"))
	v.Lows = g.Lows(grid.Balls)
	v.Highs = lottery.BallsPerDraw - v.Lows
	v.LowW = widthPct(lo.Observed[v.Lows], maxOf(lo.Observed[:]))
	v.LowNote = fmt.Sprintf("%s des tirages ont %d bas", pct(lo.Observed[v.Lows]), v.Lows)

	// Mean historical frequency of the chosen balls vs expected.
	exp := st.Expected(false)
	tot, hot, late := 0, 0, 0
	for _, b := range v.Balls {
		tot += st.Freq(b.N, false)
		switch b.Kind {
		case "hot":
			hot++
		case "late":
			late++
		}
	}
	avg := float64(tot) / float64(lottery.BallsPerDraw)
	v.FreqDelta = signedPct(int(math.Round((avg - exp) / exp * 100)))
	v.FreqNote = fmt.Sprintf("%s, %d en retard · vs %s sorties attendues par boule", plural(hot, "chaud"), late, fr1(exp))
	return v
}

func widthPct(v, max float64) int {
	if max == 0 {
		return 0
	}
	return int(math.Round(v / max * 100))
}

func maxOf(xs []float64) float64 {
	m := 0.0
	for _, x := range xs {
		m = math.Max(m, x)
	}
	return m
}
