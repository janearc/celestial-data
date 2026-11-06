//go:build live

package apogee_test

import (
	"testing"

	"github.com/janearc/celestial-data/apogee"
	"github.com/janearc/celestial-data/internal/replay"
)

// the test: the file is still at the Science Archive Server, at the
// length numbers.yaml gives. It runs only with go test -tags live,
// since it needs the network, and asks the file for its length, never
// for the file.
func TestLiveFileIsStillThere(test *testing.T) {
	get := replay.Live(test)
	archive, err := apogee.New()
	if err != nil {
		test.Fatal(err)
	}
	found, err := archive.Find(replay.Asked(test))
	if err != nil || len(found) != lone {
		test.Fatalf("%d products, %v", len(found), err)
	}
	length, err := get.Head(found[first].URL())
	if err != nil || length != allStarBytes {
		test.Errorf("%s: %d bytes, %v", found[first].URL(), length, err)
	}
}
