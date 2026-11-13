package astronn_test

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"math"
	"os"
	"strings"
	"testing"

	"github.com/janearc/celestial-data"
	"github.com/janearc/celestial-data/astronn"
	"github.com/janearc/celestial-data/internal/fitstest"
)

// The real row: firang's star, row 626007 of the catalogue, fetched on
// 2026-11-06 by range with the header before it, NAXIS2 set to one.
// Each offset is defined by the header's TFORMs, summed in order.
const (
	realFile    = "testdata/star-4081136684118965760.fits"
	headerBytes = 25920 // defined: nine 2880-byte blocks, the header's END in the ninth
	rowBytes    = 882   // defined: the header's NAXIS1
	idAt        = 0     // defined: APOGEE_ID, 18A, the row's first
	idWidth     = 18    // defined: APOGEE_ID's TFORM
	gaiaAt      = 426   // defined: source_id, K, column 76
	realID      = "2M19100020-2214052"
	realGaia    = 4081136684118965760 // defined: firang's star
	onlyRow     = 0                   // defined: the file's one row
)

// at is a column the reader gives as a number, and where its eight
// bytes lie in the row.
type at struct {
	name   string
	offset int
	got    float64
}

// the test: the real row reads as its own bytes say, each number the
// big-endian double at its column's offset, read here without the
// reader; the star is firang's, by its APOGEE and Gaia ids.
func TestTheRealRowReadsAsItsBytes(test *testing.T) {
	raw, err := os.ReadFile(realFile)
	if err != nil {
		test.Fatal(err)
	}
	catalogue, err := astronn.OpenCatalogue(bytes.NewReader(raw))
	if err != nil {
		test.Fatal(err)
	}
	star, err := catalogue.Star(onlyRow)
	if err != nil {
		test.Fatal(err)
	}
	row := raw[headerBytes : headerBytes+rowBytes]
	distance, distanceError := star.Distance()
	weighted, weightedError := star.WeightedDistance()
	for _, each := range []at{{"dist", distAt, distance},
		{"dist_error", distErrorAt, distanceError}, {"weighted_dist", weightedAt, weighted},
		{"weighted_dist_error", weightedErrorAt, weightedError},
		{"age", ageAt, star.AgeUncorrected()},
		{"age_linear_correct", linearAt, star.AgeLinear()},
		{"age_lowess_correct", lowessAt, star.Age()},
		{"age_total_error", ageErrorAt, star.AgeError()},
		{"age_model_error", ageModelAt, star.AgeModelError()},
		{"e", eccentricityAt, star.Eccentricity()},
		{"zmax", zmaxAt, star.HighestAbovePlane()},
		{"rperi", rperiAt, star.Pericentre()}, {"rap", rapAt, star.Apocentre()},
		{"Lz", lzAt, star.AngularMomentum()}, {"Energy", energyAt, star.Energy()}} {
		bits := binary.BigEndian.Uint64(row[each.offset:])
		if want := math.Float64frombits(bits); each.got != want {
			test.Errorf("%s: %v, its bytes say %v", each.name, each.got, want)
		}
	}
	id := strings.Trim(string(row[idAt:idAt+idWidth]), padding)
	scope := strings.Trim(string(row[telescopeAt:telescopeAt+telescopeLen]), padding)
	gaia, known := star.GaiaID()
	source := int64(binary.BigEndian.Uint64(row[gaiaAt:]))
	if star.ID() != id || id != realID || star.Telescope() != scope ||
		scope != "lco25m" || !known || gaia != source || gaia != realGaia {
		test.Errorf("%q %q %d %v", star.ID(), star.Telescope(), gaia, known)
	}
}

// A crafted file cut short, each defined.
const (
	telescopeAt  = 22      // defined: TELESCOPE, after APOGEE_ID and LOCATION_ID
	padding      = " \x00" // defined: what FITS pads text with, spaces or NULs
	blockBytes   = 2880    // defined: a FITS block
	craftedWidth = 154     // defined: a crafted row, 18 + 8 + 15 doubles + a long
)

