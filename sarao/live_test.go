//go:build live

package sarao_test

import (
	"testing"

	"github.com/janearc/celestial-data/internal/replay"
	"github.com/janearc/celestial-data/sarao"
)

// The live check asks SARAO itself. It runs only with go test -tags live,
// since it needs the network. It asks each file for its length, never for
// the file.
func TestLiveFindsFilesThatAreThere(t *testing.T) {
	get := replay.Live(t)
	field, err := sarao.Footprint()
	if err != nil {
		t.Fatal(err)
	}
	mosaic, err := sarao.New(get, field)
	if err != nil {
		t.Fatal(err)
	}
	products, err := mosaic.Find(replay.Asked(t))
	if err != nil || len(products) != files {
		t.Fatalf("%d products, %v", len(products), err)
	}
	for _, product := range products {
		length, err := get.Head(product.URL())
		if err != nil || length <= unsized {
			t.Errorf("%s: %d bytes, %v", product.URL(), length, err)
		}
	}
}
