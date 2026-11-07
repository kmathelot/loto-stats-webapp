package lottery

import (
	"math/rand/v2"
	"sort"
)

// Strategy is a grid generation strategy.
type Strategy struct {
	ID    string
	Label string
	Desc  string
}

var Strategies = []Strategy{
	{"freq", "Fréquents", "Pondéré par la fréquence historique"},
	{"late", "En retard", "Pondéré par écart actuel / écart moyen"},
	{"bal", "Équilibré", "2 chauds + 2 en retard + 1 libre"},
	{"rand", "Aléatoire", "Uniforme — le témoin honnête"},
}

// StrategyAll is the pseudo-strategy "one grid per strategy".
const StrategyAll = "all"

func StrategyByID(id string) (Strategy, bool) {
	for _, s := range Strategies {
		if s.ID == id {
			return s, true
		}
	}
	return Strategy{}, false
}

// Filters are the "plausibility" filters applied to generated grids.
type Filters struct {
	Sum    bool // sum within [SumMin;SumMax]
	Parity bool // 1 to 4 even numbers
	Low    bool // 1 to 4 low numbers
}

// Accepts reports whether sorted balls pass the enabled filters.
func (f Filters) Accepts(g Game, balls []int) bool {
	if s := sum(balls); f.Sum && (s < g.SumMin || s > g.SumMax) {
		return false
	}
	if e := Evens(balls); f.Parity && (e < 1 || e > 4) {
		return false
	}
	if l := g.Lows(balls); f.Low && (l < 1 || l > 4) {
		return false
	}
	return true
}

// Grid is a proposed or user-submitted grid.
type Grid struct {
	Strategy string
	Balls    []int // sorted
	Comps    []int // sorted
}

const maxTries = 10000

// Generate builds a grid with the given strategy, retrying until filters pass.
func (s *Stats) Generate(strategy string, f Filters, r *rand.Rand) Grid {
	g := s.Game
	var balls, comps []int
	for range maxTries {
		balls, comps = s.raw(strategy, r)
		sort.Ints(balls)
		if f.Accepts(g, balls) {
			break
		}
	}
	sort.Ints(comps)
	return Grid{Strategy: strategy, Balls: balls, Comps: comps}
}

func (s *Stats) raw(strategy string, r *rand.Rand) (balls, comps []int) {
	g := s.Game
	nums, cnums := seq(g.Balls), seq(g.CompMax)
	switch strategy {
	case "freq":
		balls = weighted(nums, func(n int) float64 { return float64(s.Freq(n, false)) }, BallsPerDraw, r)
		comps = weighted(cnums, func(n int) float64 { return float64(s.Freq(n, true)) }, g.CompCount, r)
	case "late":
		balls = weighted(nums, func(n int) float64 { return s.Ratio(n, false) + 0.05 }, BallsPerDraw, r)
		comps = weighted(cnums, func(n int) float64 { return s.Ratio(n, true) + 0.05 }, g.CompCount, r)
	case "bal":
		hot := s.Ranking(false)[:15]
		late := byRatio(s, nums, false)[:15]
		balls = sample(hot, 2, r)
		balls = append(balls, sample(without(late, balls), 2, r)...)
		balls = append(balls, sample(without(nums, balls), 1, r)...)
		comps = sample(byRatio(s, cnums, true)[:5], g.CompCount, r)
	default:
		balls = sample(nums, BallsPerDraw, r)
		comps = sample(cnums, g.CompCount, r)
	}
	return balls, comps
}

func byRatio(s *Stats, nums []int, comp bool) []int {
	out := append([]int(nil), nums...)
	sort.SliceStable(out, func(i, j int) bool { return s.Ratio(out[i], comp) > s.Ratio(out[j], comp) })
	return out
}

// weighted draws k distinct numbers, each with probability proportional to w.
func weighted(pool []int, w func(int) float64, k int, r *rand.Rand) []int {
	p := append([]int(nil), pool...)
	out := make([]int, 0, k)
	for len(out) < k && len(p) > 0 {
		tot := 0.0
		for _, n := range p {
			tot += w(n)
		}
		x := r.Float64() * tot
		i := 0
		for ; i < len(p)-1; i++ {
			x -= w(p[i])
			if x <= 0 {
				break
			}
		}
		out = append(out, p[i])
		p = append(p[:i], p[i+1:]...)
	}
	return out
}

func sample(pool []int, k int, r *rand.Rand) []int {
	return weighted(pool, func(int) float64 { return 1 }, k, r)
}

func without(pool, excl []int) []int {
	out := make([]int, 0, len(pool))
outer:
	for _, n := range pool {
		for _, e := range excl {
			if n == e {
				continue outer
			}
		}
		out = append(out, n)
	}
	return out
}

// Sum of the balls.
func (g Grid) Sum() int { return sum(g.Balls) }
