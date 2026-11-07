package store

import (
	"context"
	"strings"
	"testing"
)

const lotoCSV = "\ufeffannee_numero_de_tirage;jour_de_tirage;date_de_tirage;boule_1;boule_2;boule_3;boule_4;boule_5;numero_chance;\n" +
	"1;MERCREDI;14/10/2026;4;36;32;20;46;5;\n" +
	"2;LUNDI;12/10/2026;35;32;31;45;40;4;\n" +
	"3;SAMEDI;10/10/2026;35;32;;45;40;4;\n"

func TestImportDetectsAndDedups(t *testing.T) {
	s, err := Open("")
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	ctx := context.Background()
	r, err := s.ImportCSV(ctx, strings.NewReader(lotoCSV))
	if err != nil {
		t.Fatal(err)
	}
	if r.Game.Code != "loto" || r.New != 2 || r.Skipped != 1 {
		t.Fatalf("got %+v", r)
	}
	r, _ = s.ImportCSV(ctx, strings.NewReader(lotoCSV))
	if r.New != 0 {
		t.Fatalf("re-import inserted %d", r.New)
	}
	d, err := s.Draws(ctx, r.Game)
	if err != nil || len(d) != 2 {
		t.Fatalf("draws %v %v", d, err)
	}
	if d[1].Balls != [5]int{4, 20, 32, 36, 46} || d[1].Comps[0] != 5 {
		t.Fatalf("latest draw %+v", d[1])
	}
}

func TestImportRejectsUnknownFormat(t *testing.T) {
	s, _ := Open("")
	defer s.Close()
	if _, err := s.ImportCSV(context.Background(), strings.NewReader("a;b;c\n1;2;3\n")); err == nil {
		t.Fatal("expected error")
	}
}
