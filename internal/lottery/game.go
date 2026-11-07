// Package lottery holds the domain: game definitions, draws, statistics,
// grid generation and grid verification. It has no I/O dependency.
package lottery

import "time"

// Rank is a prize rank: a predicate on (matching balls, matching complementary numbers).
type Rank struct {
	Label string // "Rang 1"
	Key   string // "5 + C"
	Match func(balls, comps int) bool
}

// Game describes a lottery game (Loto, EuroMillions).
type Game struct {
	Code       string
	Name       string
	Balls      int      // highest ball number (49 / 50)
	CompCount  int      // number of complementary numbers drawn (1 chance / 2 stars)
	CompMax    int      // highest complementary number (10 / 12)
	CompLabel  string   // singular label ("chance", "étoile")
	CompTitle  string   // plural / title label ("Numéro chance", "Étoiles")
	CompShort  string   // short marker in rank keys ("C", "★")
	CSVColumns []string // CSV columns holding the complementary numbers
	SumMin     int      // "plausibility" filter: ~95 % of observed draws
	SumMax     int
	Ranks      []Rank
}

// BallsPerDraw is the number of main balls in a draw for every supported game.
const BallsPerDraw = 5

// LowThreshold is the upper bound of "low" numbers (24 for Loto, 25 for EuroMillions).
func (g Game) LowThreshold() int { return g.Balls / 2 }

// SumTheo is the theoretical mean of the sum of the 5 balls.
func (g Game) SumTheo() float64 { return BallsPerDraw * float64(g.Balls+1) / 2 }

// Jackpot odds: C(balls,5) * C(compMax, compCount).
func (g Game) JackpotOdds() int64 {
	return binom(g.Balls, BallsPerDraw) * binom(g.CompMax, g.CompCount)
}

func exact(b, c int) func(int, int) bool {
	return func(mb, mc int) bool { return mb == b && mc == c }
}

var Loto = Game{
	Code: "loto", Name: "Loto",
	Balls: 49, CompCount: 1, CompMax: 10,
	CompLabel: "chance", CompTitle: "Numéro chance", CompShort: "C",
	CSVColumns: []string{"numero_chance"},
	SumMin:     65, SumMax: 185,
	Ranks: []Rank{
		{"Rang 1", "5 + C", exact(5, 1)},
		{"Rang 2", "5", exact(5, 0)},
		{"Rang 3", "4 + C", exact(4, 1)},
		{"Rang 4", "4", exact(4, 0)},
		{"Rang 5", "3 + C", exact(3, 1)},
		{"Rang 6", "3", exact(3, 0)},
		{"Rang 7", "2 + C", exact(2, 1)},
		{"Rang 8", "2", exact(2, 0)},
		{"Rang 9", "1+C · 0+C", func(b, c int) bool { return b <= 1 && c == 1 }},
	},
}

var EuroMillions = Game{
	Code: "euro", Name: "EuroMillions",
	Balls: 50, CompCount: 2, CompMax: 12,
	CompLabel: "étoile", CompTitle: "Étoiles", CompShort: "★",
	CSVColumns: []string{"etoile_1", "etoile_2"},
	SumMin:     65, SumMax: 185,
	Ranks: []Rank{
		{"Rang 1", "5 + 2★", exact(5, 2)},
		{"Rang 2", "5 + 1★", exact(5, 1)},
		{"Rang 3", "5", exact(5, 0)},
		{"Rang 4", "4 + 2★", exact(4, 2)},
		{"Rang 5", "4 + 1★", exact(4, 1)},
		{"Rang 6", "3 + 2★", exact(3, 2)},
		{"Rang 7", "4", exact(4, 0)},
		{"Rang 8", "2 + 2★", exact(2, 2)},
		{"Rang 9", "3 + 1★", exact(3, 1)},
		{"Rang 10", "3", exact(3, 0)},
		{"Rang 11", "1 + 2★", exact(1, 2)},
		{"Rang 12", "2 + 1★", exact(2, 1)},
		{"Rang 13", "2", exact(2, 0)},
	},
}

// Games lists the supported games, in display order.
var Games = []Game{Loto, EuroMillions}

// GameByCode returns the game with the given code.
func GameByCode(code string) (Game, bool) {
	for _, g := range Games {
		if g.Code == code {
			return g, true
		}
	}
	return Game{}, false
}

// DetectGame finds the game whose complementary columns are all present in a CSV header.
func DetectGame(header []string) (Game, bool) {
	set := make(map[string]bool, len(header))
	for _, h := range header {
		set[h] = true
	}
	for _, g := range Games {
		ok := true
		for _, c := range g.CSVColumns {
			ok = ok && set[c]
		}
		if ok {
			return g, true
		}
	}
	return Game{}, false
}

// Draw is one historical draw.
type Draw struct {
	Date  time.Time
	Day   string // as given by FDJ ("MERCREDI")
	Balls [BallsPerDraw]int
	Comps []int // sorted, len == Game.CompCount
}

// Sum of the 5 balls.
func (d Draw) Sum() int { return sum(d.Balls[:]) }

func sum(xs []int) int {
	s := 0
	for _, x := range xs {
		s += x
	}
	return s
}

func binom(n, k int) int64 {
	if k < 0 || k > n {
		return 0
	}
	r := int64(1)
	for i := 1; i <= k; i++ {
		r = r * int64(n-k+i) / int64(i)
	}
	return r
}
