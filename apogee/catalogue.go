package apogee

import (
	"io"
	"slices"
	"strings"

	"github.com/janearc/celestial-data"
	"github.com/janearc/celestial-data/internal/rows"
	"github.com/janearc/celestial/fits"
)

// column is one of the file's columns the reader reads, apart from the
// elements'.
type column int

// The columns read, in the file's order.
const (
	apogeeID column = iota
	telescope
	field
	ra
	dec
	snr
	starFlag
	radial
	scatter
	radialError
	gaiaID
	aspcapFlag
	extraTarget
	teff
	teffError
	logg
	loggError
	metals
	metalsError
	alpha
	alphaError
	iron
	ironError
	ironFlag
	allColumns
)

// The kinds the file writes, each defined by the standard's TFORM
// letters.
const (
	text   = 'A' // defined: characters
	long   = 'K' // defined: a 64-bit integer
	whole  = 'J' // defined: a 32-bit integer
	double = 'D' // defined: a 64-bit float
	single = 'E' // defined: a 32-bit float
)

// The numbers the reader reads by.
const (
	oneValue = 1 // defined: a column read holds one value a row
)

// held is a column as the file writes it: its name and its kind. The
// file writes no units.
type held struct {
	name string
	kind byte
}

// named is each column as the file writes it, read 2026-11-06 from its
// header.
var named = [allColumns]held{
	apogeeID:    {name: "APOGEE_ID", kind: text},
	telescope:   {name: "TELESCOPE", kind: text},
	field:       {name: "FIELD", kind: text},
	ra:          {name: "RA", kind: double},
	dec:         {name: "DEC", kind: double},
	snr:         {name: "SNR", kind: single},
	starFlag:    {name: "STARFLAG", kind: long},
	radial:      {name: "VHELIO_AVG", kind: single},
	scatter:     {name: "VSCATTER", kind: single},
	radialError: {name: "VERR", kind: single},
	gaiaID:      {name: "GAIAEDR3_SOURCE_ID", kind: long},
	aspcapFlag:  {name: "ASPCAPFLAG", kind: long},
	extraTarget: {name: "EXTRATARG", kind: whole},
	teff:        {name: "TEFF", kind: single},
	teffError:   {name: "TEFF_ERR", kind: single},
	logg:        {name: "LOGG", kind: single},
	loggError:   {name: "LOGG_ERR", kind: single},
	metals:      {name: "M_H", kind: single},
	metalsError: {name: "M_H_ERR", kind: single},
	alpha:       {name: "ALPHA_M", kind: single},
	alphaError:  {name: "ALPHA_M_ERR", kind: single},
	iron:        {name: "FE_H", kind: single},
	ironError:   {name: "FE_H_ERR", kind: single},
	ironFlag:    {name: "FE_H_FLAG", kind: whole},
}

// elements are the elements whose abundance against iron the file
// holds, by their symbols, in the file's order. CI is carbon from its
// atomic lines and TiII titanium from its ion's. Yb is as the file names
// its column, YB_FE; the data model calls that column [Y/Fe].
var elements = [...]string{"C", "CI", "N", "O", "Na", "Mg", "Al", "Si",
	"P", "S", "K", "Ca", "Ti", "TiII", "V", "Cr", "Mn", "Co", "Ni", "Cu",
	"Ce", "Yb"}

// Each element's three columns are its symbol upper-cased and these.
const (
	againstIron  = "_FE"      // defined: [X/Fe], single
	itsError     = "_FE_ERR"  // defined: its uncertainty, single
	itsFlag      = "_FE_FLAG" // defined: its ELEMFLAG bits, whole
	elementCount = len(elements)
)

// Elements are the symbols of the elements the file holds abundances
// of, in its order, as Star.Abundance takes them.
func Elements() []string { return slices.Clone(elements[:]) }

// trio is an element's three columns.
type trio struct{ value, uncertainty, flag fits.Column }

// Catalogue is the allStar file on disk, read as its rows are asked
// for.
type Catalogue struct {
	table      *fits.Table
	found      [allColumns]fits.Column
	abundances [elementCount]trio
}

// OpenCatalogue is the catalogue in an allStar file. It refuses a file
// with no binary table, one that lacks a column read, and a column of
// another kind, of many values a row, or with a unit.
func OpenCatalogue(file io.ReaderAt) (*Catalogue, error) {
	table, err := fits.OpenTable(file)
	if err != nil {
		return nil, rows.Refusing(err, "the allStar file")
	}
	catalogue := &Catalogue{table: table}
	for which, want := range named {
		found, err := heldTo(table, want)
		if err != nil {
			return nil, err
		}
		catalogue.found[which] = *found
	}
	for at, symbol := range elements {
		columns, err := trioOf(table, symbol)
		if err != nil {
			return nil, err
		}
		catalogue.abundances[at] = *columns
	}
	return catalogue, nil
}

