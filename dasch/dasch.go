// Package dasch finds the light curves held by the archive at a place
// on the sky: Harvard's Digital Access to a Sky Century @ Harvard, the
// photographic plates of the Harvard College Observatory, 1880s to 1989,
// scanned and measured; its final release is DR7.
//
// DR7 serves a light curve only through the Starglass web API, which
// takes a POST with a JSON body, not an address a GET opens. Its public
// tier asks for no key.
//
// The API was read on 2026-11-08 from the archive's page on the DR7 web APIs,
// its page on the data products, Starglass's page on authentication, and
// daschlab's source (apiclient.py, lightcurves.py, refcat.py), which is
// Harvard's own client for it.
//
// A search is two questions. The first, querycat, is a box search of the
// preferred reference catalogue, for the sources round
// a place; each source has two numbers, ref_number and gsc_bin_index.
//
// The second, lightcurve, takes those and answers with one source's raw
// light curve: every measure of it, flagged or not. Find asks the first
// and names a light curve of each source in the circle. Each is a
// product whose address Lightcurves, a web, answers with the second.
//
// The archive is asked only by name.
package dasch

import (
	"cmp"
	"errors"
	"fmt"
	"math"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"unicode"

	"github.com/janearc/celestial-data"
)

// adapterName is the archive's name, as the command line takes it.
const adapterName = "dasch"

// refcat is the reference catalogue the sources are searched in. The
// archive's pages name the preferred catalogue and an alternative.
const refcat = "apass"

// The columns of the catalogue search that are kept, as the API names
// them in the first line of its answer.
const (
	columnNumber = "ref_number"
	columnBin    = "gsc_bin_index"
	columnRA     = "ra_deg"
	columnDec    = "dec_deg"
)

// What the sky is measured in. All are defined.
const (
	arcseconds = 3600 // defined: how many arcseconds make a degree
	// widest is the longest radius, in arcseconds, a box search is
	// asked: the API refuses a larger one. Read 2026-11-08.
	widest   = 3600
	halfTurn = 180 // defined: half a circle, in degrees
	// twice is defined: the haversine halves each angle, doubles the arc.
	twice = 2
	least = 1 // defined: the fewest sources a search asks to name
)

// Archive is the archive's DR7 light curves.
type Archive struct {
	base   *celestialdata.Adapter
	asker  *asker
	most   int
	refcat string
}

// New is DR7's light curves, asked through doer, naming itself to the
// archive by agent, and naming at most so many sources a circle, the
// nearest first. It refuses no doer, no agent, and fewer than one source.
func New(doer Doer, agent string, most int) (*Archive, error) {
	return build(adapterName, doer, agent, most)
}

// build is an archive of a name. The name is New's, a constant; a test
// gives another to see the refusal of a name that is not one word.
func build(name string, doer Doer, agent string, most int) (*Archive,
	error) {
	base, err := celestialdata.Named(name)
	if err != nil {
		return nil, err
	}
	if doer == nil || agent == "" || most < least {
		return nil, base.Refuse("needs a doer, an agent and "+
			"at least %d source, not %v, %q and %d", least, doer,
			agent, most)
	}
	return &Archive{base: base, most: most, refcat: refcat,
		asker: newAsker(doer, agent)}, nil
}

// Name is the adapter's name as the command line takes it.
func (archive *Archive) Name() string { return archive.base.Name() }

// ByNameOnly says the archive is asked only when it is named.
func (*Archive) ByNameOnly() bool { return true }

// source is a source of the catalogue: its two numbers as the API wants
// them back, its place, and how far that is from the circle's centre.
type source struct {
	number     uint64
	bin        uint32
	ra, dec    float64
	separation float64
}

// Find is the light curve of each of the nearest sources of the circle, at
// most so many, nearest first. The API's search is of a box; a source in
// the box's corner, beyond the circle, is left out. A circle wider than
// the API allows is refused before anything is asked.
func (archive *Archive) Find(near *celestialdata.Near) (
	[]*celestialdata.Product, error) {
	radius := near.Radius() * arcseconds
	if radius > widest {
		return nil, archive.base.Refuse("asks a box of at most %d "+
			"arcseconds, not %v", widest, radius)
	}
	centre := near.Centre()
	box := catalogue{Refcat: archive.refcat, RA: centre.RA(),
		Dec: centre.Dec(), Radius: radius}
	listed, err := archive.asker.post(queryPath, box)
	if err != nil {
		return nil, archive.base.Wrap(err)
	}
	sources, err := within(listed, near)
	if err != nil {
		return nil, archive.base.Wrap(err)
	}
	// the circle's count is said in each product, so sources past most
	// are left out in the open
	inCircle := len(sources)
	if len(sources) > archive.most {
		sources = sources[:archive.most]
	}
	var products []*celestialdata.Product
	for place, each := range sources {
		product, err := archive.product(each, place+least, inCircle)
		if err != nil {
			return nil, archive.base.Wrap(err)
		}
		products = append(products, product)
	}
	return products, nil
}

// product is a source's light curve, as a product: the nearest but so
// many of the sources in the circle, whose count it says.
func (archive *Archive) product(each source, nearest, inCircle int) (
	*celestialdata.Product, error) {
	id := fmt.Sprintf("%s-%d", archive.refcat, each.number)
	name := fmt.Sprintf("dasch-dr7-%s-lightcurve.csv", id)
	about := fmt.Sprintf("the DASCH DR7 light curve of %s source %d at "+
		"ra %.5f, dec %.5f, %.1f "+
		"arcseconds from the centre, "+
		"number %d "+
		"of the %d sources in the circle, every measure flagged or not",
		archive.refcat, each.number, each.ra, each.dec,
		each.separation*arcseconds, nearest, inCircle)
	return archive.base.Product(id, name, address(archive.refcat, each),
		celestialdata.Unsized, about)
}

