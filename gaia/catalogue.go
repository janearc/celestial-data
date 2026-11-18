package gaia

import (
	"io"
	"math"

	"github.com/janearc/celestial-data"
	"github.com/janearc/celestial-data/internal/rows"
	"github.com/janearc/celestial/fits"
	"github.com/janearc/celestial/galactocentric"
	"github.com/janearc/celestial/measure"
	"github.com/janearc/celestial/sky"
)

// column is one of the query's columns, in the archive's order.
type column int

// The query's columns, in the order the archive answers them.
const (
	sourceID column = iota
	refEpoch
	ra
	raError
	dec
	decError
	parallax
	parallaxError
	pmRA
	pmRAError
	pmDec
	pmDecError
	radialVelocity
	radialVelocityError
	raDecCorr
	raParallaxCorr
	raPMRACorr
	raPMDecCorr
	decParallaxCorr
	decPMRACorr
	decPMDecCorr
	parallaxPMRACorr
	parallaxPMDecCorr
	pmRAPMDecCorr
	ruwe
	gMagnitude
	bpRP
	nuEff
	pseudocolour
	eclipticLatitude
	paramsSolved
	allColumns // defined: how many columns the query has
)

// archived is a column's name, kind and unit as the archive writes
// them: the kind is its TFORM's letter.
type archived struct {
	name, unit string
	kind       byte
}

// The kinds the archive writes, each defined by the standard's TFORM
// letters.
const (
	long   = 'K' // defined: a 64-bit integer
	double = 'D' // defined: a 64-bit float
	single = 'E' // defined: a 32-bit float
	short  = 'I' // defined: a 16-bit integer
)

// named is each column as the archive writes it, read 2026-11-05 from
// the product's header; no unit where the archive writes none.
var named = [allColumns]archived{
	sourceID:      {name: "source_id", kind: long},
	refEpoch:      {name: "ref_epoch", unit: "yr", kind: double},
	ra:            {name: "ra", unit: "deg", kind: double},
	raError:       {name: "ra_error", unit: "mas", kind: single},
	dec:           {name: "dec", unit: "deg", kind: double},
	decError:      {name: "dec_error", unit: "mas", kind: single},
	parallax:      {name: "parallax", unit: "mas", kind: double},
	parallaxError: {name: "parallax_error", unit: "mas", kind: single},
	pmRA:          {name: "pmra", unit: "mas.yr**-1", kind: double},
	pmRAError:     {name: "pmra_error", unit: "mas.yr**-1", kind: single},
	pmDec:         {name: "pmdec", unit: "mas.yr**-1", kind: double},
	pmDecError:    {name: "pmdec_error", unit: "mas.yr**-1", kind: single},
	radialVelocity: {name: "radial_velocity", unit: "km.s**-1",
		kind: single},
	radialVelocityError: {name: "radial_velocity_error", unit: "km.s**-1",
		kind: single},
	raDecCorr:         {name: "ra_dec_corr", kind: single},
	raParallaxCorr:    {name: "ra_parallax_corr", kind: single},
	raPMRACorr:        {name: "ra_pmra_corr", kind: single},
	raPMDecCorr:       {name: "ra_pmdec_corr", kind: single},
	decParallaxCorr:   {name: "dec_parallax_corr", kind: single},
	decPMRACorr:       {name: "dec_pmra_corr", kind: single},
	decPMDecCorr:      {name: "dec_pmdec_corr", kind: single},
	parallaxPMRACorr:  {name: "parallax_pmra_corr", kind: single},
	parallaxPMDecCorr: {name: "parallax_pmdec_corr", kind: single},
	pmRAPMDecCorr:     {name: "pmra_pmdec_corr", kind: single},
	ruwe:              {name: "ruwe", kind: single},
	gMagnitude:        {name: "phot_g_mean_mag", unit: "mag", kind: single},
	bpRP:              {name: "bp_rp", unit: "mag", kind: single},
	nuEff: {name: "nu_eff_used_in_astrometry", unit: "um**-1",
		kind: single},
	pseudocolour:     {name: "pseudocolour", unit: "um**-1", kind: single},
	eclipticLatitude: {name: "ecl_lat", unit: "deg", kind: double},
	paramsSolved:     {name: "astrometric_params_solved", kind: short},
}

// The reader's own numbers, each defined.
const (
	masInArcsecond = 1000 // defined: a thousand milliarcseconds in one
	itself         = 1.0  // defined: a parameter's correlation with itself
	noInteger      = 0    // defined: the whole number given after an error
)

// Catalogue is what the archive's job answered, on disk: gaia_source
// rows in a FITS binary table, read as they are asked for.
//
// Its columns are the query's, found by name; each is held to the
// archive's kind and unit, since a column of another kind or in another
// unit would be another query, or another archive's writing of this one.
type Catalogue struct {
	table *fits.Table
	found [allColumns]fits.Column
}

