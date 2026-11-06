package apogee_test

import (
	"bytes"
	"errors"
	"fmt"
	"math"
	"slices"
	"strconv"
	"strings"
	"testing"

	"pgregory.net/rapid"

	"github.com/janearc/celestial-data"
	"github.com/janearc/celestial-data/apogee"
	"github.com/janearc/celestial-data/internal/fitstest"
)

// The made-up star the tests write and read back, each value defined
// here and distinct from every other, so that two columns swapped in
// the reader are seen. The numbers are sums of halves, so a single
// holds them exactly.
const (
	madeID         = "2M17455519-2900281"      // defined: an APOGEE_ID's form
	madeTelescope  = "apo25m"                  // defined: a telescope's name
	madeField      = "GALCEN"                  // defined: a field's name
	madeRA         = 266.4                     // defined: degrees
	madeDec        = -29.0                     // defined: degrees
	madeVisits     = 3                         // defined: NVISITS, not read
	madeSNR        = 151.5                     // defined: signal to noise
	madeConditions = 1 << 16                   // defined: STARFLAG's SUSPECT_RV_COMBINATION
	madeRadial     = -423.25                   // defined: km/s
	madeScatter    = 0.796875                  // defined: km/s
	madeRadialErr  = 0.125                     // defined: km/s
	madeGaia       = 4_037_334_576_111_139_712 // defined: past 2^53
	madeASPCAP     = 1<<23 | 1<<3              // defined: STAR_BAD and M_H_WARN
	madeTargeting  = 1 << 4                    // defined: EXTRATARG's DUPLICATE
	madeTeff       = 4321.5                    // defined: kelvin
	madeTeffErr    = 81.25                     // defined: kelvin
	madeLogg       = 1.375                     // defined: log g, cgs
	madeLoggErr    = 0.0625                    // defined: dex
	madeMetals     = -0.4375                   // defined: dex
	madeMetalsErr  = 0.015625                  // defined: dex
	madeAlpha      = 0.1875                    // defined: dex
	madeAlphaErr   = 0.0078125                 // defined: dex
	madeIron       = -0.46875                  // defined: dex
	madeIronErr    = 0.01171875                // defined: dex
	madeIronFlag   = 1<<8 | 1                  // defined: GRIDEDGE_WARN and GRIDEDGE_BAD
	elementBase    = 0.25                      // defined: the first element's [X/Fe]
	elementStep    = 0.03125                   // defined: each next element's step up
	uncertainBase  = 0.5                       // defined: the first element's uncertainty
	uncertainStep  = 0.0078125                 // defined: each next one's step up
	flagBase       = 1000                      // defined: the first element's flag, each next one up
	pastThePole    = 91                        // defined: degrees, past the north pole
)

// The made-up file's layout, each defined.
const (
	idWidth        = 18   // defined: APOGEE_ID's characters
	telescopeWidth = 6    // defined: TELESCOPE's characters
	fieldWidth     = 8    // defined: FIELD's characters
	paramValues    = 9    // defined: PARAM's values a row, stepped over
	singleBytes    = 4    // defined: an E value's bytes
	fileRows       = 3    // defined: the Find file's rows
	noRows         = 0    // defined: a file of none
	oneRow         = 1    // defined: a file of one
	firstRow       = 0    // defined: the first row
	thirdRow       = 2    // defined: the third
	otherGaia      = 77   // defined: another source
	absentGaia     = 78   // defined: a source no row holds
	topBit         = 63   // defined: a mask's last bit
	pastTheMask    = 64   // defined: a bit past a 64-bit mask
	farPast        = 200  // defined: a bit far past it
	mostRows       = 12   // defined: the property's most rows
	sourcePool     = 5    // defined: the sources the property's rows draw from
	cutBlock       = 2880 // defined: a FITS block's bytes, the file cut by one
	everyBit       = -1   // defined: a mask with every bit set
	lowest         = 1    // defined: a mask of the lowest bit, before its shift
)

// madeUp is a star as the file writes it, value by value.
type madeUp struct {
	id        string
	ra, dec   float64
	gaia      int64
	targeting int32
}

// theStar is the made-up star with every value defined above.
func theStar() madeUp {
	return madeUp{id: madeID, ra: madeRA, dec: madeDec, gaia: madeGaia,
		targeting: madeTargeting}
}

