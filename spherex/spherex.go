// Package spherex finds the images NASA's SPHEREx mission has taken of a
// place on the sky, as IRSA holds them, and names a cutout of each: a
// small multi-extension FITS file round the circle's centre.
//
// The cutout has the image's extensions of pixels, flags, variance,
// zodiacal light and wavelength cut to the place, and its point spread
// function whole.
//
// The mission's linear variable filters put a star at a different
// wavelength in each image, so the cutouts of one place, taken together,
// hold its spectrum between 0.75 and 5 micron. Making the spectrum is
// not done here; this package finds and fetches.
//
// The images are asked of IRSA's TAP service, which lists them in its
// spherex.artifact and spherex.plane tables. The cutouts are cut by
// IRSA's image server from the address of the whole image.
//
// The tables, the layout of the files and the cutout form were read on
// 2026-11-06 (sources.yaml).
//
// IRSA asks that work using SPHEREx data say so, in the words the
// readme gives, and cite the release's DOI: 10.26131/IRSA652 for QR2
// and 10.26131/IRSA662 for QR3.
//
// IRSA's pages state no limit on the rate of requests. Find asks once,
// and the command fetches one cutout at a time.
//
// A cutout is some 4 to 5 MB, nearly all of it the point spread
// function, which the image server does not cut. The server says no
// length for a cutout.
//
// So a product here is held, by Fetch, to being a whole cutout: a FITS
// file all the way through, of the primary, the image, its flags,
// variance and zodiacal light of its size, the release's PSF and the
// wavelength table, in that order and no more.
//
// A file cut short, even where a unit ends, or not FITS, is not kept.
//
// The archive is asked only by name.
package spherex

import (
	_ "embed"
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"time"

	"github.com/janearc/celestial-data"
	"github.com/janearc/celestial-data/tap"
	"github.com/janearc/celestial-data/web"
)

// adapterName is the archive's name, as the command line takes it.
const adapterName = "spherex"

// IRSA's places: its TAP service, and the root the images' paths are
// relative to. A path begins ibe/data/spherex/, where the image server
// serves them and cuts them.
const (
	sync = "https://irsa.ipac.caltech.edu/TAP/sync"
	root = "https://irsa.ipac.caltech.edu/"
)

// What IRSA's tables are asked. Each image is a row of the artifact table
// joined to its plane, which holds its footprint, wavelength range and
// time. The rows are in the order they were taken, and the file's path
// breaks a tie: the six detectors are exposed together.
const (
	columns = "a.uri, p.energy_bounds_lower, p.energy_bounds_upper, " +
		"p.time_bounds_lower, p.provenance_version"
	tables = "spherex.artifact a JOIN spherex.plane p ON " +
		"a.planeid = p.planeid"
	order = "p.time_bounds_lower, a.uri"
)

// The columns of the answer, and how its numbers are read.
const (
	colPath    = "uri"
	colLow     = "energy_bounds_lower"
	colHigh    = "energy_bounds_upper"
	colTime    = "time_bounds_lower"
	colVersion = "provenance_version"
	// micron is a micron in metres; the table gives the wavelengths in
	// metres. It's defined.
	micron = 1e-6
	// unixMJD is the Modified Julian Date of 1970-01-01. It's defined.
	unixMJD = 40587
	// daySeconds is the seconds in a day. It's defined.
	daySeconds = 86400
	// twice is how many radii a square's side is: the square holds the
	// circle. It's defined.
	twice = 2
	// plain is strconv's format of a number with no exponent, as ADQL
	// is given one. It's defined by strconv.
	plain = 'f'
	// shortest is strconv's precision for the fewest digits that read
	// back. It's defined by strconv.
	shortest = -1
	// bits is the size of a float64. It's defined.
	bits = 64
	// noNanos is the nanoseconds a moment is made with. It's defined.
	noNanos = 0
)

// numbersFile is the numbers file the sizes are read from.
const numbersFile = "numbers.yaml"

//go:embed numbers.yaml
var numbersRaw []byte