// OpenCatalogue is the catalogue in a product file. It refuses a file
// with no binary table, one that lacks a column of the query, and a
// column of another kind or in another unit than the archive writes.
func OpenCatalogue(file io.ReaderAt) (*Catalogue, error) {
	table, err := fits.OpenTable(file)
	if err != nil {
		return nil, rows.Refusing(err, "the product")
	}
	catalogue := &Catalogue{table: table}
	for which, want := range named {
		found, err := table.Column(want.name)
		if err != nil {
			return nil, celestialdata.Refuse(
				"the product has no %s",
				want.name)
		}
		if found.Kind() != want.kind {
			return nil, celestialdata.Refuse(
				"%s is of kind %c, not %c",
				want.name, found.Kind(), want.kind)
		}
		if found.Unit() != want.unit {
			return nil, celestialdata.Refuse("%s is in %q, not %q",
				want.name,
				found.Unit(), want.unit)
		}
		catalogue.found[which] = *found
	}
	return catalogue, nil
}

// Rows is how many sources the catalogue holds.
func (catalogue *Catalogue) Rows() int64 { return catalogue.table.Rows() }

// Source is the source in a row, counted from none. It refuses a row
// the catalogue does not have, one the file cannot give, and one whose
// place is not on the sky. A value the archive left empty is not a
// number.
func (catalogue *Catalogue) Source(index int64) (*Source, error) {
	row, err := catalogue.table.Row(index)
	if err != nil {
		return nil, err
	}
	read := reading{row: row, found: catalogue.found}
	id := read.whole(sourceID)
	values := read.floats()
	solved := read.whole(paramsSolved)
	if read.err != nil {
		return nil, read.err
	}
	place, err := rows.Place(values[ra], values[dec])
	if err != nil {
		return nil, rows.Refusing(err, "source %d", id)
	}
	return &Source{id: id, epoch: values[refEpoch], place: *place,
		raError: mas(values[raError]), decError: mas(values[decError]),
		parallax:      mas(values[parallax]),
		parallaxError: mas(values[parallaxError]),
		motion: galactocentric.NewProperMotion(values[pmRA],
			values[pmDec]),
		pmRAError:        values[pmRAError],
		pmDecError:       values[pmDecError],
		radial:           values[radialVelocity],
		radialError:      values[radialVelocityError],
		correlations:     correlated(values),
		ruwe:             values[ruwe],
		magnitude:        values[gMagnitude],
		colour:           values[bpRP],
		nuEff:            values[nuEff],
		pseudocolour:     values[pseudocolour],
		eclipticLatitude: measure.Degrees(values[eclipticLatitude]),
		solved:           int(solved)}, nil
}

// reading is a row being read, column by column, keeping the first
// error: the rows are read whole from the file, so the first read of a
// row is the one that can fail, and the reads after it are spared.
type reading struct {
	row   *fits.Row
	found [allColumns]fits.Column
	err   error
}

// whole is a column's whole number, read whole so that no digit is
// lost, or nothing after an error.
func (read *reading) whole(which column) int64 {
	if read.err != nil {
		return noInteger
	}
	value, _, err := read.row.Int(read.found[which])
	read.err = err
	return value
}

// floats are the row's numbers, each column's at its place, but the two
// whole-number columns, which are read apart; nothing after an error.
func (read *reading) floats() [allColumns]float64 {
	var values [allColumns]float64
	for which, found := range read.found {
		if column(which) == sourceID || column(which) == paramsSolved ||
			read.err != nil {
			continue
		}
		values[which], read.err = read.row.Float(found)
	}
	return values
}

// mas is an angle of so many milliarcseconds.
func mas(amount float64) measure.Angle {
	return measure.Arcseconds(amount / masInArcsecond)
}

// Parameter is one of the five astrometric parameters the archive
// solves together.
type Parameter int

// The five astrometric parameters, in the archive's order.
const (
	RA Parameter = iota
	Dec
	Parallax
	PMRA
	PMDec
	parameters // defined: how many there are
)

// Correlations is the correlation between each pair of the five
// astrometric parameters, as the archive gives them: one for a
// parameter with itself.
type Correlations struct {
	between [parameters][parameters]float64
}

// Between is the correlation between two parameters. It refuses a
// parameter that is not one of the five, with not a number beside the
// error.
func (corr Correlations) Between(one, other Parameter) (float64, error) {
	for _, each := range []Parameter{one, other} {
		if each < RA || each >= parameters {
			return math.NaN(), celestialdata.Refuse(
				"no parameter %d",
				each)
		}
	}
	return corr.between[one][other], nil
}

// pair is two parameters and the column that holds their correlation.
type pair struct {
	one, other Parameter
	at         column
}