// layout is the made-up file's columns in the order the real file
// writes them, two it does not read among them: NVISITS, and PARAM of
// nine values a row.
func layout() []fitstest.Column {
	columns := []fitstest.Column{
		fitstest.NewColumn("APOGEE_ID", fmt.Sprintf("%dA", idWidth)),
		fitstest.NewColumn("TELESCOPE", fmt.Sprintf("%dA", telescopeWidth)),
		fitstest.NewColumn("FIELD", fmt.Sprintf("%dA", fieldWidth)),
		fitstest.NewColumn("RA", "D"), fitstest.NewColumn("DEC", "D"),
		fitstest.NewColumn("NVISITS", "J"), fitstest.NewColumn("SNR", "E"),
		fitstest.NewColumn("STARFLAG", "K"), fitstest.NewColumn("VHELIO_AVG", "E"),
		fitstest.NewColumn("VSCATTER", "E"), fitstest.NewColumn("VERR", "E"),
		fitstest.NewColumn("GAIAEDR3_SOURCE_ID", "K"), fitstest.NewColumn("ASPCAPFLAG", "K"),
		fitstest.NewColumn("PARAM", fmt.Sprintf("%dE", paramValues)),
		fitstest.NewColumn("EXTRATARG", "J"), fitstest.NewColumn("TEFF", "E"),
		fitstest.NewColumn("TEFF_ERR", "E"), fitstest.NewColumn("LOGG", "E"),
		fitstest.NewColumn("LOGG_ERR", "E"), fitstest.NewColumn("M_H", "E"),
		fitstest.NewColumn("M_H_ERR", "E"), fitstest.NewColumn("ALPHA_M", "E"),
		fitstest.NewColumn("ALPHA_M_ERR", "E"), fitstest.NewColumn("FE_H", "E"),
		fitstest.NewColumn("FE_H_ERR", "E"), fitstest.NewColumn("FE_H_FLAG", "J")}
	for _, symbol := range apogee.Elements() {
		stem := strings.ToUpper(symbol)
		columns = append(columns, fitstest.NewColumn(stem+"_FE", "E"),
			fitstest.NewColumn(stem+"_FE_ERR", "E"),
			fitstest.NewColumn(stem+"_FE_FLAG", "J"))
	}
	return columns
}

// written is a star's row, in the layout's order.
func written(star madeUp) *fitstest.Row {
	row := fitstest.NewRow()
	row.Text(star.id, idWidth).Text(madeTelescope, telescopeWidth).
		Text(madeField, fieldWidth).Double(star.ra).Double(star.dec)
	row.Int(madeVisits).Single(madeSNR).Long(madeConditions)
	row.Single(madeRadial).Single(madeScatter).Single(madeRadialErr)
	row.Long(star.gaia).Long(madeASPCAP).Skip(paramValues * singleBytes)
	row.Int(star.targeting).Single(madeTeff).Single(madeTeffErr)
	row.Single(madeLogg).Single(madeLoggErr).Single(madeMetals)
	row.Single(madeMetalsErr).Single(madeAlpha).Single(madeAlphaErr)
	row.Single(madeIron).Single(madeIronErr).Int(madeIronFlag)
	for at := range apogee.Elements() {
		row.Single(elementValue(at)).Single(elementUncertainty(at))
		row.Int(int32(flagBase + at))
	}
	return row
}

// elementValue is the made-up [X/Fe] of the element at a place.
func elementValue(at int) float32 {
	return float32(elementBase + float64(at)*elementStep)
}

// elementUncertainty is the made-up uncertainty of the element at a
// place.
func elementUncertainty(at int) float32 {
	return float32(uncertainBase + float64(at)*uncertainStep)
}

// opened is the catalogue of a file's bytes, or the test stops.
func opened(test stopping, file []byte) *apogee.Catalogue {
	test.Helper()
	catalogue, err := apogee.OpenCatalogue(bytes.NewReader(file))
	if err != nil {
		test.Fatal(err)
	}
	return catalogue
}

