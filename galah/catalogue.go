package galah

import (
	"fmt"
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
	sobjectID column = iota
	twoMASS
	gaiaID
	ra
	dec
	spectroscopic
	snr
	radial
	radialError
	teff
	teffError
	logg
	loggError
	iron
	ironError
	ironFlag
	vsini
	vsiniError
	lithium
	lithiumLimit
	lithiumBelow
	lithiumAbove
	lithiumFlag
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

// oneValue is a column read holding one value a row. It's defined.
const oneValue = 1

// held is a column as the file writes it: its name and its kind. No
// column read has a unit in the file.
type held struct {
	name string
	kind byte
}

// named is each column as the file writes it, read 2026-11-06 from its
// header.
var named = [allColumns]held{
	sobjectID:     {name: "sobject_id", kind: long},
	twoMASS:       {name: "tmass_id", kind: text},
	gaiaID:        {name: "gaiadr3_source_id", kind: long},
	ra:            {name: "ra", kind: double},
	dec:           {name: "dec", kind: double},
	spectroscopic: {name: "flag_sp", kind: whole},
	snr:           {name: "snr_px_ccd3", kind: single},
	radial:        {name: "rv_comp_1", kind: single},
	radialError:   {name: "e_rv_comp_1", kind: single},
	teff:          {name: "teff", kind: single},
	teffError:     {name: "e_teff", kind: single},
	logg:          {name: "logg", kind: single},
	loggError:     {name: "e_logg", kind: single},
	iron:          {name: "fe_h", kind: single},
	ironError:     {name: "e_fe_h", kind: single},
	ironFlag:      {name: "flag_fe_h", kind: whole},
	vsini:         {name: "vsini", kind: single},
	vsiniError:    {name: "e_vsini", kind: single},
	lithium:       {name: "a_li", kind: single},
	lithiumLimit:  {name: "a_li_upp_lim", kind: single},
	lithiumBelow:  {name: "e_a_li_low", kind: single},
	lithiumAbove:  {name: "e_a_li_upp", kind: single},
	lithiumFlag:   {name: "flag_a_li", kind: long},
}

// elements are the elements whose abundance against iron the file
// holds, by their symbols, in the file's order. Lithium is apart, as
// an absolute abundance.
var elements = [...]string{"C", "N", "O", "Na", "Mg", "Al", "Si", "K",
	"Ca", "Sc", "Ti", "V", "Cr", "Mn", "Co", "Ni", "Cu", "Zn", "Rb", "Sr",
	"Y", "Zr", "Mo", "Ru", "Ba", "La", "Ce", "Nd", "Sm", "Eu"}

// Each element's three columns are its symbol lower-cased between
// these.
const (
	againstIron  = "_fe"   // defined: [X/Fe], single, after the symbol
	itsError     = "e_"    // defined: its uncertainty, single, before it
	itsFlag      = "flag_" // defined: its flag's bits, whole, before it
	elementCount = len(elements)
)

// Elements are the symbols of the elements the file holds abundances
// of against iron, in its order, as Star.Abundance takes them.
func Elements() []string { return slices.Clone(elements[:]) }

// trio is an element's three columns.
type trio struct{ value, uncertainty, flag fits.Column }

// Catalogue is the allstar file on disk, read as its rows are asked
// for.
type Catalogue struct {
	table      *fits.Table
	found      [allColumns]fits.Column
	abundances [elementCount]trio
}

// OpenCatalogue is the catalogue in an allstar file. It refuses a file
// with no binary table, one that lacks a column read, and a column of
// another kind, of many values a row, or with a unit.
func OpenCatalogue(file io.ReaderAt) (*Catalogue, error) {
	table, err := fits.OpenTable(file)
	if err != nil {
		return nil, rows.Refusing(err, "the allstar file")
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
		return nil, celestialdata.Refuse("the allstar file has no %s",
			want.name)
	case found.Kind() != want.kind:
		return nil, celestialdata.Refuse("%s is of kind %c, not %c",
			want.name, found.Kind(), want.kind)
	case found.Count() != oneValue:
		return nil, celestialdata.Refuse("%s holds %d values a row, not one",
			want.name, found.Count())
	case found.Unit() != "":
		return nil, celestialdata.Refuse(
			"%s is in %q, where the allstar file writes no unit", want.name,
			found.Unit())
	}
	return found, nil
}

// trioOf is an element's three columns, by its symbol, each held as
// heldTo holds a column.
func trioOf(table *fits.Table, symbol string) (*trio, error) {
	stem := strings.ToLower(symbol) + againstIron
	value, err := heldTo(table, held{name: stem, kind: single})
	if err != nil {
		return nil, err
	}
	uncertainty, err := heldTo(table, held{name: itsError + stem,
		kind: single})
	if err != nil {
		return nil, err
	}
	flag, err := heldTo(table, held{name: itsFlag + stem, kind: whole})
	if err != nil {
		return nil, err
	}
	return &trio{value: *value, uncertainty: *uncertainty, flag: *flag}, nil
}

// Rows is how many rows the file holds, a row a star.
func (catalogue *Catalogue) Rows() int64 { return catalogue.table.Rows() }

// Star is the star in a row, counted from none. It refuses a row the
// file does not have or cannot give, one whose survey id or flag was
// left empty, and one whose place is not on the sky.
//
// A row whose place was left empty has none, and a number left empty is
// not a number.
func (catalogue *Catalogue) Star(index int64) (*Star, error) {
	row, err := catalogue.table.Row(index)
	if err != nil {
		return nil, err
	}
	read := reading{row: row, found: catalogue.found}
	star := &Star{sobject: read.whole(sobjectID), twoMASS: read.text(twoMASS),
		snr:    read.float(snr),
		radial: read.float(radial), radialError: read.float(radialError),
		teff: read.float(teff), teffError: read.float(teffError),
		logg: read.float(logg), loggError: read.float(loggError),
		vsini: read.float(vsini), vsiniError: read.float(vsiniError),
		spectroscopic: Flags(read.whole(spectroscopic)),
		iron: read.abundance(trio{value: catalogue.found[iron],
			uncertainty: catalogue.found[ironError],
			flag:        catalogue.found[ironFlag]}),
		lithium: Lithium{value: read.float(lithium), limit: read.float(lithiumLimit),
			below: read.float(lithiumBelow), above: read.float(lithiumAbove),
		}}
	star.gaia, star.inGaia = read.maybe(gaiaID)
	flag, flagged := read.maybe(lithiumFlag)
	star.lithium.flags, star.lithium.flagged = Flags(flag), flagged
	for at, columns := range catalogue.abundances {
		star.abundances[at] = read.abundance(columns)
	}
	ascension, declination := read.float(ra), read.float(dec)
	if read.err != nil {
		return nil, fmt.Errorf("row %d: %w", index, read.err)
	}
	star.place, err = rows.Placed(ascension, declination)
	if err != nil {
		return nil, rows.Refusing(err, "%d", star.sobject)
	}
	return star, nil
}

// Find is every row whose star is one of the Gaia DR3 sources given,
// in the file's order. It reads every row's source, a pass over the
// whole file; a row with no source is never one asked.
func (catalogue *Catalogue) Find(sources ...int64) ([]*Star, error) {
	wanted := make(map[int64]bool, len(sources))
	for _, source := range sources {
		wanted[source] = true
	}
	var found []*Star
	for index := range catalogue.table.Rows() {
		row, rowErr := catalogue.table.Row(index)
		if rowErr != nil {
			return nil, rowErr
		}
		source, known, err := row.Int(catalogue.found[gaiaID])
		if err != nil {
			return nil, err
		}
		if !known || !wanted[source] {
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

// whole is a column's whole number, which the survey must have given.
func (read *reading) whole(which column) int64 {
	return read.integer(read.found[which])
}

// maybe is a column's whole number and whether the survey gave one.
func (read *reading) maybe(which column) (int64, bool) {
	value, known, err := read.row.Int(read.found[which])
	read.keep(err)
	return value, known
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
// One the survey left empty is refused, by its column.
func (read *reading) integer(column fits.Column) int64 {
	value, known, err := read.row.Int(column)
	read.keep(err)
	if !known {
		name := column.Name()
		read.keep(celestialdata.Refuse("the survey left %s empty", name))
	}
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
