//go:build live

package lamost_test

import (
	"testing"

	"github.com/janearc/celestial-data/internal/replay"
	"github.com/janearc/celestial-data/lamost"
)

// spectra is how many a live search of the recorded circle finds: three
// of each resolution. It's defined.
const spectra = 2 * most

// the test: LAMOST's service still lists the spectra of the recorded
// search. It runs only with go test -tags live. The files are not
// asked for: LAMOST says no length for them, and answers an obsid it
// lacks with a page, not an error.
func TestLiveSpectraAreStillListed(test *testing.T) {
	archive, err := lamost.New(replay.Live(test), most)
	if err != nil {
		test.Fatal(err)
	}
	found, err := archive.Find(three(test))
	if err != nil || len(found) != spectra {
		test.Errorf("%d products, %v", len(found), err)
	}
}
