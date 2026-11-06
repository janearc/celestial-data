package galah_test

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
	"github.com/janearc/celestial-data/galah"
	"github.com/janearc/celestial-data/internal/fitstest"
)

// The made-up star the tests write and read back, each value defined
// here and distinct from every other, so that two columns swapped in
// the reader are seen. The numbers are sums of halves, so a single
// holds them exactly.
const (
	madeSObject     = 131216001101012345        // defined: a sobject_id's form
	madeTwoMASS     = "17455519-2900281"        // defined: a tmass_id's form
	madeGaia        = 4_037_334_576_111_139_712 // defined: past 2^53
	madeRA          = 266.4                     // defined: degrees
	madeDec         = -29.0                     // defined: degrees
	madeSpectral    = 1<<13 | 1<<2              // defined: flag_sp's extrapolated and single-lined binary bits
	madeSNR         = 61.5                      // defined: per pixel, the third camera
	madeRadial      = -423.25                   // defined: km/s
	madeRadialErr   = 0.125                     // defined: km/s
	madeTeff        = 4321.5                    // defined: kelvin
	madeTeffErr     = 81.25                     // defined: kelvin
	madeLogg        = 1.375                     // defined: log g, cgs
	madeLoggErr     = 0.0625                    // defined: dex
	madeIron        = -0.46875                  // defined: dex
	madeIronErr     = 0.01171875                // defined: dex
	madeIronFlag    = 16                        // defined: flag_fe_h's below-the-limit value
	madeVsini       = 12.75                     // defined: km/s
	madeVsiniErr    = 1.5                       // defined: km/s
	madeLithium     = 2.3125                    // defined: A(Li), dex
	madeLithiumLow  = 0.09375                   // defined: dex below
	madeLithiumHigh = 0.109375                  // defined: dex above
	madeLithiumFlag = 2                         // defined: flag_a_li with its second bit, a value not a limit
	madeLimit       = 1.5                       // defined: an upper limit of A(Li), dex
	elementBase     = 0.25                      // defined: the first element's [X/Fe]
	elementStep     = 0.03125                   // defined: each next element's step up
	uncertainBase   = 0.5                       // defined: the first element's uncertainty
	uncertainStep   = 0.0078125                 // defined: each next one's step up
	flagBase        = 1000                      // defined: the first element's flag, each next one up
	pastThePole     = 91                        // defined: degrees, past the north pole
)

// The made-up file's layout and the tests' other numbers, each defined.
const (
	twoMASSWidth = 17            // defined: tmass_id's characters
	unread       = 3             // defined: the bytes of an unread column, 3A
	longNull     = math.MinInt64 // defined: the file's TNULL on its longs
	wholeNull    = math.MinInt32 // defined: the file's TNULL on its wholes
	fileRows     = 3             // defined: the Find file's rows
	oneRow       = 1             // defined: a file of one
	firstRow     = 0             // defined: the first row
	otherGaia    = 77            // defined: another source
	absentGaia   = 78            // defined: a source no row holds
	mostRows     = 12            // defined: the property's most rows
	sourcePool   = 5             // defined: the sources the property's rows draw from
	cutBlock     = 2880          // defined: a FITS block's bytes, the file cut by one
	cutRow       = "row 0: EOF"  // defined: the words of a row the file cannot give
	everyBit     = -1            // defined: a mask with every bit set
	lowest       = 1             // defined: a mask of the lowest bit, before its shift
	topBit       = 63            // defined: a mask's last bit
	pastTheMask  = 64            // defined: a bit past a 64-bit mask
	madeElements = 30            // defined: the header's element columns, read 2026-11-06
	lastSymbol   = "Eu"          // defined: the header's last element
	firstID      = 1             // defined: the Find file's first survey id
	secondID     = 2             // defined: its second
	thirdID      = 3             // defined: its third
)

// madeUp is a star as the file writes it, by the values the tests
// change.
type madeUp struct {
	sobject, gaia, liFlag int64
	ra, dec               float64
	lithium, limit        float64
	flag                  int32
}

// theStar is the made-up star with every value defined above, its
// lithium measured and so with no upper limit.
func theStar() madeUp {
	return madeUp{sobject: madeSObject, gaia: madeGaia, ra: madeRA,
		dec: madeDec, flag: madeSpectral, liFlag: madeLithiumFlag,
		lithium: madeLithium, limit: math.NaN()}
}

