//go:build live

package cds_test

import (
	"io"
	"testing"

	"github.com/janearc/celestial-data/cds"
	"github.com/janearc/celestial-data/internal/replay"
)

// fits is how every FITS file begins. It's defined by the FITS standard.
const fits = "SIMPLE  ="

// tiny is the size the live check asks for, the least that still makes a
// FITS file. It's defined: one pixel a side.
const tiny = 1

// The live check asks Strasbourg itself for a cutout one pixel a side and
// reads its first card. It runs only with go test -tags live, since it
// needs the network. When Strasbourg cannot be reached it fails, and New
// takes a mirror's host instead.
func TestLiveDrawsACutout(t *testing.T) {
	cutouts, err := cds.New(cds.Strasbourg, survey, tiny)
	if err != nil {
		t.Fatal(err)
	}
	products, err := cutouts.Find(replay.Asked(t))
	if err != nil {
		t.Fatal(err)
	}
	body, _, err := replay.Live(t).Get(products[only].URL())
	if err != nil {
		t.Fatal(err)
	}
	defer body.Close()
	first := make([]byte, len(fits))
	if _, err := io.ReadFull(body, first); err != nil ||
		string(first) != fits {
		t.Errorf("%q, %v", first, err)
	}
}
