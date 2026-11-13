package astronn

import (
	"io"
	"math"

	"github.com/janearc/celestial-data/internal/rows"
	"github.com/janearc/celestial/fits"
)

// column is one of the file's columns the reader reads.
type column int

// The columns read, in the file's order.
const (
	apogeeID column = iota
	telescope
	distance
	distanceError
	weighted
	weightedError
	age
	ageLinear
	ageLowess
	ageError
	ageModelError
	gaiaID
	eccentricity
	highest
	pericentre
	apocentre
	angular
	energy
	allColumns
)

// The kinds the file writes, each defined by the standard's TFORM
// letters.
const (
	text   = 'A' // defined: characters
	long   = 'K' // defined: a 64-bit integer
	double = 'D' // defined: a 64-bit float
)

// fileName is how a refusal names the file.
const fileName = "astroNN file"

// held is a column as the file writes it: its name and its kind.
type held struct {
	name string
	kind byte
}

// named is each column as the file writes it, read 2026-11-06 from its
// header: the data model's names, but for those the file lower-cases.
var named = [allColumns]held{
	apogeeID:      {name: "APOGEE_ID", kind: text},
	telescope:     {name: "TELESCOPE", kind: text},
	distance:      {name: "dist", kind: double},
	distanceError: {name: "dist_error", kind: double},
	weighted:      {name: "weighted_dist", kind: double},
	weightedError: {name: "weighted_dist_error", kind: double},
	age:           {name: "age", kind: double},
	ageLinear:     {name: "age_linear_correct", kind: double},
	ageLowess:     {name: "age_lowess_correct", kind: double},
	ageError:      {name: "age_total_error", kind: double},
	ageModelError: {name: "age_model_error", kind: double},
	gaiaID:        {name: "source_id", kind: long},
	eccentricity:  {name: "e", kind: double},
	highest:       {name: "zmax", kind: double},
	pericentre:    {name: "rperi", kind: double},
	apocentre:     {name: "rap", kind: double},
	angular:       {name: "Lz", kind: double},
	energy:        {name: "Energy", kind: double},
}

// Catalogue is the astroNN file, read as its rows are asked for.
type Catalogue struct {
	table *fits.Table
	found [allColumns]fits.Column
}

// OpenCatalogue is the catalogue in an astroNN file. It refuses a file
// with no binary table, one that lacks a column read, and a column of
// another kind, of many values a row, or with a unit.
func OpenCatalogue(file io.ReaderAt) (*Catalogue, error) {
	table, err := fits.OpenTable(file)
	if err != nil {
		return nil, rows.Refusing(err, "the %s", fileName)
	}
	catalogue := &Catalogue{table: table}
	for which, want := range named {
		found, err := rows.Held(table, fileName, want.name, want.kind)
		if err != nil {
			return nil, err
		}
		catalogue.found[which] = *found
	}
	return catalogue, nil
}

// Rows is how many rows the file holds: one for each of allStar's, in
// allStar's order, so a star's row here is its row there.
func (catalogue *Catalogue) Rows() int64 { return catalogue.table.Rows() }

// Star is the star in a row, counted from none. It refuses a row the
// file does not have or cannot give, by its number. A number the
// networks left empty is not a number, and a source Gaia did not match
// is no source.
func (catalogue *Catalogue) Star(index int64) (*Star, error) {
	row, err := catalogue.table.Row(index)
	read := reading{row: row, found: catalogue.found, err: err}
	star := &Star{id: read.text(apogeeID), telescope: read.text(telescope),
		distance: uncertain{read.float(distance),
			read.float(distanceError)},
		weighted: uncertain{read.float(weighted),
			read.float(weightedError)},
		raw: read.float(age), linear: read.float(ageLinear),
		lowess: read.float(ageLowess), ageError: read.float(ageError),
		ageModel:   read.float(ageModelError),
		eccentric:  read.float(eccentricity),
		highest:    read.float(highest),
		pericentre: read.float(pericentre),
		apocentre:  read.float(apocentre),
		angular:    read.float(angular), energy: read.float(energy)}
	star.gaia, star.inGaia = read.maybe(gaiaID)
	if read.err != nil {
		return nil, rows.Refusing(read.err, "row %d", index)
	}
	return star, nil
}

// Find is every row whose star is one of the Gaia DR3 sources given,
// in the file's order. It reads every row, a pass over the whole file;
// a row with no source is never one asked. A caller who knows a star's
// row in allStar asks Star for that row instead.
func (catalogue *Catalogue) Find(sources ...int64) ([]*Star, error) {
	wanted := make(map[int64]bool, len(sources))
	for _, source := range sources {
		wanted[source] = true
	}
	var found []*Star
	for index := range catalogue.table.Rows() {
		star, err := catalogue.Star(index)
		if err != nil {
			return nil, err
		}
		if source, known := star.GaiaID(); known && wanted[source] {
			found = append(found, star)
		}
	}
	return found, nil
}

// reading is a row being read, column by column. It holds the first
// error, from the row or a cell, and reads nothing after it.
type reading struct {
	row   *fits.Row
	found [allColumns]fits.Column
	err   error
}

// maybe is a column's whole number and whether the file gave one;
// after an error, no number, and none given.
func (read *reading) maybe(which column) (int64, bool) {
	if read.err != nil {
		var none int64
		return none, false
	}
	value, known, err := read.row.Int(read.found[which])
	read.err = err
	return value, known
}

// float is a column's number.
func (read *reading) float(which column) float64 {
	if read.err != nil {
		return math.NaN()
	}
	value, err := read.row.Float(read.found[which])
	read.err = err
	return value
}

// text is a column's text.
func (read *reading) text(which column) string {
	if read.err != nil {
		return ""
	}
	value, err := read.row.Text(read.found[which])
	read.err = err
	return value
}
