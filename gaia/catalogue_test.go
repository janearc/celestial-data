package gaia_test

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"math"
	"strings"
	"testing"

	"github.com/janearc/celestial-data"
	"github.com/janearc/celestial-data/gaia"
)

// The FITS numbers the product tests write by, each defined by the
// standard as recalled.
const (
	block     = 2880 // defined: a FITS block's bytes
	cardWidth = 80   // defined: a card's characters
	keyWidth  = 8    // defined: a keyword's characters
	longBytes = 8    // defined: a K column's bytes
	dblBytes  = 8    // defined: a D column's bytes
	sglBytes  = 4    // defined: an E column's bytes
	shrtBytes = 2    // defined: an I column's bytes
	textChars = 4    // defined: the text column a test lays out, 4A
	none      = 0    // defined: nothing
)

// The made-up star the product tests write and read back, each value
// defined here and each distinct from every other, so that two
// columns swapped in the reader are seen.
const (
	madeID           = 6_000_000_000_000_000_000 // defined: past 2^53
	madeEpoch        = 2016.0                    // defined: a Julian year
	madeRA           = 266.4                     // defined: degrees
	madeRAError      = 0.31                      // defined: mas
	madeDec          = -29.0                     // defined: degrees
	madeDecError     = 0.32                      // defined: mas
	madeParallax     = 0.5                       // defined: mas
	madeParallaxErr  = 0.05                      // defined: mas
	madePMRA         = -3.1                      // defined: mas a year
	madePMRAError    = 0.41                      // defined: mas a year
	madePMDec        = -5.7                      // defined: mas a year
	madePMDecError   = 0.42                      // defined: mas a year
	madeRadial       = 123.4                     // defined: km/s
	madeRadialError  = 1.7                       // defined: km/s
	madeRUWE         = 1.1                       // defined: its RUWE
	madeMagnitude    = 15.5                      // defined: G
	madeColour       = 1.2                       // defined: BP minus RP
	madeNuEff        = 1.55                      // defined: 1/um
	madePseudocolour = 1.65                      // defined: 1/um
	madeEclLat       = -5.9                      // defined: degrees
	madeSolved       = 31                        // defined: all five
	firstCorr        = 0.11                      // defined: the first
	corrStep         = 0.01                      // defined: each one's step
	tenCorrs         = 10                        // defined: pairs of five
	itself           = 1                         // defined: one with itself
	singleRoom       = 1e-6                      // defined: float32 rounding
	doubleRoom       = 1e-14                     // defined: a round trip
	firstRow         = 0                         // defined: the first row
	oneRow           = 1                         // defined: one row
	twoRows          = 2                         // defined: two rows
	pastTheSky       = 91                        // defined: past the pole
	noParameter      = 7                         // defined: not of the five
	belowRA          = -1                        // defined: before the first
	fiveOfThem       = 5                         // defined: the count, not one
	masInArcsecond   = 1000                      // defined: mas in one
)

// layout is a column of the product as the archive writes it: its name,
// its form and its unit.
type layout struct{ name, form, unit string }

// archiveLayout is the query's 31 columns as the archive answered them,
// read 2026-11-05 from the product's header.
func archiveLayout() []layout {
	return []layout{{"source_id", "K", ""}, {"ref_epoch", "D", "yr"},
		{"ra", "D", "deg"}, {"ra_error", "E", "mas"}, {"dec", "D", "deg"},
		{"dec_error", "E", "mas"}, {"parallax", "D", "mas"},
		{"parallax_error", "E", "mas"}, {"pmra", "D", "mas.yr**-1"},
		{"pmra_error", "E", "mas.yr**-1"}, {"pmdec", "D", "mas.yr**-1"},
		{"pmdec_error", "E", "mas.yr**-1"}, {"radial_velocity", "E", "km.s**-1"},
		{"radial_velocity_error", "E", "km.s**-1"}, {"ra_dec_corr", "E", ""},
		{"ra_parallax_corr", "E", ""}, {"ra_pmra_corr", "E", ""},
		{"ra_pmdec_corr", "E", ""}, {"dec_parallax_corr", "E", ""},
		{"dec_pmra_corr", "E", ""}, {"dec_pmdec_corr", "E", ""},
		{"parallax_pmra_corr", "E", ""}, {"parallax_pmdec_corr", "E", ""},
		{"pmra_pmdec_corr", "E", ""}, {"ruwe", "E", ""},
		{"phot_g_mean_mag", "E", "mag"}, {"bp_rp", "E", "mag"},
		{"nu_eff_used_in_astrometry", "E", "um**-1"},
		{"pseudocolour", "E", "um**-1"}, {"ecl_lat", "D", "deg"},
		{"astrometric_params_solved", "I", ""}}
}

