//go:build live

package nustar_test

import (
	"testing"

	"github.com/janearc/celestial-data/internal/replay"
	"github.com/janearc/celestial-data/nustar"
)

// The live check asks HEASARC itself. It runs only with go test -tags
// live, since it needs the network and HEASARC's answer grows with time.
// It asks for each file's length and never for the file.
func TestLiveFindsFilesThatAreThere(t *testing.T) {
	get := replay.Live(t)
	archive, err := nustar.New(get, most)
	if err != nil {
		t.Fatal(err)
	}
	products, err := archive.Find(replay.Asked(t))
	if err != nil || len(products) == none {
		t.Fatalf("%d products, %v", len(products), err)
	}
	for _, product := range products {
		length, err := get.Head(product.URL())
		if err != nil || length <= unsized {
			t.Errorf("%s: %d bytes, %v", product.URL(), length, err)
		}
	}
}
