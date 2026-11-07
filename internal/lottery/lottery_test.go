package lottery

import (
	"math/rand/v2"
	"testing"
	"time"
)

func draw(day int, balls [5]int, comps ...int) Draw {
	return Draw{Date: time.Date(2024, 1, day, 0, 0, 0, 0, time.UTC), Balls: balls, Comps: comps}
}

func sample3() *Stats {
	return NewStats(Loto, []Draw{
		draw(1, [5]int{1, 2, 3, 4, 5}, 1),
		draw(2, [5]int{1, 10, 20, 30, 40}, 2),
		draw(3, [5]int{2, 11, 21, 31, 41}, 1),
	})
}

func TestFreqAndGap(t *testing.T) {
	s := sample3()
	if got := s.Freq(1, false); got != 2 {
		t.Errorf("freq(1)=%d, want 2", got)
	}
	if got := s.Gap(1, false); got != 1 {
		t.Errorf("gap(1)=%d, want 1", got)
	}
	if got := s.Gap(49, false); got != 3 {
		t.Errorf("gap(never)=%d, want N", got)
	}
	if got := s.Gap(1, true); got != 0 {
		t.Errorf("gap(chance 1)=%d, want 0", got)
	}
	if r := s.Ranking(false); r[0] != 1 || r[1] != 2 {
		t.Errorf("ranking starts %v", r[:2])
	}
}

func TestJackpotOdds(t *testing.T) {
	if got := Loto.JackpotOdds(); got != 19068840 {
		t.Errorf("loto odds %d", got)
	}
	if got := EuroMillions.JackpotOdds(); got != 139838160 {
		t.Errorf("euro odds %d", got)
	}
}

func TestTheoreticalDistSumsTo100(t *testing.T) {
	d := sample3().EvenDist()
	tot := 0.0
	for _, v := range d.Theo {
		tot += v
	}
	if tot < 99.99 || tot > 100.01 {
		t.Errorf("theoretical total %f", tot)
	}
}

func TestGenerateRespectsFilters(t *testing.T) {
	s := sample3()
	r := rand.New(rand.NewPCG(1, 2))
	f := Filters{Sum: true, Parity: true, Low: true}
	for _, st := range []string{"freq", "late", "bal", "rand"} {
		for range 50 {
			g := s.Generate(st, f, r)
			if _, _, err := Loto.ValidateGrid(g.Balls, g.Comps); err != nil {
				t.Fatalf("%s: invalid grid %v: %v", st, g, err)
			}
			if !f.Accepts(Loto, g.Balls) {
				t.Fatalf("%s: grid %v fails filters", st, g.Balls)
			}
		}
	}
}

func TestVerifyRanks(t *testing.T) {
	s := sample3()
	v := s.Verify([]int{1, 2, 3, 4, 5}, []int{1}, 2)
	if v.Ranks[0].Count != 1 {
		t.Errorf("rank 1 count %d", v.Ranks[0].Count)
	}
	if v.Best[0].DrawIndex != 0 || v.Best[0].Balls != 5 {
		t.Errorf("best %+v", v.Best[0])
	}
	if _, _, err := Loto.ValidateGrid([]int{1, 1, 2, 3, 4}, []int{1}); err == nil {
		t.Error("duplicates accepted")
	}
}