// The names of the numbers in numbers.yaml.
const (
	leastSide  = "least-side"
	mostSide   = "most-side"
	skyPlaces  = "ra-dec-places"
	sidePlaces = "side-places"
)

// least is the fewest images a search may ask for. It's defined: a search
// that asks for none finds nothing.
const least = 1

// imagePath is where an image is kept, as the artifact table gives it:
// the release, the planning period and the processing under it, the
// detector, and the file, which names the observation, the detector and
// the processing again.
//
// The two detectors, and the two processings, must agree. Every part is
// defined by IRSA's browsable directories, as the user guide's data
// access page describes them.
//
// A processing may be a rerun, named so: l2b_retry-v27-2026-261 was
// among the images over a star on 2026-11-06.
var imagePath = regexp.MustCompile(`^ibe/data/spherex/(qr[0-9]+)/level2/` +
	`[0-9A-Za-z_]+/(l2b[a-z_]*-[0-9a-z-]+)/([1-6])/level2_` +
	`([0-9A-Za-z_]+)D([1-6])_spx_(l2b[a-z_]*-[0-9a-z-]+)\.fits$`)

// afterWhole is where the parts of a match count from: submatch zero is
// the whole path. It's defined by regexp.
const afterWhole = 1

// The parts of a path that imagePath captures.
const (
	partRelease = iota + afterWhole
	partDirVersion
	partDirDetector
	partObservation
	partNameDetector
	partNameVersion
	parts
)

// A product's file is held to being a whole cutout by this check.
var _ celestialdata.Checker = wholeCutout{}

// Archive is the SPHEREx images IRSA holds.
type Archive struct {
	base *celestialdata.Adapter
	most int
	// least and greatest are the shortest and longest side asked, in
	// degrees; skyDecimals and sideDecimals the decimals the centre and
	// the side are written with.
	least, greatest           float64
	skyDecimals, sideDecimals int
}

// New is the SPHEREx images, asked through get for at most most images a
// search. It refuses no web and a most below one.
func New(get web.Web, most int) (*Archive, error) {
	base, err := celestialdata.NewAdapter(adapterName, get)
	if err != nil {
		return nil, err
	}
	if most < least {
		return nil, base.Refuse("needs at least one image a search, "+
			"not %d", most)
	}
	kept := celestialdata.NewMeasured(numbersFile, numbersRaw)
	return &Archive{base: base, most: most,
		least:        kept.Value(leastSide),
		greatest:     kept.Value(mostSide),
		skyDecimals:  int(kept.Value(skyPlaces)),
		sideDecimals: int(kept.Value(sidePlaces))}, nil
}

// Name is the adapter's name as the command line takes it.
func (archive *Archive) Name() string { return archive.base.Name() }

// ByNameOnly says the archive is asked only when it is named.
func (*Archive) ByNameOnly() bool { return true }

// number is a number as ADQL is given it.
func number(value float64) string {
	return strconv.FormatFloat(value, plain, shortest, bits)
}

// question is the ADQL for the images whose footprints meet the circle,
// the earliest first. INTERSECTS takes the footprint first: the other way
// round, IRSA's service did not read it.
func (archive *Archive) question(near *celestialdata.Near) string {
	centre := near.Centre()
	ra, dec := number(centre.RA()), number(centre.Dec())
	radius := number(near.Radius())
	return fmt.Sprintf("SELECT TOP %d %s FROM %s WHERE 1 = "+
		"INTERSECTS(p.poly, CIRCLE('ICRS', %s, %s, %s)) ORDER BY %s",
		archive.most, columns, tables, ra, dec, radius, order)
}

// cut is the square of sky a cutout holds, as the address and the file's
// name write it: the centre in degrees and the side in degrees.
type cut struct{ ra, dec, side string }

