//go:build live

package sdssv_test

import (
	"testing"

	"github.com/janearc/celestial-data/internal/replay"
	"github.com/janearc/celestial-data/sdssv"
)

// emptyLength is the length a file has when the server says none. It's
// defined: a file that is there has some bytes.
const emptyLength = 0

// the test: SkyServer still lists the spectra of the recorded search,
// and the first is still at the Science Archive Server. It runs only
// with go test -tags live, and asks the file for its length, never for
// the file.
func TestLiveSpectraAreStillThere(test *testing.T) {
	get := replay.Live(test)
	archive, err := sdssv.New(get, most)
	if err != nil {
		test.Fatal(err)
	}
	found, err := archive.Find(three(test))
	if err != nil || len(found) != most {
		test.Fatalf("%d products, %v", len(found), err)
	}
	length, err := get.Head(found[none].URL())
	if err != nil || length <= emptyLength {
		test.Errorf("%s: %d bytes, %v", found[none].URL(), length, err)
	}
}
