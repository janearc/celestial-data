//go:build live

package cardiff_test

import (
	"testing"

	"github.com/janearc/celestial-data/cardiff"
	"github.com/janearc/celestial-data/internal/replay"
)

// The live check asks Cardiff itself. It runs only with go test -tags live,
// since it needs the network. It asks each file for its length, never for
// the file.
func TestLiveFindsFilesThatAreThere(t *testing.T) {
	get := replay.Live(t)
	tile, err := cardiff.Footprint()
	if err != nil {
		t.Fatal(err)
	}
	found, err := cardiff.New(get, tile)
	if err != nil {
		t.Fatal(err)
	}
	products, err := found.Find(replay.Asked(t))
	if err != nil || len(products) != tiles*maps {
		t.Fatalf("%d products, %v", len(products), err)
	}
	for _, product := range products {
		length, err := get.Head(product.URL())
		if err != nil || length <= unsized {
			t.Errorf("%s: %d bytes, %v", product.URL(), length, err)
		}
	}
}