// heldTo is a column of the table held to its name and kind, to one
// value a row and to no unit. It refuses a column the table lacks, and
// one of another kind, of many values or with a unit.
func heldTo(table *fits.Table, want held) (*fits.Column, error) {
	found, err := table.Column(want.name)
	switch {
	case err != nil:
		return nil, celestialdata.Refuse("the allStar file has no %s",
			want.name)
	case found.Kind() != want.kind:
		return nil, celestialdata.Refuse("%s is of kind %c, not %c",
			want.name, found.Kind(), want.kind)
	case found.Count() != oneValue:
		return nil, celestialdata.Refuse("%s holds %d values a row, not one",
			want.name, found.Count())
	case found.Unit() != "":
		return nil, celestialdata.Refuse(
			"%s is in %q, where the allStar file writes no unit", want.name,
			found.Unit())
	}
	return found, nil
}

// trioOf is an element's three columns, by its symbol, each held as
// heldTo holds a column.
func trioOf(table *fits.Table, symbol string) (*trio, error) {
	stem := strings.ToUpper(symbol)
	value, err := heldTo(table, held{name: stem + againstIron, kind: single})
	if err != nil {
		return nil, err
	}
	uncertainty, err := heldTo(table, held{name: stem + itsError,
		kind: single})
	if err != nil {
		return nil, err
	}
	flag, err := heldTo(table, held{name: stem + itsFlag, kind: whole})
	if err != nil {
		return nil, err
	}
	return &trio{value: *value, uncertainty: *uncertainty, flag: *flag}, nil
}

// Rows is how many rows the file holds: a row a star, but for the
// stars observed from both of the survey's telescopes, or in more than
// one field, which have a row each.
func (catalogue *Catalogue) Rows() int64 { return catalogue.table.Rows() }

// Star is the star in a row, counted from none. It refuses a row the
// file does not have or cannot give, and one whose place is not on the
// sky, but for a row whose place the survey left empty, which has none.
// A value the survey left empty is not a number.
func (catalogue *Catalogue) Star(index int64) (*Star, error) {
	row, err := catalogue.table.Row(index)
	if err != nil {
		return nil, err
	}
	read := reading{row: row, found: catalogue.found}
	star := &Star{gaia: read.whole(gaiaID), id: read.text(apogeeID),
		telescope: read.text(telescope), field: read.text(field),
		snr: read.float(snr), radial: read.float(radial),
		scatter: read.float(scatter), radialError: read.float(radialError),
		teff: read.float(teff), teffError: read.float(teffError),
		logg: read.float(logg), loggError: read.float(loggError),
		metals: read.float(metals), metalsError: read.float(metalsError),
		alpha: read.float(alpha), alphaError: read.float(alphaError),
		iron: read.abundance(trio{value: catalogue.found[iron],
			uncertainty: catalogue.found[ironError],
			flag:        catalogue.found[ironFlag]}),
		aspcap:     Flags(read.whole(aspcapFlag)),
		conditions: Flags(read.whole(starFlag)),
		targeting:  Flags(read.whole(extraTarget))}
	for at, columns := range catalogue.abundances {
		star.abundances[at] = read.abundance(columns)
	}
	ascension, declination := read.float(ra), read.float(dec)
	if read.err != nil {
		return nil, read.err
	}
	star.place, err = rows.Placed(ascension, declination)
	if err != nil {
		return nil, rows.Refusing(err, "%s", star.id)
	}
	return star, nil
}

// Find is every row whose star is one of the Gaia DR3 sources given,
// in the file's order, a star's duplicates among them. It reads every
// row's source, a pass over the whole file.
func (catalogue *Catalogue) Find(sources ...int64) ([]*Star, error) {
	wanted := make(map[int64]bool, len(sources))
	for _, source := range sources {
		wanted[source] = true
	}
	var found []*Star
	for index := range catalogue.table.Rows() {
		// Row refuses only an index outside the table, and index runs
		// over the table's own rows, so it refuses none here.
		row, _ := catalogue.table.Row(index)
		source, _, err := row.Int(catalogue.found[gaiaID])
		if err != nil {
			return nil, err
		}
		if !wanted[source] {
			continue
		}
		star, err := catalogue.Star(index)
		if err != nil {
			return nil, err
		}
		found = append(found, star)
	}
	return found, nil
}

// reading is a row being read, column by column, keeping its first
// error: a row is read whole from the file, so its first read is the
// one that can fail.
type reading struct {
	row   *fits.Row
	found [allColumns]fits.Column
	err   error
}

// keep keeps an error when it is the row's first.
func (read *reading) keep(err error) {
	if read.err == nil {
		read.err = err
	}
}

// whole is a column's whole number.
func (read *reading) whole(which column) int64 {
	return read.integer(read.found[which])
}

// float is a column's number.
func (read *reading) float(which column) float64 {
	return read.number(read.found[which])
}

// text is a column's text.
func (read *reading) text(which column) string {
	value, err := read.row.Text(read.found[which])
	read.keep(err)
	return value
}

// integer is a column's whole number, read whole so no digit is lost.
func (read *reading) integer(column fits.Column) int64 {
	value, _, err := read.row.Int(column)
	read.keep(err)
	return value
}

// number is a column's number.
func (read *reading) number(column fits.Column) float64 {
	value, err := read.row.Float(column)
	read.keep(err)
	return value
}

// abundance is an element's three columns, read.
func (read *reading) abundance(columns trio) Abundance {
	value := read.number(columns.value)
	uncertainty := read.number(columns.uncertainty)
	flags := Flags(read.integer(columns.flag))
	return Abundance{value: value, uncertainty: uncertainty, flags: flags}
}