// pairs are the ten pairs of parameters the archive correlates.
var pairs = []pair{
	{one: RA, other: Dec, at: raDecCorr},
	{one: RA, other: Parallax, at: raParallaxCorr},
	{one: RA, other: PMRA, at: raPMRACorr},
	{one: RA, other: PMDec, at: raPMDecCorr},
	{one: Dec, other: Parallax, at: decParallaxCorr},
	{one: Dec, other: PMRA, at: decPMRACorr},
	{one: Dec, other: PMDec, at: decPMDecCorr},
	{one: Parallax, other: PMRA, at: parallaxPMRACorr},
	{one: Parallax, other: PMDec, at: parallaxPMDecCorr},
	{one: PMRA, other: PMDec, at: pmRAPMDecCorr},
}

// correlated is a row's correlations, both ways round, with one for
// each parameter with itself.
func correlated(values [allColumns]float64) Correlations {
	var corr Correlations
	for _, each := range pairs {
		corr.between[each.one][each.other] = values[each.at]
		corr.between[each.other][each.one] = values[each.at]
	}
	for each := range parameters {
		corr.between[each][each] = itself
	}
	return corr
}

// Source is one star as gaia_source has it: where it is on the sky at
// the catalogue's epoch, how far it is by its parallax, how it moves
// across the sky and along the line of sight, each with its error, and
// the correlations between its five astrometric parameters.
//
// With them its RUWE, its G magnitude and BP minus RP colour, the
// effective wavenumber its astrometry used and its pseudocolour, its
// ecliptic latitude, and which parameters were solved.
//
// The id and the parameters solved are whole numbers read whole; the
// archive writes no TNULL on them, so neither is ever null, and a null
// written one day would read as the number it is.
type Source struct {
	id                      int64
	epoch                   float64
	place                   sky.Equatorial
	raError, decError       measure.Angle
	parallax, parallaxError measure.Angle
	motion                  galactocentric.ProperMotion
	pmRAError, pmDecError   float64
	radial, radialError     float64
	correlations            Correlations
	ruwe, magnitude, colour float64
	nuEff, pseudocolour     float64
	eclipticLatitude        measure.Angle
	solved                  int
}

// ID is the source's identifier in the archive, its source_id.
func (source Source) ID() int64 { return source.id }

// Epoch is the Julian year the source's place is given at.
func (source Source) Epoch() float64 { return source.epoch }

// Place is where the source is on the sky at the epoch.
func (source Source) Place() sky.Equatorial { return source.place }

// PlaceError is the error of the place, along ascension and along
// declination.
func (source Source) PlaceError() (ascension, declination measure.Angle) {
	return source.raError, source.decError
}

// Parallax is the source's parallax, which may be negative as the
// archive measures it.
func (source Source) Parallax() measure.Angle { return source.parallax }

// ParallaxError is the error of the parallax.
func (source Source) ParallaxError() measure.Angle {
	return source.parallaxError
}

// Motion is the source's proper motion.
func (source Source) Motion() galactocentric.ProperMotion {
	return source.motion
}

// MotionError is the error of the proper motion along ascension and
// along declination, in milliarcseconds a year.
func (source Source) MotionError() (ascension, declination float64) {
	return source.pmRAError, source.pmDecError
}

// RadialVelocity is the source's speed along the line of sight in
// kilometres per second, away from the Sun.
func (source Source) RadialVelocity() float64 { return source.radial }

// RadialVelocityError is the error of the radial velocity in
// kilometres per second.
func (source Source) RadialVelocityError() float64 {
	return source.radialError
}

// Correlations are the correlations between the source's five
// astrometric parameters.
func (source Source) Correlations() Correlations {
	return source.correlations
}

// RUWE is the source's renormalised unit weight error, the archive's
// measure of how well one star's motion fits its observations.
func (source Source) RUWE() float64 { return source.ruwe }

// Magnitude is the source's mean G magnitude.
func (source Source) Magnitude() float64 { return source.magnitude }

// Colour is the source's BP minus RP colour, in magnitudes.
func (source Source) Colour() float64 { return source.colour }

// EffectiveWavenumber is the one the source's astrometry used, in
// inverse micrometres.
func (source Source) EffectiveWavenumber() float64 { return source.nuEff }

// Pseudocolour is the source's astrometrically fitted pseudocolour, in
// inverse micrometres.
func (source Source) Pseudocolour() float64 { return source.pseudocolour }

// EclipticLatitude is the source's latitude on the ecliptic.
func (source Source) EclipticLatitude() measure.Angle {
	return source.eclipticLatitude
}

// ParametersSolved is which astrometric parameters the archive solved
// for the source, as its astrometric_params_solved has it: 3 for a
// place alone, 31 for all five, 95 with the pseudocolour.
func (source Source) ParametersSolved() int { return source.solved }