// the anti-test: a file cut short of its rows is refused by Star and by
// Find, naming the row, and nothing read from it is handed back.
func TestAFileCutShortIsRefused(test *testing.T) {
	whole := fitstest.File(layout(), craftedRow(firstSource),
		craftedRow(secondSource))
	short := whole[:len(whole)-blockBytes+craftedWidth]
	catalogue, err := astronn.OpenCatalogue(bytes.NewReader(short))
	if err != nil {
		test.Fatal(err)
	}
	star, err := catalogue.Star(onlyRow)
	if star != nil || !errors.Is(err, celestialdata.Refused()) ||
		!strings.Contains(err.Error(), "row 0") {
		test.Errorf("a short row: %v, %v", star, err)
	}
	found, err := catalogue.Find(firstSource)
	if found != nil || !errors.Is(err, celestialdata.Refused()) {
		test.Errorf("a short file found %d: %v", len(found), err)
	}
}

// The crafted tables' columns, as the file names them, in its order.
var formsOf = []columnForm{{"APOGEE_ID", "18A"}, {"TELESCOPE", "8A"},
	{"dist", "D"}, {"dist_error", "D"}, {"weighted_dist", "D"},
	{"weighted_dist_error", "D"}, {"age", "D"}, {"age_linear_correct", "D"},
	{"age_lowess_correct", "D"}, {"age_total_error", "D"},
	{"age_model_error", "D"}, {"source_id", "K"}, {"e", "D"}, {"zmax", "D"},
	{"rperi", "D"}, {"rap", "D"}, {"Lz", "D"}, {"Energy", "D"}}

// The crafted rows' numbers, each defined.
const (
	doublesBefore = 9   // defined: the crafted row's doubles before its source
	doublesAfter  = 6   // defined: the doubles after it
	telescopeLen  = 8   // defined: TELESCOPE's width
	firstSource   = 11  // defined: the first crafted row's source
	secondSource  = 22  // defined: the second's
	notASource    = 33  // defined: a source neither row holds
	nullSource    = -1  // defined: the TNULL a crafted source column carries
	ageValue      = 6.5 // defined: the crafted rows' age
	noneFound     = 0   // defined: a search that finds nothing
	secondRow     = 1   // defined: the second crafted row
	beyond        = 2   // defined: a row past the two crafted
	twoRows       = 2   // defined: the crafted rows
)

// layout is the crafted table's columns, with the source column nulled
// at nullSource.
func layout() []fitstest.Column {
	var columns []fitstest.Column
	for _, pair := range formsOf {
		made := fitstest.NewColumn(pair.name, pair.form)
		if pair.name == "source_id" {
			made = made.Nulled(nullSource)
		}
		columns = append(columns, made)
	}
	return columns
}

// craftedRow is a row of the crafted table: its text, its source, and
// every number ageValue.
func craftedRow(source int64) *fitstest.Row {
	row := fitstest.NewRow().Text(realID, idWidth).Text("lco25m", telescopeLen)
	for range doublesBefore {
		row = row.Double(ageValue)
	}
	row = row.Long(source)
	for range doublesAfter {
		row = row.Double(ageValue)
	}
	return row
}

// crafted is the crafted table of two rows, the second's source given,
// opened, or the test stops.
func crafted(test *testing.T, second int64) *astronn.Catalogue {
	test.Helper()
	file := fitstest.File(layout(), craftedRow(firstSource),
		craftedRow(second))
	catalogue, err := astronn.OpenCatalogue(bytes.NewReader(file))
	if err != nil {
		test.Fatal(err)
	}
	return catalogue
}

// the test: Find gives the rows of the sources asked, and none for a
// source no row holds; a source the file left empty is no source, and
// is never found.
func TestFindGivesTheRowsOfTheSourcesAsked(test *testing.T) {
	catalogue := crafted(test, secondSource)
	found, err := catalogue.Find(secondSource)
	if err != nil || len(found) != lone {
		test.Fatalf("%d found, %v", len(found), err)
	}
	if gaia, known := found[onlyRow].GaiaID(); gaia != secondSource || !known ||
		found[onlyRow].Age() != ageValue {
		test.Errorf("%d %v", gaia, known)
	}
	if none, err := catalogue.Find(notASource); len(none) != noneFound || err != nil {
		test.Errorf("a source no row holds: %d, %v", len(none), err)
	}
	empty := crafted(test, nullSource)
	star, err := empty.Star(secondRow)
	if err != nil {
		test.Fatal(err)
	}
	if _, known := star.GaiaID(); known || empty.Rows() != twoRows {
		test.Error("an empty source read as one")
	}
	if none, _ := empty.Find(nullSource); len(none) != noneFound {
		test.Error("an empty source was found")
	}
}