// cutOf is the square round the circle's centre whose side is twice the
// radius, no less than the least a side is asked at and no more than the
// most.
func (archive *Archive) cutOf(near *celestialdata.Near) cut {
	centre := near.Centre()
	asked := twice * near.Radius()
	wide := min(max(asked, archive.least), archive.greatest)
	return cut{ra: fmt.Sprintf("%.*f", archive.skyDecimals, centre.RA()),
		dec:  fmt.Sprintf("%.*f", archive.skyDecimals, centre.Dec()),
		side: fmt.Sprintf("%.*f", archive.sideDecimals, wide)}
}

// Find is a cutout of each image whose footprint meets the circle, the
// earliest first, at most as many as the archive was made to ask for.
// The cutout is centred on the circle's centre and is twice the radius
// a side, but never less nor more than numbers.yaml says.
func (archive *Archive) Find(near *celestialdata.Near) (
	[]*celestialdata.Product, error) {
	answer, err := tap.Query(archive.base.Web(), sync,
		archive.question(near))
	if err != nil {
		return nil, archive.base.Wrap(err)
	}
	square := archive.cutOf(near)
	var products []*celestialdata.Product
	var refused error
	for at := range answer.Len() {
		seen, err := sighted(answer, at)
		if err != nil {
			return nil, archive.base.Wrap(err)
		}
		made, err := celestialdata.Found(archive.cutout(seen, square))
		products = append(products, made...)
		refused = errors.Join(refused, err)
	}
	return products, refused
}

// cutout is the product of one image, held to being a whole cutout of
// its release.
func (archive *Archive) cutout(seen *sighting,
	square cut) (*celestialdata.Product, error) {
	name := fmt.Sprintf("spherex_%s_%s_%s_ra%s_dec%s_%sdeg.fits",
		seen.release, seen.id, seen.version, square.ra, square.dec,
		square.side)
	address := root + seen.path + "?center=" + square.ra + "," +
		square.dec + "&size=" + square.side
	about := fmt.Sprintf("SPHEREx %s image %s, %.2f to %.2f micron, "+
		"taken %s (MJD %.4f), pipeline %s; a cutout %s degrees square",
		seen.release, seen.id, seen.low, seen.high, seen.when(),
		seen.mjd, seen.pipeline, square.side)
	product, err := archive.base.Product(seen.id, name, address,
		celestialdata.Unsized, about)
	check := wholeCutout{psf: psfOf(seen.release)}
	return product.Checked(check), archive.base.Wrap(err)
}

// sighting is an image as IRSA's tables give it.
type sighting struct {
	path, release, id, version, pipeline string
	low, high, mjd                       float64
}

// when is the day and minute the image was taken, in UTC.
func (seen sighting) when() string {
	seconds := int64((seen.mjd - unixMJD) * daySeconds)
	moment := time.Unix(seconds, noNanos).UTC()
	return moment.Format("2006-01-02 15:04 UTC")
}

// sighted is a row of the answer. The image's id is the observation's
// and the detector's, such as 2026W37_2A_0136_2D2. A path that does not
// read as an image's, or whose parts disagree, is an error.
func sighted(answer rows, at int) (*sighting, error) {
	path, pathErr := answer.Text(at, colPath)
	pipeline, pipeErr := answer.Text(at, colVersion)
	low, lowErr := answer.Number(at, colLow)
	high, highErr := answer.Number(at, colHigh)
	mjd, mjdErr := answer.Number(at, colTime)
	if err := errors.Join(pathErr, pipeErr, lowErr, highErr,
		mjdErr); err != nil {
		return nil, err
	}
	found := imagePath.FindStringSubmatch(path)
	if len(found) != parts || found[partDirDetector] !=
		found[partNameDetector] || found[partDirVersion] !=
		found[partNameVersion] {
		return nil, fmt.Errorf("the path %q is not an image's", path)
	}
	return &sighting{path: path, release: found[partRelease],
		id:      found[partObservation] + "D" + found[partNameDetector],
		version: found[partNameVersion], pipeline: pipeline,
		low: low / micron, high: high / micron, mjd: mjd}, nil
}

// rows is what sighted reads of an answered table.
type rows interface {
	Text(at int, name string) (string, error)
	Number(at int, name string) (float64, error)
}
