package main

import (
	"testing"

	"github.com/janearc/celestial-data"
)

// productsNamed are products of an archive by their file names.
func productsNamed(t *testing.T, names ...string) []*celestialdata.Product {
	t.Helper()
	base, err := celestialdata.Named("made")
	if err != nil {
		t.Fatal(err)
	}
	var made []*celestialdata.Product
	for _, name := range names {
		product, err := base.Product(name, name, "https://example.org/"+
			name, celestialdata.Unsized, "a made file")
		if err != nil {
			t.Fatal(err)
		}
		made = append(made, product)
	}
	return made
}

// the test: the files a stop leaves unasked are those --match would have
// asked for, not every file found. A counterexample from review 018,
// round 2: three found, one matching, the stop on it leaves none.
func TestAStopCountsOnlyTheMatchingFilesLeft(t *testing.T) {
	found := productsNamed(t, "a-curve.csv", "b-other.csv", "c-other.csv")
	if left := matching(found[thisOne:], "curve"); left != none {
		t.Errorf("%d left, not none", left)
	}
	if left := matching(found[thisOne:], ""); left != len(found)-thisOne {
		t.Errorf("%d left of an empty match, not %d", left,
			len(found)-thisOne)
	}
}