// ofStars is the catalogue of a file of the stars' rows, or the test
// stops.
func ofStars(test stopping, stars ...madeUp) *apogee.Catalogue {
	test.Helper()
	rows := make([]*fitstest.Row, len(stars))
	for at, star := range stars {
		rows[at] = written(star)
	}
	file := fitstest.File(layout(), rows...)
	return opened(test, file)
}

// theRow is the made-up star read back from a file of it alone, or the
// test stops.
func theRow(test *testing.T) *apogee.Star {
	test.Helper()
	catalogue := ofStars(test, theStar())
	star, err := catalogue.Star(firstRow)
	if err != nil {
		test.Fatal(err)
	}
	return star
}

// stopping is what the helpers stop a test or a property through.
type stopping interface {
	Helper()
	Fatal(args ...any)
}

// fitted is a value and its uncertainty as read, beside what was
// written.
type fitted struct {
	name                    string
	value, uncertainty      float64
	wrote, wroteUncertainty float64
}

// fittedOf is a pair read through a star's method, beside what was
// written.
func fittedOf(name string, read func() (float64, float64), wrote,
	wroteUncertainty float64) fitted {
	value, uncertainty := read()
	return fitted{name: name, value: value, uncertainty: uncertainty,
		wrote: wrote, wroteUncertainty: wroteUncertainty}
}

// the test: the made-up star reads back as it was written, every value
// in its place, the columns not read stepped over.
func TestAStarIsReadBackAsItWasWritten(test *testing.T) {
	star := theRow(test)
	if star.GaiaID() != madeGaia || star.ID() != madeID ||
		star.Telescope() != madeTelescope || star.Field() != madeField {
		test.Errorf("%d %q %q %q", star.GaiaID(), star.ID(), star.Telescope(),
			star.Field())
	}
	place := star.Place()
	if place == nil {
		test.Fatal("the made-up star has no place")
	}
	ra, dec := place.Ascension().InDegrees(), place.Declination().InDegrees()
	if ra != madeRA || dec != madeDec || star.SNR() != madeSNR {
		test.Errorf("ra %g dec %g snr %g", ra, dec, star.SNR())
	}
	if star.RadialVelocity() != madeRadial ||
		star.RadialScatter() != madeScatter ||
		star.RadialError() != madeRadialErr {
		test.Errorf("rv %g scatter %g error %g", star.RadialVelocity(),
			star.RadialScatter(), star.RadialError())
	}
	pairs := []fitted{
		fittedOf("temperature", star.Temperature, madeTeff, madeTeffErr),
		fittedOf("gravity", star.Gravity, madeLogg, madeLoggErr),
		fittedOf("metals", star.Metals, madeMetals, madeMetalsErr),
		fittedOf("alpha", star.Alpha, madeAlpha, madeAlphaErr)}
	for _, pair := range pairs {
		if pair.value != pair.wrote || pair.uncertainty != pair.wroteUncertainty {
			test.Errorf("%s read %g +- %g, wrote %g +- %g", pair.name,
				pair.value, pair.uncertainty, pair.wrote, pair.wroteUncertainty)
		}
	}
	iron := star.Iron()
	if iron.Value() != madeIron || iron.Uncertainty() != madeIronErr ||
		iron.Flags() != madeIronFlag {
		test.Errorf("iron %g +- %g flags %d", iron.Value(), iron.Uncertainty(),
			iron.Flags())
	}
	if star.ASPCAP() != madeASPCAP || star.Conditions() != madeConditions ||
		star.Targeting() != madeTargeting {
		test.Errorf("aspcap %d conditions %d targeting %d", star.ASPCAP(),
			star.Conditions(), star.Targeting())
	}
}

// the test: each element's abundance is read from its own three
// columns, by its symbol.
func TestEachElementIsReadInItsPlace(test *testing.T) {
	star := theRow(test)
	for at, symbol := range apogee.Elements() {
		abundance, err := star.Abundance(symbol)
		if err != nil {
			test.Fatal(err)
		}
		if abundance.Value() != float64(elementValue(at)) ||
			abundance.Uncertainty() != float64(elementUncertainty(at)) ||
			abundance.Flags() != apogee.Flags(flagBase+at) {
			test.Errorf("%s: %g +- %g flags %d", symbol, abundance.Value(),
				abundance.Uncertainty(), abundance.Flags())
		}
	}
}