// broken is a crafted table's columns with one changed, and the words
// its refusal must say.
type broken struct {
	columns []fitstest.Column
	said    string
}

// without is the crafted columns but the one named.
func without(name string) []fitstest.Column {
	var columns []fitstest.Column
	for _, column := range layout() {
		if column.Name() != name {
			columns = append(columns, column)
		}
	}
	return columns
}

// with is the crafted columns with the one named made otherwise.
func with(name string, made fitstest.Column) []fitstest.Column {
	columns := layout()
	for at, column := range columns {
		if column.Name() == name {
			columns[at] = made
		}
	}
	return columns
}

// the anti-test: a file the reader cannot read is refused, saying why:
// one that lacks each column read, a column of another kind, of many
// values, or with a unit, and bytes that are no FITS file.
func TestAFileNotReadIsRefused(test *testing.T) {
	var cases []broken
	for _, pair := range formsOf {
		cases = append(cases, broken{without(pair.name),
			fmt.Sprintf("the astroNN file has no %s", pair.name)})
	}
	single := fitstest.NewColumn("age", "E")
	many := fitstest.NewColumn("rap", "2D")
	inParsecs := fitstest.NewColumn("dist", "D").In("pc")
	cases = append(cases,
		broken{with("age", single), "age is of kind E, not D"},
		broken{with("rap", many), "rap holds 2 values a row, not one"},
		broken{with("dist", inParsecs),
			`dist is in "pc", where the astroNN file writes no unit`})
	for _, each := range cases {
		file := fitstest.File(each.columns)
		catalogue, err := astronn.OpenCatalogue(bytes.NewReader(file))
		if catalogue != nil || !errors.Is(err, celestialdata.Refused()) ||
			!strings.Contains(err.Error(), each.said) {
			test.Errorf("%v, which does not say %q", err, each.said)
		}
	}
	catalogue, err := astronn.OpenCatalogue(strings.NewReader("no fits"))
	if catalogue != nil || !errors.Is(err, celestialdata.Refused()) ||
		!strings.Contains(err.Error(), "the astroNN file") {
		test.Errorf("no fits: %v", err)
	}
}

// the anti-test: a row the file does not have is refused, and the file
// is not left unreadable by it.
func TestARowTheFileLacksIsRefused(test *testing.T) {
	catalogue := crafted(test, secondSource)
	star, err := catalogue.Star(beyond)
	if star != nil || err == nil {
		test.Errorf("row %d: %v", beyond, err)
	}
	if again, err := catalogue.Star(onlyRow); again == nil || err != nil {
		test.Errorf("after a refusal: %v", err)
	}
}

// columnForm is a column of the crafted table, by its name and form.
type columnForm struct{ name, form string }

// Where each number the reader gives lies in the real row, each defined
// by the header's TFORMs summed in order, its column's number beside it.
const (
	distAt          = 222 // defined: dist, column 50
	distErrorAt     = 230 // defined: dist_error, 51
	weightedAt      = 286 // defined: weighted_dist, 58
	weightedErrorAt = 294 // defined: weighted_dist_error, 59
	ageAt           = 386 // defined: age, 71
	linearAt        = 394 // defined: age_linear_correct, 72
	lowessAt        = 402 // defined: age_lowess_correct, 73
	ageErrorAt      = 410 // defined: age_total_error, 74
	ageModelAt      = 418 // defined: age_model_error, 75
	eccentricityAt  = 554 // defined: e, 92
	zmaxAt          = 570 // defined: zmax, 94
	rperiAt         = 586 // defined: rperi, 96
	rapAt           = 602 // defined: rap, 98
	lzAt            = 682 // defined: Lz, 108
	energyAt        = 850 // defined: Energy, 129
)
