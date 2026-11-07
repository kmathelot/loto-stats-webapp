package lottery

import (
	"math"
	"sort"
)

// Stats holds pre-computed statistics for one game. It is immutable once built
// and safe for concurrent reads.
type Stats struct {
	Game  Game
	Draws []Draw // ascending by date

	freq, freqComp []int // index = number
	last, lastComp []int // index of the last draw where the number appeared, -1 if never
	sums           []int // sorted sums
	hot            map[int]bool
}

// NewStats computes statistics from draws sorted by ascending date.
func NewStats(g Game, draws []Draw) *Stats {
	s := &Stats{
		Game:     g,
		Draws:    draws,
		freq:     make([]int, g.Balls+1),
		freqComp: make([]int, g.CompMax+1),
		last:     filled(g.Balls+1, -1),
		lastComp: filled(g.CompMax+1, -1),
		sums:     make([]int, 0, len(draws)),
	}
	for i, d := range draws {
		for _, b := range d.Balls {
			s.freq[b]++
			s.last[b] = i
		}
		for _, c := range d.Comps {
			s.freqComp[c]++
			s.lastComp[c] = i
		}
		s.sums = append(s.sums, d.Sum())
	}
	sort.Ints(s.sums)
	s.hot = make(map[int]bool, 10)
	if len(draws) > 0 {
		for _, x := range s.Ranking(false)[:10] {
			s.hot[x] = true
		}
	}
	return s
}

func filled(n, v int) []int {
	out := make([]int, n)
	for i := range out {
		out[i] = v
	}
	return out
}

// N is the number of draws.
func (s *Stats) N() int { return len(s.Draws) }

// Max returns the highest number of the range (balls or complementary).
func (s *Stats) Max(comp bool) int {
	if comp {
		return s.Game.CompMax
	}
	return s.Game.Balls
}

// Freq is the number of times n was drawn.
func (s *Stats) Freq(n int, comp bool) int {
	if comp {
		return s.freqComp[n]
	}
	return s.freq[n]
}

// Expected is the expected frequency of a number under uniform draws.
func (s *Stats) Expected(comp bool) float64 {
	if comp {
		return float64(s.Game.CompCount*s.N()) / float64(s.Game.CompMax)
	}
	return float64(BallsPerDraw*s.N()) / float64(s.Game.Balls)
}

// LastSeen returns the index of the last draw containing n, or -1.
func (s *Stats) LastSeen(n int, comp bool) int {
	if comp {
		return s.lastComp[n]
	}
	return s.last[n]
}

// Gap is the number of draws since n last appeared (0 = in the latest draw).
func (s *Stats) Gap(n int, comp bool) int {
	l := s.LastSeen(n, comp)
	if l < 0 {
		return s.N()
	}
	return s.N() - 1 - l
}

// MeanGap is the average number of draws between two appearances of n.
func (s *Stats) MeanGap(n int, comp bool) float64 {
	f := s.Freq(n, comp)
	if f == 0 {
		return math.Inf(1)
	}
	return float64(s.N()) / float64(f)
}

// Ratio is current gap / mean gap: ≥ 2 means "late".
func (s *Stats) Ratio(n int, comp bool) float64 {
	return float64(s.Gap(n, comp)) / s.MeanGap(n, comp)
}

// Ranking returns numbers sorted by descending frequency (ties: ascending number).
func (s *Stats) Ranking(comp bool) []int {
	nums := seq(s.Max(comp))
	sort.SliceStable(nums, func(i, j int) bool {
		fi, fj := s.Freq(nums[i], comp), s.Freq(nums[j], comp)
		if fi != fj {
			return fi > fj
		}
		return nums[i] < nums[j]
	})
	return nums
}

// ByGap returns numbers sorted by descending current gap.
func (s *Stats) ByGap(comp bool) []int {
	nums := seq(s.Max(comp))
	sort.SliceStable(nums, func(i, j int) bool {
		gi, gj := s.Gap(nums[i], comp), s.Gap(nums[j], comp)
		if gi != gj {
			return gi > gj
		}
		return nums[i] < nums[j]
	})
	return nums
}

// IsHot reports whether n is in the top 10 most drawn balls.
func (s *Stats) IsHot(n int) bool { return s.hot[n] }

// IsLate reports whether n's current gap is at least twice its mean gap.
func (s *Stats) IsLate(n int, comp bool) bool { return s.Ratio(n, comp) >= 2 }

// Chi2 runs a χ² uniformity test; p is approximated with Wilson–Hilferty.
func (s *Stats) Chi2(comp bool) (stat float64, df int, p float64) {
	max := s.Max(comp)
	exp := s.Expected(comp)
	if exp == 0 {
		return 0, max - 1, 1
	}
	for n := 1; n <= max; n++ {
		d := float64(s.Freq(n, comp)) - exp
		stat += d * d / exp
	}
	df = max - 1
	k := float64(df)
	z := (math.Cbrt(stat/k) - (1 - 2/(9*k))) / math.Sqrt(2/(9*k))
	p = 0.5 * math.Erfc(z/math.Sqrt2)
	return stat, df, p
}

