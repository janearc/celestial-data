//go:build gaiadata

package gaia_test

import (
	"bytes"
	"encoding/binary"
	"os"
	"testing"

	"github.com/janearc/celestial-data/gaia"
)

// productVariable names the product on disk for the test of it.
const productVariable = "GAIA_DR3_RV"

// The product's layout as its header had it on 2026-11-05, each
// defined by that header: where the rows begin and each tested
// column's place in a row, by the forms before it (K8, then D8 and E4
// as the query lists them).
const (
	headerBlocks = 6     // defined: the primary header's block and the table header's five
	rowBytes     = 154   // defined: NAXIS1
	idAt         = 0     // defined: source_id, K
	raAt         = 16    // defined: ra, D, after source_id and ref_epoch
	decAt        = 28    // defined: dec, D, after ra and ra_error
	parallaxAt   = 40    // defined: parallax, D, after dec and dec_error
	radialAt     = 76    // defined: radial_velocity, E, after the proper motion and its errors
	firstOf      = 0     // defined: the first row
	lastOf       = 1     // defined: what the count is over the last row's index
	masPerArcsec = 1e3   // defined: milliarcseconds in an arcsecond
	roundTrip    = 1e-14 // defined: a double through radians, or a thousandth, and back, as a share
)

// decoded is a row's tested values, decoded by hand from the file's
// bytes at the layout's offsets: no fits code in it.
type decoded struct {
	id                int64
	ra, dec, parallax float64
	radial            float32
}

// byHand reads a row's bytes from the file at the layout's place for
// it and decodes the tested columns, big-endian as the standard
// stores them, or the test stops.
func byHand(test *testing.T, file *os.File, row int64) decoded {
	test.Helper()
	raw := make([]byte, rowBytes)
	at := int64(headerBlocks)*block + row*rowBytes
	if _, err := file.ReadAt(raw, at); err != nil {
		test.Fatal(err)
	}
	var got decoded
	for _, each := range []struct {
		at   int
		into any
	}{{idAt, &got.id}, {raAt, &got.ra}, {decAt, &got.dec},
		{parallaxAt, &got.parallax}, {radialAt, &got.radial}} {
		if err := binary.Read(bytes.NewReader(raw[each.at:]), binary.BigEndian,
			each.into); err != nil {
			test.Fatal(err)
		}
	}
	return got
}

// the product on disk, 33 million rows, reads at its first and last
// rows as the bytes at the layout's offsets say, decoded by hand: the
// id, the place, the parallax and the radial velocity each the same.
// It is named by GAIA_DR3_RV, and the test fails when that is unset, so
// that nothing is skipped in silence. Run with -tags gaiadata.
func TestTheProductOnDiskReadsAsItsBytesSay(test *testing.T) {
	path := os.Getenv(productVariable)
	if path == "" {
		test.Fatalf("%s is not set", productVariable)
	}
	file, err := os.Open(path)
	if err != nil {
		test.Fatal(err)
	}
	defer file.Close()
	catalogue, err := gaia.OpenCatalogue(file)
	if err != nil {
		test.Fatal(err)
	}
	for _, row := range []int64{firstOf, catalogue.Rows() - lastOf} {
		source, err := catalogue.Source(row)
		if err != nil {
			test.Fatal(err)
		}
		want := byHand(test, file, row)
		got := decoded{id: source.ID(),
			ra:       source.Place().Ascension().InDegrees(),
			dec:      source.Place().Declination().InDegrees(),
			parallax: source.Parallax().InArcseconds() * masPerArcsec,
			radial:   float32(source.RadialVelocity())}
		if got.id != want.id || !near(got.ra, want.ra, roundTrip) ||
			!near(got.dec, want.dec, roundTrip) ||
			!near(got.parallax, want.parallax, roundTrip) ||
			got.radial != want.radial {
			test.Errorf("row %d reads %+v, its bytes say %+v", row, got, want)
		}
		test.Logf("row %d of %d: id %d at %g %g, parallax %g mas, %g km/s",
			row, catalogue.Rows(), want.id, want.ra, want.dec, want.parallax,
			want.radial)
	}
}