// address is where a source's light curve is kept. It is no address a GET
// opens: it holds the question, and Lightcurves asks it as a POST.
func address(which string, each source) string {
	number := strconv.FormatUint(each.number, decimal)
	bin := strconv.FormatUint(uint64(each.bin), decimal)
	asked := url.Values{"refcat": {which}, "ref_number": {number},
		"gsc_bin_index": {bin}}
	return root + curvePath + "?" + asked.Encode()
}

// decimal is the base numbers are written in. It's defined.
const decimal = 10

// within is the sources a catalogue search listed that are in the circle,
// the nearest first. The first line names the columns, in either
// spelling (see snake).
func within(listed []string, near *celestialdata.Near) ([]source, error) {
	names := split(listed[first])
	at, err := columnsAt(names)
	if err != nil {
		return nil, err
	}
	var missing []string
	for _, name := range []string{columnNumber, columnBin, columnRA,
		columnDec} {
		if _, there := at[name]; !there {
			missing = append(missing, name)
		}
	}
	if len(missing) > none {
		return nil, fmt.Errorf("the search answered with no %v; its "+
			"columns are %v", missing, names)
	}
	centre := near.Centre()
	var sources []source
	seen := map[uint64]bool{}
	for _, line := range listed[first+header:] {
		each, err := read(split(line), at, len(names))
		if err != nil {
			return nil, fmt.Errorf("the line %q: %w", line, err)
		}
		// two sources of one number would be two products of one name
		if seen[each.number] {
			return nil, fmt.Errorf("the source %d is listed twice",
				each.number)
		}
		seen[each.number] = true
		each.separation = apart(centre.RA(), centre.Dec(), each.ra,
			each.dec)
		if each.separation <= near.Radius() {
			sources = append(sources, *each)
		}
	}
	slices.SortStableFunc(sources, func(one, other source) int {
		return cmp.Compare(one.separation, other.separation)
	})
	return sources, nil
}

// first is where a list begins. It's defined.
const first = 0

// header is how many lines an answer's names of its columns take, before
// its rows: one, the first. It's defined.
const header = 1

// columnsAt is where each column of a first line is, by its name in
// snake case. It refuses a line that names one column twice, in one
// spelling or both: which of the two to read is not the adapter's to
// choose.
func columnsAt(names []string) (map[string]int, error) {
	at := make(map[string]int)
	for place, name := range names {
		named := snake(name)
		if _, twice := at[named]; twice {
			return nil, fmt.Errorf(
				"the answer names the column %s "+
					"twice; its columns are %v",
				named, names)
		}
		at[named] = place
	}
	return at, nil
}

// snake is a column's name in snake case, as the adapter reads it. Both
// spellings are read: refNumber is ref_number, dateJd is date_jd, and a
// name in snake case is itself.
//
// The archive's web APIs page, kept on 2026-11-09, says the raw API names its
// columns in camel case (wordsLikeThis); its column pages use snake case
// (words_like_this), and so did the API's answers kept on 2026-11-08.
//
// A word begins where a capital follows a small letter or a digit, so a
// run of capitals is one word: dateJD is date_jd too, not date_j_d.
func snake(name string) string {
	var named strings.Builder
	previous := rune(first)
	for _, letter := range name {
		if unicode.IsUpper(letter) {
			if unicode.IsLower(previous) ||
				unicode.IsDigit(previous) {
				named.WriteByte('_')
			}
			previous = letter
			letter = unicode.ToLower(letter)
		} else {
			previous = letter
		}
		named.WriteRune(letter)
	}
	return named.String()
}

// read is a source from a line's fields, where at says in which field
// each column is. width is how many fields a line has. A line with
// another number, or a field that is no number, is an error and no
// source.
func read(fields []string, at map[string]int, width int) (*source,
	error) {
	if len(fields) != width {
		return nil, fmt.Errorf("%d fields, not the %d of the "+
			"columns", len(fields), width)
	}
	number, numberErr := strconv.ParseUint(fields[at[columnNumber]],
		decimal, bits64)
	bin, binErr := strconv.ParseUint(fields[at[columnBin]], decimal,
		bits32)
	ra, raErr := strconv.ParseFloat(fields[at[columnRA]], bits64)
	dec, decErr := strconv.ParseFloat(fields[at[columnDec]], bits64)
	if err := errors.Join(numberErr, binErr, raErr, decErr); err != nil {
		return nil, err
	}
	// a place that is not on the sky, NaN or past a pole or a turn, is
	// an error in the answer, not a source too far to keep
	if _, err := celestialdata.NewPosition(ra, dec); err != nil {
		return nil, err
	}
	return &source{number: number, bin: uint32(bin), ra: ra, dec: dec}, nil
}

// The widths numbers are read at. They're defined by strconv.
const (
	bits32 = 32 // defined by strconv: a number that fits 32 bits
	bits64 = 64 // defined by strconv: a number that fits 64 bits
)

// apart is the angle between two places, in degrees, by the haversine
// formula, which keeps its accuracy for places close together.
func apart(ra, dec, otherRA, otherDec float64) float64 {
	rad := math.Pi / halfTurn
	dDec := (otherDec - dec) * rad
	dRA := (otherRA - ra) * rad
	half := math.Sin(dDec/twice)*math.Sin(dDec/twice) +
		math.Cos(dec*rad)*math.Cos(otherDec*rad)*
			math.Sin(dRA/twice)*math.Sin(dRA/twice)
	return twice * math.Asin(math.Sqrt(half)) / rad
}