// SumBin is one bucket of the sum histogram.
type SumBin struct {
	Lo, Hi int
	Count  int
	Pct    float64
}

// SumStep is the width of the sum histogram buckets.
const SumStep = 20

// SumBins builds the histogram of the sum of the 5 balls.
func (s *Stats) SumBins() []SumBin {
	if s.N() == 0 {
		return nil
	}
	lo := s.sums[0] / SumStep * SumStep
	hi := s.sums[len(s.sums)-1] / SumStep * SumStep
	var bins []SumBin
	for b := lo; b <= hi; b += SumStep {
		bins = append(bins, SumBin{Lo: b, Hi: b + SumStep - 1})
	}
	for _, v := range s.sums {
		bins[(v/SumStep*SumStep-lo)/SumStep].Count++
	}
	for i := range bins {
		bins[i].Pct = 100 * float64(bins[i].Count) / float64(s.N())
	}
	return bins
}

// SumBinOf returns the histogram bucket containing sum v (Count 0 if outside history).
func (s *Stats) SumBinOf(v int) SumBin {
	for _, b := range s.SumBins() {
		if v >= b.Lo && v <= b.Hi {
			return b
		}
	}
	lo := v / SumStep * SumStep
	return SumBin{Lo: lo, Hi: lo + SumStep - 1}
}

// SumSummary describes the distribution of sums.
type SumSummary struct {
	Mean, StdDev, Theo float64
	Min, Max, Median   int
	InFilterPct        float64 // within [SumMin;SumMax]
	InCorePct          float64 // within [100;150]
}

func (s *Stats) Sums() SumSummary {
	n := s.N()
	out := SumSummary{Theo: s.Game.SumTheo()}
	if n == 0 {
		return out
	}
	tot, inF, inC := 0, 0, 0
	for _, v := range s.sums {
		tot += v
		if v >= s.Game.SumMin && v <= s.Game.SumMax {
			inF++
		}
		if v >= 100 && v <= 150 {
			inC++
		}
	}
	out.Mean = float64(tot) / float64(n)
	var sq float64
	for _, v := range s.sums {
		d := float64(v) - out.Mean
		sq += d * d
	}
	out.StdDev = math.Sqrt(sq / float64(n))
	out.Min, out.Max, out.Median = s.sums[0], s.sums[n-1], s.sums[n/2]
	out.InFilterPct = 100 * float64(inF) / float64(n)
	out.InCorePct = 100 * float64(inC) / float64(n)
	return out
}

// PercentileOfSum is the share of draws whose sum is ≤ v.
func (s *Stats) PercentileOfSum(v int) float64 {
	if s.N() == 0 {
		return 0
	}
	return 100 * float64(sort.SearchInts(s.sums, v+1)) / float64(s.N())
}

// Evens counts even numbers; Lows counts numbers ≤ the game's low threshold.
func Evens(balls []int) int {
	c := 0
	for _, b := range balls {
		if b%2 == 0 {
			c++
		}
	}
	return c
}

func (g Game) Lows(balls []int) int {
	c := 0
	for _, b := range balls {
		if b <= g.LowThreshold() {
			c++
		}
	}
	return c
}

// Dist is an observed vs theoretical distribution over k = 0..5.
type Dist struct {
	Observed [BallsPerDraw + 1]float64 // % of draws
	Theo     [BallsPerDraw + 1]float64 // % (hypergeometric)
}

// EvenDist: number of even balls per draw.
func (s *Stats) EvenDist() Dist {
	return s.dist(Evens, s.Game.Balls/2)
}

// LowDist: number of low balls per draw.
func (s *Stats) LowDist() Dist {
	return s.dist(s.Game.Lows, s.Game.LowThreshold())
}

func (s *Stats) dist(count func([]int) int, successes int) Dist {
	var d Dist
	for _, dr := range s.Draws {
		d.Observed[count(dr.Balls[:])]++
	}
	total := float64(binom(s.Game.Balls, BallsPerDraw))
	for k := 0; k <= BallsPerDraw; k++ {
		if s.N() > 0 {
			d.Observed[k] = 100 * d.Observed[k] / float64(s.N())
		}
		d.Theo[k] = 100 * float64(binom(successes, k)*binom(s.Game.Balls-successes, BallsPerDraw-k)) / total
	}
	return d
}

func seq(n int) []int {
	out := make([]int, n)
	for i := range out {
		out[i] = i + 1
	}
	return out
}
