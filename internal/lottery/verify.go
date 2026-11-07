package lottery

import (
	"errors"
	"fmt"
	"sort"
)

// ValidateGrid checks a user grid: exactly 5 distinct balls and CompCount
// distinct complementary numbers, all in range. Returns sorted copies.
func (g Game) ValidateGrid(balls, comps []int) ([]int, []int, error) {
	if len(balls) != BallsPerDraw || len(comps) != g.CompCount {
		return nil, nil, fmt.Errorf("il faut %d numéros et %d %s", BallsPerDraw, g.CompCount, g.CompLabel)
	}
	b, err := distinctInRange(balls, g.Balls)
	if err != nil {
		return nil, nil, err
	}
	c, err := distinctInRange(comps, g.CompMax)
	if err != nil {
		return nil, nil, err
	}
	return b, c, nil
}

func distinctInRange(xs []int, max int) ([]int, error) {
	out := append([]int(nil), xs...)
	sort.Ints(out)
	for i, x := range out {
		if x < 1 || x > max {
			return nil, fmt.Errorf("numéro %d hors de 1–%d", x, max)
		}
		if i > 0 && out[i-1] == x {
			return nil, errors.New("numéros en double")
		}
	}
	return out, nil
}

// Match is the result of comparing a grid with one draw.
type Match struct {
	DrawIndex int
	Balls     int // matching balls
	Comps     int // matching complementary numbers
}

// MatchDraw compares a grid against one draw.
func MatchDraw(d Draw, balls, comps []int) Match {
	m := Match{}
	for _, b := range d.Balls {
		m.Balls += count(balls, b)
	}
	for _, c := range d.Comps {
		m.Comps += count(comps, c)
	}
	return m
}

func count(xs []int, v int) int {
	for _, x := range xs {
		if x == v {
			return 1
		}
	}
	return 0
}

// RankCount is the number of past draws that would have won a given rank.
type RankCount struct {
	Rank  Rank
	Count int
}

// Verification is the outcome of playing a grid on every past draw.
type Verification struct {
	Matches []Match     // one per draw, same order as Stats.Draws
	Ranks   []RankCount // one per Game.Ranks
	Wins    int         // draws matching any rank
	Best    []Match     // closest draws, best first
}

// Verify plays a (validated) grid against the whole history.
func (s *Stats) Verify(balls, comps []int, best int) Verification {
	v := Verification{
		Matches: make([]Match, len(s.Draws)),
		Ranks:   make([]RankCount, len(s.Game.Ranks)),
	}
	for i, r := range s.Game.Ranks {
		v.Ranks[i].Rank = r
	}
	for i, d := range s.Draws {
		m := MatchDraw(d, balls, comps)
		m.DrawIndex = i
		v.Matches[i] = m
		for j, r := range s.Game.Ranks {
			if r.Match(m.Balls, m.Comps) {
				v.Ranks[j].Count++
				v.Wins++
				break
			}
		}
	}
	sorted := append([]Match(nil), v.Matches...)
	sort.SliceStable(sorted, func(i, j int) bool {
		si := sorted[i].Balls*(s.Game.CompCount+1) + sorted[i].Comps
		sj := sorted[j].Balls*(s.Game.CompCount+1) + sorted[j].Comps
		if si != sj {
			return si > sj
		}
		return sorted[i].DrawIndex > sorted[j].DrawIndex // most recent first
	})
	if best > len(sorted) {
		best = len(sorted)
	}
	v.Best = sorted[:best]
	return v
}
