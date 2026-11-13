//go:build live

package spherex_test

import (
	"io"
	"strings"
	"testing"

	"github.com/janearc/celestial-data/internal/replay"
	"github.com/janearc/celestial-data/spherex"
)

// the test: IRSA still lists the recorded search's images, still serves
// the whole image of the first at a length of whole FITS blocks, and still
// cuts it, the cutout beginning as a FITS file does. It runs only with go
// test -tags live. It asks for a length and a header only: the cutout is
// opened and its first block read, and the rest is left unread. The
// cutout service answers a HEAD with 405, so it is asked with a GET.
func TestLiveTheImagesAreStillThere(test *testing.T) {
	get := replay.Live(test)
	archive, err := spherex.New(get, most)
	if err != nil {
		test.Fatal(err)
	}
	products, err := archive.Find(recorded(test))
	if err != nil || len(products) != most {
		test.Fatalf("%d products, %v", len(products), err)
	}
	image, _, _ := strings.Cut(products[first].URL(), "?")
	length, err := get.Head(image)
	if err != nil || length <= block || length%block != none {
		test.Errorf("the image is %d bytes, %v", length, err)
	}
	body, _, err := get.Get(products[first].URL())
	if err != nil {
		test.Fatal(err)
	}
	defer body.Close()
	start := make([]byte, block)
	if _, err := io.ReadFull(body, start); err != nil ||
		!strings.HasPrefix(string(start), "SIMPLE  =") {
		test.Errorf("the cutout begins %q, %v", start[:len("SIMPLE  =")], err)
	}
}
