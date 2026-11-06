//go:build galahdata

package galah_test

import (
	"encoding/binary"
	"math"
	"os"
	"testing"

	"github.com/janearc/celestial-data/galah"
)

// fileVariable names the allstar file on disk for the test of it.
const fileVariable = "GALAH_DR4_ALLSTAR"

// The file's layout as its header had it on 2026-11-06, each defined
// by that header: where the rows begin, and each tested column's place
// in a row by the widths of the columns before it.
const (
	headerBlocks = 24     // defined: the primary header's block and the table header's 23
	blockBytes   = 2880   // defined: a FITS block's bytes
	rowBytes     = 826    // defined: NAXIS1
	allRows      = 917588 // defined: NAXIS2
	sobjectAt    = 0      // defined: sobject_id, K
	gaiaAt       = 25     // defined: gaiadr3_source_id, K
	raAt         = 65     // defined: ra, D
	decAt        = 73     // defined: dec, D
	flagSPAt     = 81     // defined: flag_sp, J
	teffAt       = 194    // defined: teff, E
	ironAt       = 210    // defined: fe_h, E
	magnesiumAt  = 298    // defined: mg_fe, E
	mgFlagAt     = 306    // defined: flag_mg_fe, J
	lithiumAt    = 798    // defined: a_li, E
	limitAt      = 802    // defined: a_li_upp_lim, E
	liFlagAt     = 818    // defined: flag_a_li, K
	middleRow    = 458794 // defined: NAXIS2 halved
	lastRow      = 917587 // defined: NAXIS2 less one, counted from none
	roundTrip    = 1e-14  // defined: a double through radians and back, as a share
)

// decoded is a row's tested values, decoded by hand from the file's
// bytes at the layout's places: no fits code in it.
type decoded struct {
	sobject, gaia, liFlag   int64
	ra, dec                 float64
	flagSP, mgFlag          int32
	teff, iron, mg, lithium float32
	limit                   float32
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
		sobject: int64(binary.BigEndian.Uint64(raw[sobjectAt:])),
		gaia:    int64(binary.BigEndian.Uint64(raw[gaiaAt:])),
		liFlag:  int64(binary.BigEndian.Uint64(raw[liFlagAt:])),
		ra:      math.Float64frombits(binary.BigEndian.Uint64(raw[raAt:])),
		dec:     math.Float64frombits(binary.BigEndian.Uint64(raw[decAt:])),
		flagSP:  int32(binary.BigEndian.Uint32(raw[flagSPAt:])),
		mgFlag:  int32(binary.BigEndian.Uint32(raw[mgFlagAt:])),
		teff:    math.Float32frombits(binary.BigEndian.Uint32(raw[teffAt:])),
		iron:    math.Float32frombits(binary.BigEndian.Uint32(raw[ironAt:])),
		mg:      math.Float32frombits(binary.BigEndian.Uint32(raw[magnesiumAt:])),
		lithium: math.Float32frombits(binary.BigEndian.Uint32(raw[lithiumAt:])),
		limit:   math.Float32frombits(binary.BigEndian.Uint32(raw[limitAt:]))}
}

// same is whether two numbers are the same, a value the survey left
// empty being the same as another.
func same(got, want float64) bool {
	return got == want || math.IsNaN(got) && math.IsNaN(want)
}

// near is whether a place's degrees came back from radians as written,
// to a double's rounding.
func near(got, want float64) bool {
	return math.Abs(got-want) <= roundTrip*math.Abs(want)
}

// placedAsWritten is whether a star's place is its row's: none where
// the row's degrees are empty, and otherwise the degrees.
func placedAsWritten(star *galah.Star, want decoded) bool {
	place := star.Place()
	if math.IsNaN(want.ra) && math.IsNaN(want.dec) {
		return place == nil
	}
	return place != nil && near(place.Ascension().InDegrees(), want.ra) &&
		near(place.Declination().InDegrees(), want.dec)
}

// sourceAsWritten is whether a star's Gaia source is its row's: none
// where the row holds the file's TNULL.
func sourceAsWritten(star *galah.Star, want decoded) bool {
	gaia, inGaia := star.GaiaID()
	if want.gaia == longNull {
		return !inGaia
	}
	return inGaia && gaia == want.gaia
}

// the test: the file on disk opens, every column read held to the kind
// its header gives, and its first, middle and last rows read as their
// bytes say, decoded by hand. It runs with go test -tags galahdata and
// GALAH_DR4_ALLSTAR naming the file.
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
	catalogue, err := galah.OpenCatalogue(file)
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
		if !placedAsWritten(star, want) || !sourceAsWritten(star, want) {
			test.Errorf("row %d: at %v, source %v, by hand %+v", row,
				star.Place(), star.SObjectID(), want)
		}
		liFlags, flagged := star.Lithium().Flags()
		if star.SObjectID() != want.sobject ||
			int32(star.Spectroscopic()) != want.flagSP ||
			int32(magnesium.Flags()) != want.mgFlag ||
			flagged != (want.liFlag != longNull) ||
			flagged && int64(liFlags) != want.liFlag {
			test.Errorf("row %d: %d %d %d %d %v, by hand %+v", row,
				star.SObjectID(), star.Spectroscopic(), magnesium.Flags(),
				liFlags, flagged, want)
		}
		if !same(teff, float64(want.teff)) ||
			!same(star.Iron().Value(), float64(want.iron)) ||
			!same(magnesium.Value(), float64(want.mg)) ||
			!same(star.Lithium().Value(), float64(want.lithium)) ||
			!same(star.Lithium().Limit(), float64(want.limit)) {
			test.Errorf("row %d: teff %g fe %g mg %g li %g, by hand %+v", row,
				teff, star.Iron().Value(), magnesium.Value(),
				star.Lithium().Value(), want)
		}
	}
}

// unflaggedLithium is the rows whose flag_a_li the survey left empty,
// counted 2026-11-06 by a scan of the column's TNULLs. It's defined.
const unflaggedLithium = 25364

// the test: every row of the file on disk reads as a star, and as many
// have no lithium flag as the scan counted. A row whose lithium flag
// says UpperLimit has a limit and no value; any other, a value and no
// limit, but a row with no flag, which has neither.
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
	catalogue, err := galah.OpenCatalogue(file)
	if err != nil {
		test.Fatal(err)
	}
	unflagged := 0
	for row := range catalogue.Rows() {
		star, err := catalogue.Star(row)
		if err != nil {
			test.Fatalf("row %d: %v", row, err)
		}
		lithium := star.Lithium()
		flags, flagged := lithium.Flags()
		if !flagged {
			unflagged++
		}
		limited := flagged && flags.Has(galah.UpperLimit)
		valued := flagged && !limited
		if math.IsNaN(lithium.Limit()) == limited ||
			math.IsNaN(lithium.Value()) == valued {
			test.Fatalf("row %d: lithium %g limit %g flags %d, %v", row,
				lithium.Value(), lithium.Limit(), flags, flagged)
		}
	}
	if unflagged != unflaggedLithium {
		test.Errorf("%d rows with no lithium flag", unflagged)
	}
}
