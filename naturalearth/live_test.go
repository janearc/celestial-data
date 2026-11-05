//go:build live

package naturalearth_test

import (
	"testing"

	"github.com/janearc/celestial-data/internal/replay"
	"github.com/janearc/celestial-data/naturalearth"
)

// emptyLength is the length a file has when the server says none. It's
// defined: a file that is there has some bytes.
const emptyLength = 0

// The live check asks the archive itself. It runs only with go test -tags
// live, since it needs the network. It asks the file for its length,
// never for the file.
func TestLiveFileIsStillThere(t *testing.T) {
	get := replay.Live(t)
	archive, err := naturalearth.New()
	if err != nil {
		t.Fatal(err)
	}
	products, err := archive.Find(replay.Asked(t))
	if err != nil || len(products) != lone {
		t.Fatalf("%d products, %v", len(products), err)
	}
	length, err := get.Head(products[first].URL())
	if err != nil || length <= emptyLength {
		t.Errorf("%s: %d bytes, %v", products[first].URL(), length, err)
	}
}