// bytesOf is the bytes a column of a form takes in a row.
func bytesOf(form string) int {
	switch form {
	case "K":
		return longBytes
	case "D":
		return dblBytes
	case "E":
		return sglBytes
	case "4A":
		return textChars
	}
	return shrtBytes
}

// card is one header card: a keyword, its value and no comment.
func card(key, value string) string {
	return fmt.Sprintf("%-*s= %s", keyWidth, key, value)
}

// quoted is a card of a numbered keyword with a string value.
func quoted(prefix string, number int, value string) string {
	return card(fmt.Sprintf("%s%d", prefix, number), fmt.Sprintf("'%s'", value))
}

// hdu is a header's cards and its data as the standard lays them out:
// each card padded to its width, the header ended and padded to whole
// blocks with spaces, the data padded with zeros.
func hdu(cards []string, data []byte) []byte {
	var out bytes.Buffer
	for _, each := range append(cards, "END") {
		fmt.Fprintf(&out, "%-*s", cardWidth, each)
	}
	for out.Len()%block != none {
		out.WriteByte(' ')
	}
	out.Write(data)
	for out.Len()%block != none {
		out.WriteByte(none)
	}
	return out.Bytes()
}

// productCards are the cards of a product's table of so many rows, laid
// out as given.
func productCards(rows int, columns []layout) []string {
	width := none
	for _, each := range columns {
		width += bytesOf(each.form)
	}
	fields := fmt.Sprint(len(columns))
	cards := []string{card("XTENSION", "'BINTABLE'"), card("BITPIX", "8"),
		card("NAXIS", "2"), card("NAXIS1", fmt.Sprint(width)),
		card("NAXIS2", fmt.Sprint(rows)), card("PCOUNT", "0"),
		card("GCOUNT", "1"), card("TFIELDS", fields)}
	for which, each := range columns {
		number := which + oneRow
		cards = append(cards, quoted("TTYPE", number, each.name),
			quoted("TFORM", number, each.form))
		if each.unit != "" {
			cards = append(cards, quoted("TUNIT", number, each.unit))
		}
	}
	return cards
}

// product is a file of a primary array of nothing and a table of the
// rows given, laid out as given.
func product(columns []layout, rows ...[]byte) []byte {
	primary := hdu([]string{card("SIMPLE", "T"), card("BITPIX", "8"),
		card("NAXIS", "0")}, nil)
	var data bytes.Buffer
	for _, row := range rows {
		data.Write(row)
	}
	cards := productCards(len(rows), columns)
	table := hdu(cards, data.Bytes())
	return append(primary, table...)
}

// madeUp is the made-up star as the archive would write it: its values
// in the query's order, by form.
type madeUp struct {
	id                      int64
	epoch, ra, dec          float64
	parallax, pmRA, pmDec   float64
	raError, decError       float32
	parallaxError           float32
	pmRAError, pmDecError   float32
	radial, radialError     float32
	corrs                   [tenCorrs]float32
	ruwe, magnitude, colour float32
	nuEff, pseudocolour     float32
	eclLat                  float64
	solved                  int16
}

// theStar is the made-up star with every value defined above, its
// correlations stepping up from the first.
func theStar() madeUp {
	star := madeUp{id: madeID, epoch: madeEpoch, ra: madeRA, dec: madeDec,
		parallax: madeParallax, pmRA: madePMRA, pmDec: madePMDec,
		raError: madeRAError, decError: madeDecError,
		parallaxError: madeParallaxErr, pmRAError: madePMRAError,
		pmDecError: madePMDecError, radial: madeRadial,
		radialError: madeRadialError, ruwe: madeRUWE, magnitude: madeMagnitude,
		colour: madeColour, nuEff: madeNuEff, pseudocolour: madePseudocolour,
		eclLat: madeEclLat, solved: madeSolved}
	for which := range star.corrs {
		star.corrs[which] = float32(firstCorr + corrStep*float64(which))
	}
	return star
}

