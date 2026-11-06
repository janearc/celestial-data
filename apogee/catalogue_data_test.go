//go:build apogeedata

package apogee_test

import (
	"encoding/binary"
	"math"
	"os"
	"strings"
	"testing"

	"github.com/janearc/celestial-data/apogee"
)

// fileVariable names the allStar file on disk for the test of it.
const fileVariable = "APOGEE_DR17_ALLSTAR"

// The file's layout as its header had it on 2026-11-06, each defined
// by that header: where the rows begin, and each tested column's place
// in a row by the widths of the columns before it.
const (
	headerBlocks = 15     // defined: the primary header's block and the table header's fourteen
	blockBytes   = 2880   // defined: a FITS block's bytes
	rowBytes     = 5410   // defined: NAXIS1
	allRows      = 733901 // defined: NAXIS2
	idAt         = 64     // defined: APOGEE_ID, 30A
	idChars      = 30     // defined: its characters
	raAt         = 360    // defined: RA, D
	decAt        = 368    // defined: DEC, D
	gaiaAt       = 1168   // defined: GAIAEDR3_SOURCE_ID, K
	aspcapAt     = 2888   // defined: ASPCAPFLAG, K
	teffAt       = 4590   // defined: TEFF, E
	magnesiumAt  = 4722   // defined: MG_FE, E
	ironAt       = 4914   // defined: FE_H, E
	ironFlagAt   = 4926   // defined: FE_H_FLAG, J
	roundTrip    = 1e-14  // defined: a double through radians and back, as a share
	middleRow    = 366950 // defined: NAXIS2 halved, rounded down
	lastRow      = 733900 // defined: NAXIS2 less one, counted from none
)

// decoded is a row's tested values, decoded by hand from the file's
// bytes at the layout's places: no fits code in it.
type decoded struct {
	id                    string
	ra, dec               float64
	gaia, aspcap          int64
	teff, magnesium, iron float32
	ironFlag              int32
}

// byHand reads a row's bytes from the file and decodes the tested
// columns, big-endian as the standard stores them, or the test stops.
func byHand(test *testing.T, file *os.File, row int64) decoded {
	test.Helper()
	raw := make([]byte, rowBytes)
	at := int64(headerBlocks*blockBytes) + row*rowBytes
	if _, err := file.ReadAt(raw, at); err != nil {
		test.Fatal(err)
	}
	return decoded{
		id:        strings.TrimRight(string(raw[idAt:idAt+idChars]), " \x00"),
		ra:        math.Float64frombits(binary.BigEndian.Uint64(raw[raAt:])),
		dec:       math.Float64frombits(binary.BigEndian.Uint64(raw[decAt:])),
		gaia:      int64(binary.BigEndian.Uint64(raw[gaiaAt:])),
		aspcap:    int64(binary.BigEndian.Uint64(raw[aspcapAt:])),
		teff:      math.Float32frombits(binary.BigEndian.Uint32(raw[teffAt:])),
		magnesium: math.Float32frombits(binary.BigEndian.Uint32(raw[magnesiumAt:])),
		iron:      math.Float32frombits(binary.BigEndian.Uint32(raw[ironAt:])),
		ironFlag:  int32(binary.BigEndian.Uint32(raw[ironFlagAt:]))}
}

// same is whether two numbers are the same, a value the survey left
// empty being the same as another.
func same(got, want float64) bool {
	return got == want || math.IsNaN(got) && math.IsNaN(want)
}

// the test: the file on disk opens, every column read held to the kind
// its header gives, and its first, middle and last rows read as their
// bytes say, decoded by hand. It runs with go test -tags apogeedata and
// APOGEE_DR17_ALLSTAR naming the file.
func TestTheFileOnDiskReadsAsItsBytesSay(test *testing.T) {
	path := os.Getenv(fileVariable)
	if path == "" {
		test.Skip(fileVariable + " names no file")
	}
	file, err := os.Open(path)
	if err != nil {
		test.Fatal(err)
	}
	defer file.Close()
	catalogue, err := apogee.OpenCatalogue(file)
	if err != nil {
		test.Fatal(err)
	}
	if catalogue.Rows() != allRows {
		test.Errorf("%d rows", catalogue.Rows())
	}
	for _, row := range []int64{firstRow, middleRow, lastRow} {
		want := byHand(test, file, row)
		star, err := catalogue.Star(row)
		if err != nil {
			test.Fatalf("row %d: %v", row, err)
		}
		magnesium, err := star.Abundance("Mg")
		if err != nil {
			test.Fatal(err)
		}
		teff, _ := star.Temperature()
		if !placedAsWritten(star, want) {
			test.Errorf("row %d: placed at %v, by hand %g %g", row,
				star.Place(), want.ra, want.dec)
		}
		if star.ID() != want.id || star.GaiaID() != want.gaia ||
			int64(star.ASPCAP()) != want.aspcap ||
			int32(star.Iron().Flags()) != want.ironFlag {
			test.Errorf("row %d: %q %d %d %d, by hand %+v", row, star.ID(),
				star.GaiaID(), star.ASPCAP(), star.Iron().Flags(), want)
		}
		if !same(teff, float64(want.teff)) ||
			!same(magnesium.Value(), float64(want.magnesium)) ||
			!same(star.Iron().Value(), float64(want.iron)) {
			test.Errorf("row %d: teff %g mg %g fe %g, by hand %+v", row, teff,
				magnesium.Value(), star.Iron().Value(), want)
		}
	}
}

// placedAsWritten is whether a star's place is its row's: none where
// the row's degrees are empty, and otherwise the degrees, to a double's
// rounding through radians.
func placedAsWritten(star *apogee.Star, want decoded) bool {
	place := star.Place()
	if math.IsNaN(want.ra) && math.IsNaN(want.dec) {
		return place == nil
	}
	return place != nil && near(place.Ascension().InDegrees(), want.ra) &&
		near(place.Declination().InDegrees(), want.dec)
}

// near is whether a place's degrees came back from radians as written,
// to a double's rounding.
func near(got, want float64) bool {
	return math.Abs(got-want) <= roundTrip*math.Abs(want)
}

// the test: every row of the file on disk reads as a star, and as many
// have no place as the file's rows of empty degrees.
func TestEveryRowOnDiskReads(test *testing.T) {
	path := os.Getenv(fileVariable)
	if path == "" {
		test.Skip(fileVariable + " names no file")
	}
	file, err := os.Open(path)
	if err != nil {
		test.Fatal(err)
	}
	defer file.Close()
	catalogue, err := apogee.OpenCatalogue(file)
	if err != nil {
		test.Fatal(err)
	}
	nowhere := 0
	for row := range catalogue.Rows() {
		star, err := catalogue.Star(row)
		if err != nil {
			test.Fatalf("row %d: %v", row, err)
		}
		if star.Place() == nil {
			nowhere++
		}
	}
	if nowhere != placeless {
		test.Errorf("%d rows with no place", nowhere)
	}
}

// placeless is the rows whose place the survey left empty: one, its
// first, VESTA, counted 2026-11-06. It's defined.
const placeless = 1