// layout is the made-up file's columns in the order the real file
// writes them, one it does not read among them, its whole numbers
// nulled as the real file nulls them.
func layout() []fitstest.Column {
	columns := []fitstest.Column{
		fitstest.NewColumn("sobject_id", "K").Nulled(longNull),
		fitstest.NewColumn("tmass_id", fmt.Sprintf("%dA", twoMASSWidth)),
		fitstest.NewColumn("gaiadr3_source_id", "K").Nulled(longNull),
		fitstest.NewColumn("setup", fmt.Sprintf("%dA", unread)),
		fitstest.NewColumn("ra", "D"), fitstest.NewColumn("dec", "D"),
		fitstest.NewColumn("flag_sp", "J").Nulled(wholeNull),
		fitstest.NewColumn("snr_px_ccd3", "E"),
		fitstest.NewColumn("rv_comp_1", "E"), fitstest.NewColumn("e_rv_comp_1", "E"),
		fitstest.NewColumn("teff", "E"), fitstest.NewColumn("e_teff", "E"),
		fitstest.NewColumn("logg", "E"), fitstest.NewColumn("e_logg", "E"),
		fitstest.NewColumn("fe_h", "E"), fitstest.NewColumn("e_fe_h", "E"),
		fitstest.NewColumn("flag_fe_h", "J").Nulled(wholeNull),
		fitstest.NewColumn("vsini", "E"), fitstest.NewColumn("e_vsini", "E")}
	for _, symbol := range galah.Elements() {
		stem := strings.ToLower(symbol) + "_fe"
		columns = append(columns, fitstest.NewColumn(stem, "E"),
			fitstest.NewColumn("e_"+stem, "E"),
			fitstest.NewColumn("flag_"+stem, "J").Nulled(wholeNull))
	}
	return append(columns, fitstest.NewColumn("a_li", "E"),
		fitstest.NewColumn("a_li_upp_lim", "E"),
		fitstest.NewColumn("e_a_li_low", "E"),
		fitstest.NewColumn("e_a_li_upp", "E"),
		fitstest.NewColumn("flag_a_li", "K").Nulled(longNull))
}