// written is the star's row as the archive stores it, in the query's
// order and each column's form.
func written(star madeUp) []byte {
	var row bytes.Buffer
	for _, value := range []any{star.id, star.epoch, star.ra, star.raError,
		star.dec, star.decError, star.parallax, star.parallaxError,
		star.pmRA, star.pmRAError, star.pmDec, star.pmDecError, star.radial,
		star.radialError, star.corrs, star.ruwe, star.magnitude, star.colour,
		star.nuEff, star.pseudocolour, star.eclLat, star.solved} {
		binary.Write(&row, binary.BigEndian, value)
	}
	return row.Bytes()
}

// stopping is a test or a property's draw that can be stopped.
type stopping interface {
	Helper()
	Fatal(args ...any)
}

// opened is the catalogue in a product, or the test stops.
func opened(t stopping, file []byte) *gaia.Catalogue {
	t.Helper()
	catalogue, err := gaia.OpenCatalogue(bytes.NewReader(file))
	if err != nil {
		t.Fatal(err)
	}
	return catalogue
}

// ofStars is the catalogue of a product of the stars given, laid out as
// the archive does, or the test stops.
func ofStars(t stopping, stars ...madeUp) *gaia.Catalogue {
	t.Helper()
	var rows [][]byte
	for _, star := range stars {
		rows = append(rows, written(star))
	}
	file := product(archiveLayout(), rows...)
	return opened(t, file)
}

// first is the first source of a catalogue, or the test stops.
func first(t stopping, catalogue *gaia.Catalogue) *gaia.Source {
	t.Helper()
	source, err := catalogue.Source(firstRow)
	if err != nil {
		t.Fatal(err)
	}
	return source
}

// theSource is the made-up star, written, read back, or the test
// stops.
func theSource(t *testing.T) *gaia.Source {
	t.Helper()
	catalogue := ofStars(t, theStar())
	return first(t, catalogue)
}

// reading is a value read back beside the value written.
type reading struct{ got, want float64 }

// near says whether a number is within a share of another.
func near(got, want, share float64) bool {
	return math.Abs(got-want) <= math.Abs(want)*share
}

// masOf is an angle in milliarcseconds.
func masOf(angle interface{ InArcseconds() float64 }) float64 {
	return angle.InArcseconds() * masInArcsecond
}

// the made-up values are each distinct, so that two columns swapped in
// the reader are seen by the round trip.
func TestTheMadeUpValuesAreDistinct(t *testing.T) {
	star := theStar()
	values := []float64{star.epoch, star.ra, star.dec, star.parallax,
		star.pmRA, star.pmDec, float64(star.raError), float64(star.decError),
		float64(star.parallaxError), float64(star.pmRAError),
		float64(star.pmDecError), float64(star.radial),
		float64(star.radialError), float64(star.ruwe), float64(star.magnitude),
		float64(star.colour), float64(star.nuEff), float64(star.pseudocolour),
		star.eclLat, float64(star.solved)}
	for _, corr := range star.corrs {
		values = append(values, float64(corr))
	}
	seen := map[float64]bool{}
	for _, value := range values {
		if seen[value] {
			t.Errorf("%g is written in two columns", value)
		}
		seen[value] = true
	}
}

