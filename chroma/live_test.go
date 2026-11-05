//go:build live

package chroma_test

import (
	"testing"

	"github.com/janearc/celestial-data/chroma"
	"github.com/janearc/celestial-data/internal/replay"
)

// unsized is the length of nothing. It's defined.
const unsized = 0

// The live check asks GitHub itself. It runs only with go test -tags
// live, since it needs the network. It asks each file for its length,
// never for the file.
func TestLiveFindsColourwaysThatAreThere(t *testing.T) {
	get := replay.Live(t)
	colours, err := chroma.New(get)
	if err != nil {
		t.Fatal(err)
	}
	products, err := colours.Find(replay.Asked(t))
	if err != nil || len(products) == unsized {
		t.Fatalf("%d products, %v", len(products), err)
	}
	for _, product := range products {
		length, err := get.Head(product.URL())
		if err != nil || length <= unsized {
			t.Errorf("%s: %d bytes, %v", product.URL(), length, err)
		}
	}
}