// the test: a flag says each of its bits, by the survey's numbers, and
// no bit past its 64.
func TestTheFlagsSayTheirBits(test *testing.T) {
	star := theRow(test)
	aspcap, iron := star.ASPCAP(), star.Iron().Flags()
	if !aspcap.Has(apogee.StarBad) || !aspcap.Has(apogee.MetalsWarn) ||
		aspcap.Has(apogee.MetalsBad) || aspcap.Has(apogee.StarWarn) ||
		aspcap.Has(apogee.MultipleSuspect) {
		test.Errorf("ASPCAPFLAG %b", aspcap)
	}
	if !iron.Has(apogee.GridEdgeBad) || !iron.Has(apogee.GridEdgeWarn) ||
		iron.Has(apogee.CalibrationBad) || iron.Has(apogee.CalibrationWarn) {
		test.Errorf("FE_H_FLAG %b", iron)
	}
	if !star.Targeting().Has(apogee.Duplicate) {
		test.Errorf("EXTRATARG %b", star.Targeting())
	}
	every := apogee.Flags(everyBit)
	if !every.Has(topBit) || every.Has(pastTheMask) || every.Has(farPast) {
		test.Error("a bit past the mask is set")
	}
	suspect := apogee.Flags(lowest << apogee.MultipleSuspect)
	if !suspect.Has(apogee.MultipleSuspect) || suspect.Has(apogee.StarBad) {
		test.Errorf("MULTIPLE_SUSPECT %b", suspect)
	}
}

// the test: Elements is a copy; a change to one is not seen in the
// next.
func TestElementsIsACopy(test *testing.T) {
	changed := apogee.Elements()
	changed[firstRow] = "Zz"
	if apogee.Elements()[firstRow] == "Zz" {
		test.Error("a change to Elements is kept")
	}
}

// the anti-test: an abundance of an element the file holds none of is
// refused, saying which: iron is the star's Iron, and symbols are as
// Elements spells them.
func TestAnAbundanceTheFileLacksIsRefused(test *testing.T) {
	star := theRow(test)
	for _, symbol := range []string{"Fe", "mg", ""} {
		abundance, err := star.Abundance(symbol)
		said := fmt.Sprintf("holds no abundance of %q", symbol)
		if abundance != nil || !errors.Is(err, celestialdata.Refused()) ||
			!strings.Contains(err.Error(), said) {
			test.Errorf("%q: %v, %v", symbol, abundance, err)
		}
	}
}

// without is the layout lacking a column.
func without(name string) []fitstest.Column {
	var kept []fitstest.Column
	for _, each := range layout() {
		if each.Name() != name {
			kept = append(kept, each)
		}
	}
	return kept
}

// retyped is the layout with a column's form and unit changed.
func retyped(name, form, unit string) []fitstest.Column {
	changed := layout()
	for at, each := range changed {
		if each.Name() == name {
			changed[at] = fitstest.NewColumn(name, form).In(unit)
		}
	}
	return changed
}

// breakage is a layout the reader refuses, and the words it says.
type breakage struct {
	columns []fitstest.Column
	said    string
}

// breakages are the layouts the reader refuses: each column read taken
// away, and columns of another kind, of many values or with a unit.
func breakages() map[string]breakage {
	broken := map[string]breakage{
		"a double TEFF": {retyped("TEFF", "D", ""), "TEFF is of kind D, not E"},
		"two TEFFs a row": {retyped("TEFF", "2E", ""),
			"TEFF holds 2 values a row, not one"},
		"TEFF in kelvin": {retyped("TEFF", "E", "K"),
			`TEFF is in "K", where the allStar file writes no unit`},
		"a long element flag": {retyped("CE_FE_FLAG", "K", ""),
			"CE_FE_FLAG is of kind K, not J"},
		"a double element error": {retyped("YB_FE_ERR", "D", ""),
			"YB_FE_ERR is of kind D, not E"},
		"a long element": {retyped("C_FE", "K", ""), "C_FE is of kind K, not E"}}
	for _, each := range layout() {
		if each.Name() == "NVISITS" || each.Name() == "PARAM" {
			continue
		}
		broken["no "+each.Name()] = breakage{without(each.Name()),
			"the allStar file has no " + each.Name()}
	}
	return broken
}

