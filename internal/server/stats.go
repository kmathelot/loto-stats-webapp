package server

import (
	"fmt"
	"math"
	"net/http"

	"github.com/kmathelot/loto-stats-webapp/internal/lottery"
)

type numFreq struct {
	N, F int
	W    int // bar width %
}

type heatCell struct {
	N, F   int
	Pos    bool    // above expected
	Alpha  float64 // 0.08 → 0.63
	Strong bool    // |deviation| > 10 %
}

type gapRow struct {
	N, Gap int
	Seen   string
	Ratio  string
	W      int
	Late   bool
}

type compRow struct {
	N, F, Gap int
	W         int
	Late      bool
}

type binView struct {
	Label string
	Pct   string
	H     int
	Major bool
}

type distRow struct {
	K      int
	Obs    string
	WO, WT int
}

type chiView struct {
	Label string
	Chi   string
	DF    int
	P     string
	Pos   int
	OK    bool
}

type statsData struct {
	Empty bool

	Most, Least, Late int
	MostF, LeastF     int
	LateGap           int
	ChiOK             bool

	Expected  string
	HeatCols  int
	Heat      []heatCell
	Top, Low  []numFreq
	Gaps      []gapRow
	Comps     []compRow
	Bins      []binView
	Sums      lottery.SumSummary
	SumRange  string
	Evens     []distRow
	Lows      []distRow
	LowLabel  string
	Chi       []chiView
	ChiFooter string
}

func (s *Server) statsPage(w http.ResponseWriter, r *http.Request) {
	p, err := s.page(w, r, "Statistiques", "stats")
	if err != nil {
		s.fail(w, err)
		return
	}
	p.Data = buildStats(p.Stats)
	s.render(w, "stats", "layout", p)
}

func buildStats(st *lottery.Stats) statsData {
	g := st.Game
	d := statsData{LowLabel: fmt.Sprintf("Numéros bas (1–%d)", g.LowThreshold())}
	if st.N() == 0 {
		d.Empty = true
		return d
	}
	rank := st.Ranking(false)
	exp := st.Expected(false)
	d.Most, d.Least = rank[0], rank[len(rank)-1]
	d.MostF, d.LeastF = st.Freq(d.Most, false), st.Freq(d.Least, false)
	d.Late = st.ByGap(false)[0]
	d.LateGap = st.Gap(d.Late, false)
	d.Expected = fr1(exp)

	d.HeatCols = 7
	if g.Balls%7 != 0 {
		d.HeatCols = 10
	}
	for n := 1; n <= g.Balls; n++ {
		f := st.Freq(n, false)
		dev := (float64(f) - exp) / exp
		d.Heat = append(d.Heat, heatCell{
			N: n, F: f, Pos: dev >= 0,
			Alpha:  math.Round((0.08+math.Min(math.Abs(dev)/0.2, 1)*0.55)*100) / 100,
			Strong: math.Abs(dev) > 0.1,
		})
	}
	for i := range 10 {
		d.Top = append(d.Top, numFreq{rank[i], st.Freq(rank[i], false), widthPct(float64(st.Freq(rank[i], false)), float64(d.MostF))})
		n := rank[len(rank)-1-i]
		d.Low = append(d.Low, numFreq{n, st.Freq(n, false), widthPct(float64(st.Freq(n, false)), float64(d.MostF))})
	}

	gaps := st.ByGap(false)[:10]
	maxRatio := 0.0
	for _, n := range gaps {
		maxRatio = math.Max(maxRatio, st.Ratio(n, false))
	}
	for _, n := range gaps {
		row := gapRow{N: n, Gap: st.Gap(n, false), Seen: "jamais", Ratio: fr1(st.Ratio(n, false)),
			W: widthPct(st.Ratio(n, false), maxRatio), Late: st.IsLate(n, false)}
		if l := st.LastSeen(n, false); l >= 0 {
			row.Seen = shortDate(st.Draws[l].Date)
		}
		d.Gaps = append(d.Gaps, row)
	}

	maxComp := 0
	for c := 1; c <= g.CompMax; c++ {
		maxComp = max(maxComp, st.Freq(c, true))
	}
	for c := 1; c <= g.CompMax; c++ {
		d.Comps = append(d.Comps, compRow{N: c, F: st.Freq(c, true), Gap: st.Gap(c, true),
			W: widthPct(float64(st.Freq(c, true)), float64(maxComp)), Late: st.IsLate(c, true)})
	}

	bins := st.SumBins()
	maxPct := 0.0
	for _, b := range bins {
		maxPct = math.Max(maxPct, b.Pct)
	}
	for _, b := range bins {
		d.Bins = append(d.Bins, binView{Label: fmt.Sprint(b.Lo), Pct: fr1(b.Pct) + "%",
			H: 2 + int(math.Round(b.Pct/maxPct*140)), Major: b.Pct >= maxPct/2})
	}
	d.Sums = st.Sums()
	d.SumRange = fmt.Sprintf("Dans [%d;%d]", g.SumMin, g.SumMax)

	d.Evens = distRows(st.EvenDist())
	d.Lows = distRows(st.LowDist())

	d.ChiOK = true
	for _, t := range []struct {
		label string
		comp  bool
	}{{fmt.Sprintf("Boules 1–%d", g.Balls), false}, {fmt.Sprintf("%s 1–%d", g.CompTitle, g.CompMax), true}} {
		chi, df, p := st.Chi2(t.comp)
		ok := p > 0.05
		d.ChiOK = d.ChiOK && ok
		d.Chi = append(d.Chi, chiView{Label: t.label, Chi: fr1(chi), DF: df, P: fr2(p), Pos: int(math.Round(p * 100)), OK: ok})
	}
	d.ChiFooter = fmt.Sprintf("fréquence attendue / boule : %s · observée : min %d (n°%d), max %d (n°%d)",
		fr1(exp), d.LeastF, d.Least, d.MostF, d.Most)
	return d
}

func distRows(d lottery.Dist) []distRow {
	m := math.Max(maxOf(d.Observed[:]), maxOf(d.Theo[:]))
	rows := make([]distRow, 0, len(d.Observed))
	for k := range d.Observed {
		rows = append(rows, distRow{K: k, Obs: pct(d.Observed[k]), WO: widthPct(d.Observed[k], m), WT: widthPct(d.Theo[k], m)})
	}
	return rows
}