// written is a star's row, in the layout's order.
func written(star madeUp) *fitstest.Row {
	row := fitstest.NewRow()
	row.Long(star.sobject).Text(madeTwoMASS, twoMASSWidth)
	row.Long(star.gaia).Text("", unread).Double(star.ra).Double(star.dec)
	row.Int(star.flag).Single(madeSNR).Single(madeRadial)
	row.Single(madeRadialErr).Single(madeTeff).Single(madeTeffErr)
	row.Single(madeLogg).Single(madeLoggErr).Single(madeIron)
	row.Single(madeIronErr).Int(madeIronFlag).Single(madeVsini)
	row.Single(madeVsiniErr)
	for at := range galah.Elements() {
		row.Single(elementValue(at)).Single(elementUncertainty(at))
		row.Int(int32(flagBase + at))
	}
	row.Single(float32(star.lithium)).Single(float32(star.limit))
	row.Single(madeLithiumLow).Single(madeLithiumHigh)
	return row.Long(star.liFlag)
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

// stopping is what the helpers stop a test or a property through.
type stopping interface {
	Helper()
	Fatal(args ...any)
}

// opened is the catalogue of a file's bytes, or the test stops.
func opened(test stopping, file []byte) *galah.Catalogue {
	test.Helper()
	catalogue, err := galah.OpenCatalogue(bytes.NewReader(file))
	if err != nil {
		test.Fatal(err)
	}
	return catalogue
}

// ofStars is the catalogue of a file of the stars' rows, or the test
// stops.
func ofStars(test stopping, stars ...madeUp) *galah.Catalogue {
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
func theRow(test *testing.T) *galah.Star {
	test.Helper()
	catalogue := ofStars(test, theStar())
	star, err := catalogue.Star(firstRow)
	if err != nil {
		test.Fatal(err)
	}
	return star
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
// in its place, the column not read stepped over.
func TestAStarIsReadBackAsItWasWritten(test *testing.T) {
	star := theRow(test)
	gaia, inGaia := star.GaiaID()
	if star.SObjectID() != madeSObject || star.TwoMASS() != madeTwoMASS ||
		gaia != madeGaia || !inGaia {
		test.Errorf("%d %q %d %v", star.SObjectID(), star.TwoMASS(), gaia,
			inGaia)
	}
	place := star.Place()
	if place == nil {
		test.Fatal("the made-up star has no place")
	}
	ra, dec := place.Ascension().InDegrees(), place.Declination().InDegrees()
	if ra != madeRA || dec != madeDec || star.SNR() != madeSNR ||
		star.Spectroscopic() != madeSpectral {
		test.Errorf("ra %g dec %g snr %g flag_sp %d", ra, dec, star.SNR(),
			star.Spectroscopic())
	}
	if star.RadialVelocity() != madeRadial ||
		star.RadialError() != madeRadialErr {
		test.Errorf("rv %g error %g", star.RadialVelocity(), star.RadialError())
	}
	pairs := []fitted{
		fittedOf("temperature", star.Temperature, madeTeff, madeTeffErr),
		fittedOf("gravity", star.Gravity, madeLogg, madeLoggErr),
		fittedOf("rotation", star.Rotation, madeVsini, madeVsiniErr),
		fittedOf("lithium's range", star.Lithium().Uncertainty,
			madeLithiumLow, madeLithiumHigh)}
	for _, pair := range pairs {
		if pair.value != pair.wrote || pair.uncertainty != pair.wroteUncertainty {
			test.Errorf("%s read %g, %g, wrote %g, %g", pair.name, pair.value,
				pair.uncertainty, pair.wrote, pair.wroteUncertainty)
		}
	}
	iron, lithium := star.Iron(), star.Lithium()
	if iron.Value() != madeIron || iron.Uncertainty() != madeIronErr ||
		iron.Flags() != madeIronFlag {
		test.Errorf("iron %g +- %g flags %d", iron.Value(), iron.Uncertainty(),
			iron.Flags())
	}
	flags, flagged := lithium.Flags()
	if lithium.Value() != madeLithium || !math.IsNaN(lithium.Limit()) ||
		flags != madeLithiumFlag || !flagged || flags.Has(galah.UpperLimit) {
		test.Errorf("lithium %g limit %g flags %d, %v", lithium.Value(),
			lithium.Limit(), flags, flagged)
	}
}

// the test: lithium the survey found only an upper limit for reads as
// that limit, with no value, its flag saying UpperLimit.
func TestLithiumMayBeAnUpperLimit(test *testing.T) {
	bounded := theStar()
	bounded.lithium, bounded.limit = math.NaN(), madeLimit
	bounded.liFlag = lowest << galah.UpperLimit
	star, err := ofStars(test, bounded).Star(firstRow)
	if err != nil {
		test.Fatal(err)
	}
	lithium := star.Lithium()
	flags, flagged := lithium.Flags()
	if !math.IsNaN(lithium.Value()) || lithium.Limit() != madeLimit ||
		!flagged || !flags.Has(galah.UpperLimit) {
		test.Errorf("lithium %g limit %g flags %d, %v", lithium.Value(),
			lithium.Limit(), flags, flagged)
	}
}

// the test: each element's abundance is read from its own three
// columns, by its symbol; the file's thirty, carbon to europium.
func TestEachElementIsReadInItsPlace(test *testing.T) {
	star := theRow(test)
	symbols := galah.Elements()
	if len(symbols) != madeElements || symbols[len(symbols)-oneRow] != lastSymbol {
		test.Errorf("elements %v", symbols)
	}
	for at, symbol := range symbols {
		abundance, err := star.Abundance(symbol)
		if err != nil {
			test.Fatal(err)
		}
		if abundance.Value() != float64(elementValue(at)) ||
			abundance.Uncertainty() != float64(elementUncertainty(at)) ||
			abundance.Flags() != galah.Flags(flagBase+at) {
			test.Errorf("%s: %g +- %g flags %d", symbol, abundance.Value(),
				abundance.Uncertainty(), abundance.Flags())
		}
	}
	changed := galah.Elements()
	changed[firstRow] = "Zz"
	if galah.Elements()[firstRow] == "Zz" {
		test.Error("a change to Elements is kept")
	}
}

// The bits' numbers as the survey's flags page gives their values, two
// to the number, read 2026-11-06, each defined there.
const (
	pageEmission   = 0  // defined: flag_sp value 1
	pageSingle     = 2  // defined: flag_sp value 4
	pageDouble     = 3  // defined: flag_sp value 8
	pageTriple     = 7  // defined: flag_sp value 128
	pageIronWarn   = 10 // defined: flag_sp value 1024
	pageLowSNR     = 11 // defined: flag_sp value 2048
	pageNoConverge = 12 // defined: flag_sp value 4096
	pageExtrapol   = 13 // defined: flag_sp value 8192
	pageNoResults  = 14 // defined: flag_sp value 16384
	pageUpperLimit = 0  // defined: flag_X_fe value 1
	pageNotDetect  = 1  // defined: flag_X_fe value 2
	pageFitFailed  = 2  // defined: flag_X_fe value 4
)

// pageBit is a named bit and its number by the survey's flags page.
type pageBit struct {
	named, page uint
}

// pageBits are the bits the package names, each beside the page's.
func pageBits() map[string]pageBit {
	return map[string]pageBit{
		"emission lines": {galah.EmissionLines, pageEmission},
		"single-lined":   {galah.SingleLinedBinary, pageSingle},
		"double-lined":   {galah.DoubleLinedBinary, pageDouble},
		"triple":         {galah.TripleStar, pageTriple},
		"iron warning":   {galah.IronWarn, pageIronWarn},
		"low S/N":        {galah.LowSNR, pageLowSNR},
		"not converged":  {galah.NotConverged, pageNoConverge},
		"extrapolated":   {galah.Extrapolated, pageExtrapol},
		"no results":     {galah.NoResults, pageNoResults},
		"upper limit":    {galah.UpperLimit, pageUpperLimit},
		"not detectable": {galah.NotDetectable, pageNotDetect},
		"fit failed":     {galah.FitFailed, pageFitFailed}}
}

// the test: each named bit is the survey's: a mask of the page's bit
// alone has it, and a mask of every other bit has it not; no bit past
// the mask's 64 is set.
func TestTheNamedBitsAreTheSurveys(test *testing.T) {
	for name, bit := range pageBits() {
		alone := galah.Flags(lowest << bit.page)
		others := galah.Flags(everyBit) &^ alone
		if !alone.Has(bit.named) || others.Has(bit.named) {
			test.Errorf("%s is bit %d, not the page's %d", name, bit.named,
				bit.page)
		}
	}
	every := galah.Flags(everyBit)
	if !every.Has(topBit) || every.Has(pastTheMask) {
		test.Error("a bit past the mask is set")
	}
}

// the anti-test: an abundance of an element the file holds none of
// against iron is refused, saying which: iron and lithium are the
// star's own, and symbols are as Elements spells them.
func TestAnAbundanceTheFileLacksIsRefused(test *testing.T) {
	star := theRow(test)
	for _, symbol := range []string{"Fe", "Li", "mg", ""} {
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
		"a double teff": {retyped("teff", "D", ""), "teff is of kind D, not E"},
		"two teffs a row": {retyped("teff", "2E", ""),
			"teff holds 2 values a row, not one"},
		"teff in kelvin": {retyped("teff", "E", "K"),
			`teff is in "K", where the allstar file writes no unit`},
		"a long element flag": {retyped("flag_eu_fe", "K", ""),
			"flag_eu_fe is of kind K, not J"},
		"a double element error": {retyped("e_c_fe", "D", ""),
			"e_c_fe is of kind D, not E"},
		"a long element": {retyped("mg_fe", "K", ""), "mg_fe is of kind K, not E"}}
	for _, each := range layout() {
		if each.Name() == "setup" {
			continue
		}
		broken["no "+each.Name()] = breakage{without(each.Name()),
			"the allstar file has no " + each.Name()}
	}
	return broken
}

// the anti-test: a file the reader cannot read is refused, saying
// why: one that is not a FITS file, and each of the breakages.
func TestAFileNotReadIsRefused(test *testing.T) {
	catalogue, err := galah.OpenCatalogue(strings.NewReader("not a file"))
	if catalogue != nil || !errors.Is(err, celestialdata.Refused()) ||
		!strings.Contains(err.Error(), "the allstar file") {
		test.Errorf("not a file: %v", err)
	}
	for why, broken := range breakages() {
		file := fitstest.File(broken.columns)
		catalogue, err := galah.OpenCatalogue(bytes.NewReader(file))
		if catalogue != nil || !errors.Is(err, celestialdata.Refused()) ||
			!strings.Contains(err.Error(), broken.said) {
			test.Errorf("%s: %v", why, err)
		}
	}
}

// the anti-test: a row the file does not have, or cannot give, is
// refused, by Star and by Find, saying which row; a star past the pole
// is refused by its survey id, by Star and by Find.
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
	id := strconv.Itoa(madeSObject)
	star, err = beyond.Star(firstRow)
	if star != nil || !errors.Is(err, celestialdata.Refused()) ||
		!strings.Contains(err.Error(), id) {
		test.Errorf("a star past the pole, by Star: %v", err)
	}
	found, err = beyond.Find(madeGaia)
	if found != nil || !errors.Is(err, celestialdata.Refused()) ||
		!strings.Contains(err.Error(), id) {
		test.Errorf("a star past the pole, by Find: %v", err)
	}
}

// the test: a row whose Gaia source or lithium flag the survey left
// empty has none, and Find never gives a row with no source; a row whose
// place it left empty, both numbers, has none. The anti-test: a survey
// id or another flag left empty is refused, saying which, and so is one
// empty number of a place alone.
func TestWhatTheSurveyLeftEmpty(test *testing.T) {
	unknown := theStar()
	unknown.gaia, unknown.liFlag = longNull, longNull
	catalogue := ofStars(test, unknown)
	star, err := catalogue.Star(firstRow)
	if err != nil {
		test.Fatal(err)
	}
	if _, inGaia := star.GaiaID(); inGaia {
		test.Error("a row with no source has one")
	}
	if _, flagged := star.Lithium().Flags(); flagged {
		test.Error("a row with no lithium flag has one")
	}
	if found, err := catalogue.Find(longNull); found != nil || err != nil {
		test.Errorf("Find of no source: %v, %v", found, err)
	}
	nowhere := theStar()
	nowhere.ra, nowhere.dec = math.NaN(), math.NaN()
	star, err = ofStars(test, nowhere).Star(firstRow)
	if err != nil || star.Place() != nil {
		test.Errorf("a row with no place: %v, %v", star, err)
	}
	sobject, flag, dec := theStar(), theStar(), theStar()
	sobject.sobject, flag.flag, dec.dec = longNull, wholeNull, math.NaN()
	emptied := map[string]madeUp{"the survey left sobject_id empty": sobject,
		"the survey left flag_sp empty": flag, strconv.Itoa(madeSObject): dec}
	for said, row := range emptied {
		star, err := ofStars(test, row).Star(firstRow)
		if star != nil || !errors.Is(err, celestialdata.Refused()) ||
			!strings.Contains(err.Error(), said) {
			test.Errorf("refused saying %v, not %q", err, said)
		}
	}
}

// sobjectsOf are the survey ids of the stars found.
func sobjectsOf(stars []*galah.Star) []int64 {
	ids := make([]int64, len(stars))
	for at, star := range stars {
		ids[at] = star.SObjectID()
	}
	return ids
}

// the test: Find gives every row of the sources asked, in the file's
// order; none for a source no row holds, or for none asked.
func TestFindGivesEveryRowOfTheSources(test *testing.T) {
	one, other, again := theStar(), theStar(), theStar()
	one.sobject, other.sobject, again.sobject = firstID, secondID, thirdID
	other.gaia = otherGaia
	catalogue := ofStars(test, one, other, again)
	asks := map[string][]int64{"[1 3]": {madeGaia},
		"[1 2 3]": {otherGaia, madeGaia}, "[]": {absentGaia}, "[] ": {}}
	for want, sources := range asks {
		found, err := catalogue.Find(sources...)
		got := fmt.Sprint(sobjectsOf(found))
		if err != nil || got != strings.TrimSpace(want) {
			test.Errorf("Find(%v): %s, %v", sources, got, err)
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
	var want []int64
	for at, source := range sources {
		stars[at] = theStar()
		stars[at].gaia = source
		stars[at].sobject = int64(at)
		if slices.Contains(asked, source) {
			want = append(want, stars[at].sobject)
		}
	}
	found, err := ofStars(draw, stars...).Find(asked...)
	got := sobjectsOf(found)
	if err != nil || !slices.Equal(got, want) {
		draw.Fatalf("Find(%v) over %v: %v, %v", asked, sources, got, err)
	}
}
