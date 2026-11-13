package starhorse

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
	gaiaID
	mass16
	mass50
	mass84
	teff50
	logg50
	met50
	dist16
	dist50
	dist84
	extinction16
	extinction50
	extinction84
	age05
	age16
	age50
	age84
	age95
	outputFlags
	ageFlag
	ageWarning
	allColumns
)

// The kinds the file writes, each defined by the standard's TFORM
// letters.
const (
	text   = 'A' // defined: characters
	long   = 'K' // defined: a 64-bit integer
	single = 'E' // defined: a 32-bit float
)

// fileName is how a refusal names the file.
const fileName = "StarHorse file"

// held is a column as the file writes it: its name and its kind.
type held struct {
	name string
	kind byte
}

// named is each column as the file writes it, read 2026-11-06 from its
// header. Its flags are the header's names, which its page gives without
// their final S.
var named = [allColumns]held{
	apogeeID:     {name: "APOGEE_ID", kind: text},
	gaiaID:       {name: "DR3_source_id", kind: long},
	mass16:       {name: "mass16", kind: single},
	mass50:       {name: "mass50", kind: single},
	mass84:       {name: "mass84", kind: single},
	teff50:       {name: "teff50", kind: single},
	logg50:       {name: "logg50", kind: single},
	met50:        {name: "met50", kind: single},
	dist16:       {name: "dist16", kind: single},
	dist50:       {name: "dist50", kind: single},
	dist84:       {name: "dist84", kind: single},
	extinction16: {name: "AV16", kind: single},
	extinction50: {name: "AV50", kind: single},
	extinction84: {name: "AV84", kind: single},
	age05:        {name: "age05", kind: single},
	age16:        {name: "age16", kind: single},
	age50:        {name: "age50", kind: single},
	age84:        {name: "age84", kind: single},
	age95:        {name: "age95", kind: single},
	outputFlags:  {name: "StarHorse_OUTPUTFLAGS", kind: text},
	ageFlag:      {name: "StarHorse_AGEFLAG", kind: text},
	ageWarning:   {name: "StarHorse_AGE_INOUT", kind: text},
}

// Catalogue is the StarHorse file, read as its rows are asked for.
type Catalogue struct {
	table *fits.Table
	found [allColumns]fits.Column
}

// OpenCatalogue is the catalogue in a StarHorse file. It refuses a file
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

// Rows is how many rows the file holds, a row a star StarHorse fit: its
// own rows, not allStar's.
func (catalogue *Catalogue) Rows() int64 { return catalogue.table.Rows() }

// Star is the star in a row, counted from none. It refuses a row the
// file does not have or cannot give, by its number. A number StarHorse
// left empty is not a number, an age among them for a NOAGE star.
func (catalogue *Catalogue) Star(index int64) (*Star, error) {
	row, err := catalogue.table.Row(index)
	read := reading{row: row, found: catalogue.found, err: err}
	star := &Star{id: read.text(apogeeID),
		mass: read.spread(mass16, mass50, mass84),
		teff: read.float(teff50), logg: read.float(logg50),
		metals:   read.float(met50),
		distance: read.spread(dist16, dist50, dist84),
		extinction: read.spread(extinction16, extinction50,
			extinction84),
		age:    read.spread(age16, age50, age84),
		ageLow: read.float(age05), ageHigh: read.float(age95),
		output: read.text(outputFlags), ageFlag: read.text(ageFlag),
		ageWarning: read.text(ageWarning)}
	star.gaia, star.inGaia = read.maybe(gaiaID)
	if read.err != nil {
		return nil, rows.Refusing(read.err, "row %d", index)
	}
	return star, nil
}

// Find is every row whose star is one of the Gaia DR3 sources given,
// in the file's order. It reads every row, a pass over the whole file;
// a row with no source is never one asked.
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

// maybe is a column's whole number and whether the file gave one; after
// an error, no number, and none given.
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

// spread is three columns' percentiles, the 16th, 50th and 84th.
func (read *reading) spread(low, median, high column) Spread {
	return Spread{low: read.float(low), median: read.float(median),
		high: read.float(high)}
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