// the anti-test: a file the reader cannot read is refused, saying
// why: one that is not a FITS file, and each of the breakages.
func TestAFileNotReadIsRefused(test *testing.T) {
	catalogue, err := apogee.OpenCatalogue(strings.NewReader("not a file"))
	if catalogue != nil || !errors.Is(err, celestialdata.Refused()) ||
		!strings.Contains(err.Error(), "the allStar file") {
		test.Errorf("not a file: %v", err)
	}
	for why, broken := range breakages() {
		file := fitstest.File(broken.columns)
		catalogue, err := apogee.OpenCatalogue(bytes.NewReader(file))
		if catalogue != nil || !errors.Is(err, celestialdata.Refused()) ||
			!strings.Contains(err.Error(), broken.said) {
			test.Errorf("%s: %v", why, err)
		}
	}
}

// the anti-test: a row the file does not have, or cannot give, is
// refused, by Star and by Find, saying which row; a star past the pole
// is refused by its id, by Star and by Find.
func TestARowNotReadIsRefused(test *testing.T) {
	catalogue := ofStars(test, theStar())
	if star, err := catalogue.Star(oneRow); star != nil || err == nil {
		test.Errorf("a row past the last: %v", err)
	}
	row := written(theStar())
	whole := fitstest.File(layout(), row)
	cut := opened(test, whole[:len(whole)-cutBlock])
	star, err := cut.Star(firstRow)
	if star != nil || err == nil || !strings.Contains(err.Error(), cutRow) {
		test.Errorf("a row cut away, by Star: %v", err)
	}
	found, err := cut.Find(madeGaia)
	if found != nil || err == nil || !strings.Contains(err.Error(), cutRow) {
		test.Errorf("a row cut away, by Find: %v", err)
	}
	pole := theStar()
	pole.dec = pastThePole
	beyond := ofStars(test, pole)
	star, err = beyond.Star(firstRow)
	if star != nil || !errors.Is(err, celestialdata.Refused()) ||
		!strings.Contains(err.Error(), madeID) {
		test.Errorf("a star past the pole, by Star: %v", err)
	}
	found, err = beyond.Find(madeGaia)
	if found != nil || !errors.Is(err, celestialdata.Refused()) ||
		!strings.Contains(err.Error(), madeID) {
		test.Errorf("a star past the pole, by Find: %v", err)
	}
}

// The bits' numbers as the survey's bitmask page gives them, read
// 2026-11-06, each defined there.
const (
	pageMetalsWarn   = 3            // defined: M_H_WARN, of ASPCAPFLAG
	pageStarWarn     = 7            // defined: STAR_WARN, of ASPCAPFLAG
	pageMetalsBad    = 19           // defined: M_H_BAD, of ASPCAPFLAG
	pageStarBad      = 23           // defined: STAR_BAD, of ASPCAPFLAG
	pageMultiple     = 41           // defined: MULTIPLE_SUSPECT, of ASPCAPFLAG
	pageGridEdgeBad  = 0            // defined: GRIDEDGE_BAD, of ELEMFLAG
	pageCalBad       = 1            // defined: CALRANGE_BAD, of ELEMFLAG
	pageGridEdgeWarn = 8            // defined: GRIDEDGE_WARN, of ELEMFLAG
	pageCalWarn      = 9            // defined: CALRANGE_WARN, of ELEMFLAG
	pageDuplicate    = 4            // defined: DUPLICATE, of EXTRATARG
	cutRow           = "row 0: EOF" // defined: the words of a row the file cannot give
)

// pageBit is a named bit and its number on the survey's bitmask page.
type pageBit struct {
	named, page uint
}

// pageBits are the bits the package names, each beside its number as
// the survey's bitmask page gives it, read 2026-11-06.
func pageBits() map[string]pageBit {
	return map[string]pageBit{
		"M_H_WARN":         {apogee.MetalsWarn, pageMetalsWarn},
		"STAR_WARN":        {apogee.StarWarn, pageStarWarn},
		"M_H_BAD":          {apogee.MetalsBad, pageMetalsBad},
		"STAR_BAD":         {apogee.StarBad, pageStarBad},
		"MULTIPLE_SUSPECT": {apogee.MultipleSuspect, pageMultiple},
		"GRIDEDGE_BAD":     {apogee.GridEdgeBad, pageGridEdgeBad},
		"CALRANGE_BAD":     {apogee.CalibrationBad, pageCalBad},
		"GRIDEDGE_WARN":    {apogee.GridEdgeWarn, pageGridEdgeWarn},
		"CALRANGE_WARN":    {apogee.CalibrationWarn, pageCalWarn},
		"DUPLICATE":        {apogee.Duplicate, pageDuplicate}}
}

