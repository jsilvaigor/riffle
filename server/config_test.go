package main

import "testing"

func TestParseRoots(t *testing.T) {
	roots, err := parseRoots("Movies=/media/movies, TV = /media/tv/ ,/media/clips")
	if err != nil {
		t.Fatal(err)
	}
	want := []MediaRoot{{"Movies", "/media/movies"}, {"TV", "/media/tv"}, {"clips", "/media/clips"}}
	if len(roots) != len(want) {
		t.Fatalf("got %v, want %v", roots, want)
	}
	for i := range want {
		if roots[i] != want[i] {
			t.Errorf("root %d: got %v, want %v", i, roots[i], want[i])
		}
	}
}

func TestParseRootsErrors(t *testing.T) {
	for _, in := range []string{"", " , ", "A=/x,A=/y"} {
		if _, err := parseRoots(in); err == nil {
			t.Errorf("parseRoots(%q): expected error", in)
		}
	}
}
