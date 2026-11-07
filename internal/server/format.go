package server

import (
	"fmt"
	"html/template"
	"strconv"
	"strings"
	"time"
)

// French number formatting: decimal comma, non-breaking space as thousands separator.

func frFloat(f float64, prec int) string {
	return strings.Replace(strconv.FormatFloat(f, 'f', prec, 64), ".", ",", 1)
}

func fr1(f float64) string { return frFloat(f, 1) }
func fr2(f float64) string { return frFloat(f, 2) }
func pct(f float64) string { return frFloat(f, 1) + " %" }
func signedPct(d int) string {
	if d > 0 {
		return fmt.Sprintf("+%d %%", d)
	}
	return fmt.Sprintf("%d %%", d)
}

func thousands(n int64) string {
	s := strconv.FormatInt(n, 10)
	var b strings.Builder
	for i, r := range s {
		if i > 0 && (len(s)-i)%3 == 0 {
			b.WriteString(" ")
		}
		b.WriteRune(r)
	}
	return b.String()
}

func shortDate(t time.Time) string { return t.Format("02/01/06") }

var dayAbbrev = map[string]string{
	"LUNDI": "lun.", "MARDI": "mar.", "MERCREDI": "mer.", "JEUDI": "jeu.",
	"VENDREDI": "ven.", "SAMEDI": "sam.", "DIMANCHE": "dim.",
}

func shortDay(d string) string {
	if a, ok := dayAbbrev[strings.ToUpper(strings.TrimSpace(d))]; ok {
		return a
	}
	return strings.ToLower(d)
}

func plural(n int, word string) string {
	if n > 1 {
		return fmt.Sprintf("%d %ss", n, word)
	}
	return fmt.Sprintf("%d %s", n, word)
}

func joinInts(xs []int, sep string) string {
	parts := make([]string, len(xs))
	for i, x := range xs {
		parts[i] = strconv.Itoa(x)
	}
	return strings.Join(parts, sep)
}

var funcs = template.FuncMap{
	"fr1":       fr1,
	"fr2":       fr2,
	"pct":       pct,
	"thousands": thousands,
	"shortDate": shortDate,
	"shortDay":  shortDay,
	"join":      joinInts,
	"add":       func(a, b int) int { return a + b },
	"seq": func(n int) []int {
		out := make([]int, n)
		for i := range out {
			out[i] = i + 1
		}
		return out
	},
}