// the test: each named bit is the survey's: a mask of the page's bit
// alone has it, and a mask of every other bit has it not.
func TestTheNamedBitsAreTheSurveys(test *testing.T) {
	for name, bit := range pageBits() {
		alone := apogee.Flags(lowest << bit.page)
		others := apogee.Flags(everyBit) &^ alone
		if !alone.Has(bit.named) || others.Has(bit.named) {
			test.Errorf("%s is bit %d, not the page's %d", name, bit.named,
				bit.page)
		}
	}
}

// the test: a row whose place the survey left empty, both numbers, is
// a star with no place; the anti-test: one empty number alone is
// refused, by the star's id.
func TestARowWithNoPlaceHasNone(test *testing.T) {
	nowhere := theStar()
	nowhere.ra, nowhere.dec = math.NaN(), math.NaN()
	star, err := ofStars(test, nowhere).Star(firstRow)
	if err != nil || star.Place() != nil || star.GaiaID() != madeGaia {
		test.Errorf("a row with no place: %v, %v", star, err)
	}
	halfway := theStar()
	halfway.dec = math.NaN()
	star, err = ofStars(test, halfway).Star(firstRow)
	if star != nil || !errors.Is(err, celestialdata.Refused()) ||
		!strings.Contains(err.Error(), madeID) {
		test.Errorf("a row with half a place: %v", err)
	}
}

// idsOf are the ids of the stars found.
func idsOf(stars []*apogee.Star) []string {
	ids := make([]string, len(stars))
	for at, star := range stars {
		ids[at] = star.ID()
	}
	return ids
}

// the test: Find gives every row of the sources asked, in the file's
// order, a source's two rows both; none for a source no row holds, or
// for none asked.
func TestFindGivesEveryRowOfTheSources(test *testing.T) {
	one, other, again := theStar(), theStar(), theStar()
	one.id, other.id, again.id = "first", "second", "third"
	other.gaia = otherGaia
	catalogue := ofStars(test, one, other, again)
	asks := map[string][]int64{"first third": {madeGaia},
		"first second third": {otherGaia, madeGaia}, "": {absentGaia},
		" ": {}}
	for want, sources := range asks {
		found, err := catalogue.Find(sources...)
		got := strings.Join(idsOf(found), " ")
		if err != nil || strings.TrimSpace(got) != strings.TrimSpace(want) {
			test.Errorf("Find(%v): %q, %v", sources, got, err)
		}
	}
	if catalogue.Rows() != fileRows {
		test.Errorf("%d rows", catalogue.Rows())
	}
}

// the property: Find is exactly the rows whose source is among those
// asked, in the file's order, whichever rows and whichever asked.
func TestFindIsEveryRowWhoseSourceIsAsked(test *testing.T) {
	rapid.Check(test, findsEveryRow)
}

// findsEveryRow is the property for one draw of rows and asked sources.
func findsEveryRow(draw *rapid.T) {
	sources := rapid.SliceOfN(rapid.Int64Range(oneRow, sourcePool), oneRow,
		mostRows).Draw(draw, "sources")
	asked := rapid.SliceOfDistinct(rapid.Int64Range(oneRow, sourcePool),
		rapid.ID[int64]).Draw(draw, "asked")
	stars := make([]madeUp, len(sources))
	var want []string
	for at, source := range sources {
		stars[at] = theStar()
		stars[at].gaia = source
		stars[at].id = strconv.Itoa(at)
		if slices.Contains(asked, source) {
			want = append(want, stars[at].id)
		}
	}
	found, err := ofStars(draw, stars...).Find(asked...)
	if err != nil || !slices.Equal(idsOf(found), want) {
		draw.Fatalf("Find(%v) over %v: %v, %v", asked, sources, idsOf(found),
			err)
	}
}
