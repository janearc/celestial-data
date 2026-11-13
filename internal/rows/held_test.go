package rows_test

import (
	"bytes"
	"errors"
	"strings"
	"testing"

	"github.com/janearc/celestial-data"
	"github.com/janearc/celestial-data/internal/fitstest"
	"github.com/janearc/celestial-data/internal/rows"
	"github.com/janearc/celestial/fits"
)

// The crafted file's names, each defined.
const (
	file   = "test file" // defined: how the refusals name the file
	asked  = "value"     // defined: the column asked for
	double = 'D'         // defined: TFORM's 64-bit float
)

// opened is a table of the one column given, opened, or the test stops.
func opened(test *testing.T, column fitstest.Column) *fits.Table {
	test.Helper()
	raw := fitstest.File([]fitstest.Column{column})
	table, err := fits.OpenTable(bytes.NewReader(raw))
	if err != nil {
		test.Fatal(err)
	}
	return table
}

// the test: a column of the name and kind asked, of one value a row and
// no unit, is held and handed back.
func TestAColumnAsAskedIsHeld(test *testing.T) {
	table := opened(test, fitstest.NewColumn(asked, "D"))
	found, err := rows.Held(table, file, asked, double)
	if err != nil || found.Name() != asked || found.Kind() != double {
		test.Errorf("%v, %v", found, err)
	}
}

// heldWrong is a column made otherwise and the words its refusal says.
type heldWrong struct {
	column fitstest.Column
	said   string
}

// the anti-test: a column the table lacks, one of another kind, one of
// many values a row and one with a unit are each refused, naming the
// file, and nothing is handed back.
func TestAColumnNotAsAskedIsRefused(test *testing.T) {
	for _, each := range []heldWrong{
		{fitstest.NewColumn("other", "D"), "the test file has no value"},
		{fitstest.NewColumn(asked, "E"), "value is of kind E, not D"},
		{fitstest.NewColumn(asked, "2D"), "value holds 2 values a row, not one"},
		{fitstest.NewColumn(asked, "D").In("Gyr"),
			`value is in "Gyr", where the test file writes no unit`}} {
		table := opened(test, each.column)
		found, err := rows.Held(table, file, asked, double)
		if found != nil || !errors.Is(err, celestialdata.Refused()) ||
			!strings.Contains(err.Error(), each.said) {
			test.Errorf("%v, which does not say %q", err, each.said)
		}
	}
}