// the made-up star written as the archive writes a row is read back as
// celestial's types: its place in degrees, its errors and parallax in
// milliarcseconds, its motion in milliarcseconds a year, its speed in
// kilometres a second, each as stored, a single to its rounding.
func TestASourceIsReadBackAsItWasWritten(t *testing.T) {
	source := theSource(t)
	raError, decError := source.PlaceError()
	pmRAError, pmDecError := source.MotionError()
	doubles := map[string]reading{
		"epoch":    {source.Epoch(), madeEpoch},
		"ra":       {source.Place().Ascension().InDegrees(), madeRA},
		"dec":      {source.Place().Declination().InDegrees(), madeDec},
		"parallax": {masOf(source.Parallax()), madeParallax},
		"pmra":     {source.Motion().Ascension(), madePMRA},
		"pmdec":    {source.Motion().Declination(), madePMDec},
		"ecliptic": {source.EclipticLatitude().InDegrees(), madeEclLat},
	}
	singles := map[string]reading{
		"ra error":       {masOf(raError), madeRAError},
		"dec error":      {masOf(decError), madeDecError},
		"parallax error": {masOf(source.ParallaxError()), madeParallaxErr},
		"pmra error":     {pmRAError, madePMRAError},
		"pmdec error":    {pmDecError, madePMDecError},
		"radial":         {source.RadialVelocity(), madeRadial},
		"radial error":   {source.RadialVelocityError(), madeRadialError},
		"ruwe":           {source.RUWE(), madeRUWE},
		"magnitude":      {source.Magnitude(), madeMagnitude},
		"colour":         {source.Colour(), madeColour},
		"nu eff":         {source.EffectiveWavenumber(), madeNuEff},
		"pseudocolour":   {source.Pseudocolour(), madePseudocolour},
	}
	for name, read := range doubles {
		if !near(read.got, read.want, doubleRoom) {
			t.Errorf("%s is %g, want %g", name, read.got, read.want)
		}
	}
	for name, read := range singles {
		if !near(read.got, read.want, singleRoom) {
			t.Errorf("%s is %g, want %g", name, read.got, read.want)
		}
	}
	if source.ID() != madeID || source.ParametersSolved() != madeSolved {
		t.Errorf("the id and the parameters solved are %d and %d",
			source.ID(), source.ParametersSolved())
	}
}

// pairing is two of the five parameters.
type pairing struct{ one, other gaia.Parameter }

// archivePairs are the ten pairs in the order the archive writes their
// correlations.
func archivePairs() []pairing {
	return []pairing{{gaia.RA, gaia.Dec}, {gaia.RA, gaia.Parallax},
		{gaia.RA, gaia.PMRA}, {gaia.RA, gaia.PMDec}, {gaia.Dec, gaia.Parallax},
		{gaia.Dec, gaia.PMRA}, {gaia.Dec, gaia.PMDec},
		{gaia.Parallax, gaia.PMRA}, {gaia.Parallax, gaia.PMDec},
		{gaia.PMRA, gaia.PMDec}}
}

// the correlations are read in the archive's order, both ways round,
// with one for a parameter with itself.
func TestTheCorrelationsAreReadBothWaysRound(t *testing.T) {
	corrs := theSource(t).Correlations()
	for which, pair := range archivePairs() {
		want := firstCorr + corrStep*float64(which)
		one, oneErr := corrs.Between(pair.one, pair.other)
		other, otherErr := corrs.Between(pair.other, pair.one)
		if oneErr != nil || otherErr != nil || !near(one, want, singleRoom) ||
			one != other {
			t.Errorf("pair %d is %g and %g, %v %v, want %g", which, one, other,
				oneErr, otherErr, want)
		}
	}
	for _, each := range []gaia.Parameter{gaia.RA, gaia.Dec, gaia.Parallax,
		gaia.PMRA, gaia.PMDec} {
		if self, err := corrs.Between(each, each); err != nil ||
			self != itself {
			t.Errorf("parameter %d with itself is %g, %v", each, self, err)
		}
	}
}

// the anti-test: a parameter that is not one of the five is refused,
// with not a number beside the error.
func TestAParameterThatIsNotOneOfTheFiveIsRefused(t *testing.T) {
	corrs := theSource(t).Correlations()
	for _, pair := range []pairing{{noParameter, gaia.RA},
		{gaia.RA, noParameter}, {belowRA, gaia.RA}, {gaia.RA, fiveOfThem}} {
		if got, err := corrs.Between(pair.one, pair.other); !errors.Is(err,
			celestialdata.Refused()) || !math.IsNaN(got) {
			t.Errorf("parameters %v gave %g, %v", pair, got, err)
		}
	}
}

