package starhorse_test

import (
	"bytes"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/janearc/celestial-data"
	"github.com/janearc/celestial-data/internal/fitstest"
	"github.com/janearc/celestial-data/starhorse"
)

// columnForm is a column of the crafted table, by its name and form.
type columnForm struct{ name, form string }

// The crafted table's columns, as the file names them, in its order:
// the id, the source, seventeen singles and three flags.
var formsOf = []columnForm{{"APOGEE_ID", "19A"}, {"DR3_source_id", "K"},
	{"mass16", "E"}, {"mass50", "E"}, {"mass84", "E"}, {"teff50", "E"},
	{"logg50", "E"}, {"met50", "E"}, {"dist16", "E"}, {"dist50", "E"},
	{"dist84", "E"}, {"AV16", "E"}, {"AV50", "E"}, {"AV84", "E"},
	{"age05", "E"}, {"age16", "E"}, {"age50", "E"}, {"age84", "E"},
	{"age95", "E"}, {"StarHorse_OUTPUTFLAGS", "80A"},
	{"StarHorse_AGEFLAG", "50A"}, {"StarHorse_AGE_INOUT", "50A"}}

// The crafted rows' numbers, each defined.
const (
	idWidth      = 19   // defined: APOGEE_ID's TFORM
	outputWidth  = 80   // defined: StarHorse_OUTPUTFLAGS's
	flagWidth    = 50   // defined: the age flag's and warning's
	singles      = 17   // defined: the crafted row's numbers
	firstSource  = 11   // defined: the first crafted row's source
	secondSource = 22   // defined: the second's
	notASource   = 33   // defined: a source neither row holds
	nullSource   = -1   // defined: the TNULL a crafted source column carries
	value        = 1.5  // defined: every crafted number
	onlyRow      = 0    // defined: the first row
	secondRow    = 1    // defined: the second
	beyond       = 2    // defined: a row past the two crafted
	noneFound    = 0    // defined: none found
	blockBytes   = 2880 // defined: a FITS block
	rowWidth     = 255  // defined: 19 + 8 + 17 singles + 80 + 50 + 50
	craftedID    = "2M19100020-2214052"
	craftedAge   = "NOAGE"
)

// layout is the crafted table's columns, the source nulled at nullSource.
func layout() []fitstest.Column {
	var columns []fitstest.Column
	for _, pair := range formsOf {
		made := fitstest.NewColumn(pair.name, pair.form)
		if pair.name == "DR3_source_id" {
			made = made.Nulled(nullSource)
		}
		columns = append(columns, made)
	}
	return columns
}

// craftedRow is a row of the crafted table: its id, its source, every
// number value, and the flags.
func craftedRow(source int64) *fitstest.Row {
	row := fitstest.NewRow().Text(craftedID, idWidth).Long(source)
	for range singles {
		row = row.Single(value)
	}
	return row.Text("output", outputWidth).Text(craftedAge, flagWidth).
		Text("none", flagWidth)
}

// crafted is the crafted table of two rows, the second's source given,
// opened, or the test stops.
func crafted(test *testing.T, second int64) *starhorse.Catalogue {
	test.Helper()
	file := fitstest.File(layout(), craftedRow(firstSource),
		craftedRow(second))
	catalogue, err := starhorse.OpenCatalogue(bytes.NewReader(file))
	if err != nil {
		test.Fatal(err)
	}
	return catalogue
}

// the test: a row reads as written, each spread its three columns, the
// flags as text; Find gives the rows of the sources asked and none for
// another, and a source left empty is no source and never found.
func TestARowReadsAsWrittenAndIsFound(test *testing.T) {
	catalogue := crafted(test, secondSource)
	found, err := catalogue.Find(secondSource)
	if err != nil || len(found) != lone {
		test.Fatalf("%d found, %v", len(found), err)
	}
	star := found[onlyRow]
	low, high := star.AgeWide()
	gaia, known := star.GaiaID()
	if gaia != secondSource || !known || star.ID() != craftedID ||
		star.Mass().Median() != value || star.Mass().Low() != value ||
		star.Mass().High() != value || star.Distance().Median() != value ||
		star.Extinction().Median() != value || star.Age().Median() != value ||
		low != value || high != value || star.Temperature() != value ||
		star.Gravity() != value || star.Metals() != value ||
		star.AgeFlag() != craftedAge || star.OutputFlags() != "output" ||
		star.AgeWarning() != "none" {
		test.Errorf("%+v", star)
	}
	if none, err := catalogue.Find(notASource); len(none) != noneFound ||
		err != nil {
		test.Errorf("a source no row holds: %d, %v", len(none), err)
	}
	empty := crafted(test, nullSource)
	unmatched, err := empty.Star(secondRow)
	if err != nil {
		test.Fatal(err)
	}
	if _, known := unmatched.GaiaID(); known || empty.Rows() != beyond {
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
			fmt.Sprintf("the StarHorse file has no %s", pair.name)})
	}
	double := fitstest.NewColumn("age50", "D")
	many := fitstest.NewColumn("mass50", "2E")
	inKpc := fitstest.NewColumn("dist50", "E").In("kpc")
	cases = append(cases,
		broken{with("age50", double), "age50 is of kind D, not E"},
		broken{with("mass50", many), "mass50 holds 2 values a row, not one"},
		broken{with("dist50", inKpc),
			`dist50 is in "kpc", where the StarHorse file writes no unit`})
	for _, each := range cases {
		file := fitstest.File(each.columns)
		catalogue, err := starhorse.OpenCatalogue(bytes.NewReader(file))
		if catalogue != nil || !errors.Is(err, celestialdata.Refused()) ||
			!strings.Contains(err.Error(), each.said) {
			test.Errorf("%v, which does not say %q", err, each.said)
		}
	}
	catalogue, err := starhorse.OpenCatalogue(strings.NewReader("no fits"))
	if catalogue != nil || !errors.Is(err, celestialdata.Refused()) ||
		!strings.Contains(err.Error(), "the StarHorse file") {
		test.Errorf("no fits: %v", err)
	}
}

// the anti-test: a row the file does not have, and a file cut short of
// its rows, are refused by Star and by Find, naming the row.
func TestARowTheFileCannotGiveIsRefused(test *testing.T) {
	catalogue := crafted(test, secondSource)
	if star, err := catalogue.Star(beyond); star != nil || err == nil {
		test.Errorf("row %d: %v", beyond, err)
	}
	whole := fitstest.File(layout(), craftedRow(firstSource),
		craftedRow(secondSource))
	short := whole[:len(whole)-blockBytes+rowWidth]
	cut, err := starhorse.OpenCatalogue(bytes.NewReader(short))
	if err != nil {
		test.Fatal(err)
	}
	star, err := cut.Star(onlyRow)
	if star != nil || !errors.Is(err, celestialdata.Refused()) ||
		!strings.Contains(err.Error(), "row 0") {
		test.Errorf("a short row: %v, %v", star, err)
	}
	if found, err := cut.Find(firstSource); found != nil ||
		!errors.Is(err, celestialdata.Refused()) {
		test.Errorf("a short file found %d: %v", len(found), err)
	}
}
