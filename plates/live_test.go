//go:build live

package plates_test

import (
	"strings"
	"testing"

	"github.com/janearc/celestial-data/internal/replay"
)

// the test: STScI still has the four plates the recorded search found,
// each as long as the recorded one, and still has none for the other
// two. It runs only with go test -tags live. It asks for lengths only
// and never for a file. The page that says there is no plate has a
// length that is no whole number of FITS blocks, or none.
func TestLivePlatesAreStillThere(test *testing.T) {
	get := replay.Live(test)
	for _, survey := range surveys {
		length, err := get.Head(addressOf(survey, oneArcmin))
		if err != nil {
			test.Fatalf("%s: %v", survey, err)
		}
		plated := strings.HasSuffix(recordedAll[survey], ".fits")
		if plated && length != recSize ||
			!plated && length%block == none {
			test.Errorf("%s: length %d", survey, length)
		}
	}
}