// a value the archive left empty, a float that is not a number, is
// read as not a number and refuses nothing.
func TestAnEmptyValueIsNotANumber(t *testing.T) {
	star := theStar()
	star.radialError = float32(math.NaN())
	catalogue := ofStars(t, star)
	source := first(t, catalogue)
	if !math.IsNaN(source.RadialVelocityError()) {
		t.Errorf("an empty error is %g", source.RadialVelocityError())
	}
	if !near(source.RadialVelocity(), madeRadial, singleRoom) {
		t.Errorf("the speed beside it is %g", source.RadialVelocity())
	}
}

// rows are read in any order, and the count is the table's.
func TestRowsAreReadInAnyOrder(t *testing.T) {
	far := theStar()
	far.id = madeID + oneRow
	catalogue := ofStars(t, theStar(), far)
	if catalogue.Rows() != twoRows {
		t.Fatalf("%d rows", catalogue.Rows())
	}
	for index, want := range map[int64]int64{oneRow: madeID + oneRow,
		firstRow: madeID} {
		source, err := catalogue.Source(index)
		if err != nil || source.ID() != want {
			t.Errorf("row %d is %v, %v", index, source, err)
		}
	}
}

// without is the layout with one column left out.
func without(name string) []layout {
	var kept []layout
	for _, each := range archiveLayout() {
		if each.name != name {
			kept = append(kept, each)
		}
	}
	return kept
}

// inUnit is the layout with one column in another unit.
func inUnit(name, unit string) []layout {
	changed := archiveLayout()
	for which, each := range changed {
		if each.name == name {
			changed[which].unit = unit
		}
	}
	return changed
}

// ofForm is the layout with one column of another form.
func ofForm(name, form string) []layout {
	changed := archiveLayout()
	for which, each := range changed {
		if each.name == name {
			changed[which].form = form
		}
	}
	return changed
}

// refusal is a product that is refused, and the words its refusal
// says.
type refusal struct {
	file []byte
	said string
}

// the anti-test: a product that is not the query's is refused, saying
// which column: one that lacks a column, one with a column in another
// unit, one with a column of another kind (ra as a single would read
// in silence at float32), and one with no table at all.
func TestAProductThatIsNotTheQuerysIsRefused(t *testing.T) {
	row := written(theStar())
	primary := hdu([]string{card("SIMPLE", "T"), card("BITPIX", "8"),
		card("NAXIS", "0")}, nil)
	files := map[string]refusal{
		"no ruwe":      {product(without("ruwe"), row), "no ruwe"},
		"ra in rad":    {product(inUnit("ra", "rad"), row), `ra is in "rad"`},
		"ra a single":  {product(ofForm("ra", "E"), row), "ra is of kind E"},
		"id a double":  {product(ofForm("source_id", "D"), row), "source_id is of kind D"},
		"ruwe as text": {product(ofForm("ruwe", "4A"), row), "ruwe is of kind A"},
		"no table":     {primary, "no binary table"},
	}
	for why, each := range files {
		catalogue, err := gaia.OpenCatalogue(bytes.NewReader(each.file))
		if !errors.Is(err, celestialdata.Refused()) || catalogue != nil ||
			!strings.Contains(err.Error(), each.said) {
			t.Errorf("%s: %v, %v", why, catalogue, err)
		}
	}
}

// the anti-test: a row the catalogue does not have is refused, and so
// is a source whose place is not on the sky, by its id.
func TestARowNotThereOrOffTheSkyIsRefused(t *testing.T) {
	off := theStar()
	off.dec = pastTheSky
	catalogue := ofStars(t, theStar(), off)
	if source, err := catalogue.Source(catalogue.Rows()); err == nil ||
		source != nil {
		t.Errorf("a row past the end gave %v, %v", source, err)
	}
	source, err := catalogue.Source(oneRow)
	if !errors.Is(err, celestialdata.Refused()) || source != nil ||
		!strings.Contains(err.Error(), fmt.Sprint(madeID)) {
		t.Errorf("a source off the sky gave %v, %v", source, err)
	}
}

// the anti-test: a row the file cannot give is an error, not a source.
func TestARowTheFileCannotGiveIsAnError(t *testing.T) {
	row := written(theStar())
	whole := product(archiveLayout(), row)
	catalogue := opened(t, whole[:len(whole)-block])
	if source, err := catalogue.Source(firstRow); err == nil || source != nil {
		t.Errorf("a row past the cut gave %v, %v", source, err)
	}
}
