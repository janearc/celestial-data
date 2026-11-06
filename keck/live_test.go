//go:build live

package keck_test

import (
	"testing"

	"github.com/janearc/celestial-data/internal/replay"
	"github.com/janearc/celestial-data/keck"
)

// recordedProducts is what the recorded search finds: two frames, each
// its raw file and three spectra. It's defined.
const recordedProducts = 8

// the test: KOA still lists the recorded search's frames and their
// spectra. It runs only with go test -tags live. The files are not
// asked for: KOA says no length to a HEAD, and answers a file it lacks
// with a page, not an error.
func TestLiveFramesAreStillListed(test *testing.T) {
	archive, err := keck.New(replay.Live(test), most)
	if err != nil {
		test.Fatal(err)
	}
	found, err := archive.Find(two(test))
	if err != nil || len(found) != recordedProducts {
		test.Errorf("%d products, %v", len(found), err)
	}
}
