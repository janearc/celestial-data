//go:build live

package chandra_test

import (
	"strings"
	"testing"

	"github.com/janearc/celestial-data/chandra"
	"github.com/janearc/celestial-data/internal/replay"
)

// The live check asks HEASARC and the Chandra archive themselves. It runs
// only with go test -tags live, since it needs the network and the
// archives' answers grow with time.
//
// It asks only the event files for their length, never for the files.
// Asking all 31 files took nine minutes, since each HEAD took the archive
// about 17 seconds.
func TestLiveFindsEventFilesThatAreThere(t *testing.T) {
	get := replay.Live(t)
	archive, err := chandra.New(get, most)
	if err != nil {
		t.Fatal(err)
	}
	products, err := archive.Find(replay.Asked(t))
	if err != nil || len(products) == none {
		t.Fatalf("%d products, %v", len(products), err)
	}
	events := none
	for _, product := range products {
		if !strings.HasSuffix(product.Name(), "_evt2.fits.gz") {
			continue
		}
		events++
		length, err := get.Head(product.URL())
		if err != nil || length <= unsized {
			t.Errorf("%s: %d bytes, %v", product.URL(), length, err)
		}
	}
	if events != most {
		t.Errorf("%d event files for %d observations", events, most)
	}
}
