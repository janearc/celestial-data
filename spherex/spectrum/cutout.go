package spectrum

import (
	"fmt"
	"io"
	"math"
	"sort"

	"github.com/janearc/celestial/fits"
	"github.com/janearc/celestial/psf"
)

// The flags a pixel is left out of the fit for, by their bits. The
// supplement lists them: "Pixels with any of the following flags will
// be ignored when fitting", section 3.3, the bits from its Table 16.
const (
	surError      = 1 << 2  // SUR_ERROR
	nonFunctional = 1 << 6  // NONFUNC
	missingData   = 1 << 9  // MISSING_DATA
	hot           = 1 << 10 // HOT
	cold          = 1 << 11 // COLD
	nonLinear     = 1 << 15 // NONLINEAR
	persistent    = 1 << 17 // PERSIST
	bloom         = 1 << 26 // BLOOM
	snowball      = 1 << 27 // SNOWBALL
	halo          = 1 << 28 // HALO
	satelliteHalo = 1 << 29 // SATELLITE_HALO
	leftOut       = surError | nonFunctional | missingData | hot | cold |
		nonLinear | persistent | bloom | snowball | halo | satelliteHalo
)

// The counts a cutout is read by. Each is defined.
const (
	planeAxes  = 2 // an ePSF's samples are a plane
	wavePair   = 2 // WCS-WAVE's values: a wavelength, then a bandwidth
	waveAt     = 0 // the wavelength's place in a pair
	bandAt     = 1 // the bandwidth's place
	firstRow   = 0 // a table's first row
	nextOne    = 1 // the next sample or grid point along an axis
	background = 1 // the background, one unknown beside the stars
)

// cutout is what a SPHEREx cutout holds that the measure reads: its
// pixels, their variances and flags, the image's time and detector,
// its sky, its EPSF table, and where it lies in its parent image.
type cutout struct {
	file              io.ReaderAt
	epsf              *fits.Table
	width, height     int
	pixels, variances []float64
	flags             []int64
	mjd               float64
	detector          int
	sky               *fits.Sky
	parentX, parentY  float64
	shape             *psf.Shape
	wave, band, omega float64
	crpix1A, crpix2A  float64
}

// read is a cutout's images and cards. It refuses a file without the
// IMAGE, VARIANCE and FLAGS extensions, and a QR2 cutout, which has no
// EPSF.
func read(file io.ReaderAt) (*cutout, error) {
	image, err := fits.OpenNamed(file, "IMAGE")
	if err != nil {
		return nil, err
	}
	epsf, err := fits.OpenTableNamed(file, "EPSF")
	if err != nil {
		return nil, fmt.Errorf("%w: no EPSF, as a QR2 cutout has "+
			"none; its PSF cube and its gain want SPHEREx's "+
			"calibration files: %w", errRefused, err)
	}
	cut := &cutout{file: file, epsf: epsf, width: image.Width(),
		height: image.Height()}
	header := image.Header()
	for key, into := range map[string]*float64{"MJD-OBS": &cut.mjd,
		"CRPIX1A": &cut.crpix1A, "CRPIX2A": &cut.crpix2A} {
		if *into, err = header.Number(key); err != nil {
			return nil, err
		}
	}
	detector, err := header.Integer("DETECTOR")
	if err != nil {
		return nil, err
	}
	cut.detector = int(detector)
	if cut.sky, err = image.Sky(); err != nil {
		return nil, err
	}
	if cut.pixels, err = rows(image); err != nil {
		return nil, err
	}
	return cut, cut.readRest(file)
}

// readRest reads the variances and the flags beside the pixels.
func (cut *cutout) readRest(file io.ReaderAt) error {
	variance, err := fits.OpenNamed(file, "VARIANCE")
	if err != nil {
		return err
	}
	if cut.variances, err = rows(variance); err != nil {
		return err
	}
	flags, err := fits.OpenNamed(file, "FLAGS")
	if err != nil {
		return err
	}
	bits, err := rows(flags)
	if err != nil {
		return err
	}
	cut.flags = make([]int64, len(bits))
	for at, each := range bits {
		cut.flags[at] = int64(each)
	}
	if len(cut.variances) != len(cut.pixels) ||
		len(cut.flags) != len(cut.pixels) {
		return fmt.Errorf("%w: IMAGE, VARIANCE and FLAGS of different "+
			"sizes", errRefused)
	}
	return nil
}

// rows is an image's every pixel, row by row from the bottom.
func rows(image *fits.Image) ([]float64, error) {
	var all []float64
	for y := range image.Height() {
		row, err := image.Row(y)
		if err != nil {
			return nil, err
		}
		all = append(all, row...)
	}
	return all, nil
}

// place is where the star falls in the cutout at the image's time,
// counted from the first pixel's centre, and it reads the ePSF zone
// and the wavelength there. It refuses a star the cutout does not
// hold, as one clipped at the detector's edge may not.
func (cut *cutout) place(star Star) (float64, float64, error) {
	ra, dec := moved(star, cut.mjd)
	px, py, seen := cut.sky.Pixel(ra, dec)
	x, y := px-firstPixel, py-firstPixel
	inside := x >= -halfPixel && x <= float64(cut.width)-halfPixel &&
		y >= -halfPixel && y <= float64(cut.height)-halfPixel
	if !seen || !inside {
		return 0, 0, fmt.Errorf("%w: the star falls at %.2f, %.2f, "+
			"off the cutout's %d by %d", errRefused, x, y,
			cut.width, cut.height)
	}
	cut.omega = solidAngle(cut.sky, px, py)
	// the user guide: a parent's pixel from none is one more than a
	// cutout's from none, less CRPIXnA
	cut.parentX = firstPixel + x - cut.crpix1A
	cut.parentY = firstPixel + y - cut.crpix2A
	var err error
	if cut.shape, err = cut.zone(); err != nil {
		return 0, 0, err
	}
	cut.wave, cut.band, err = cut.wavelength()
	return x, y, err
}

// zone is the ePSF of the zone whose centre is nearest the star, from
// the EPSF table: XCENTER and YCENTER count the parent's pixels from
// none, and OVSMPX is the samples to a pixel.
func (cut *cutout) zone() (*psf.Shape, error) {
	table := cut.epsf
	columns := map[string]*fits.Column{}
	var err error
	for _, name := range []string{"XCENTER", "YCENTER", "EPSF"} {
		if columns[name], err = table.Column(name); err != nil {
			return nil, err
		}
	}
	nearest, at := math.Inf(1), int64(-nextOne)
	for row := range table.Rows() {
		cells, err := table.Row(row)
		if err != nil {
			return nil, err
		}
		x, err := cells.Float(*columns["XCENTER"])
		if err != nil {
			return nil, err
		}
		y, err := cells.Float(*columns["YCENTER"])
		if err != nil {
			return nil, err
		}
		if apart := math.Hypot(x-cut.parentX, y-cut.parentY); apart <
			nearest {
			nearest, at = apart, row
		}
	}
	return shapeAt(table, columns["EPSF"], at)
}

// shapeAt is the ePSF in a row of the EPSF table, a square of samples.
func shapeAt(table *fits.Table, column *fits.Column, at int64) (
	*psf.Shape, error) {
	cells, err := table.Row(at)
	if err != nil {
		return nil, err
	}
	samples, err := cells.Floats(*column)
	if err != nil {
		return nil, err
	}
	over, err := table.Header().Integer("OVSMPX")
	if err != nil {
		return nil, err
	}
	dims := column.Dims()
	if len(dims) != planeAxes || dims[0] != dims[nextOne] {
		return nil, fmt.Errorf("%w: an EPSF shaped %v, not a square",
			errRefused, dims)
	}
	return psf.ShapeOf(samples, dims[0], int(over))
}

// wavelength is the wavelength and bandwidth at the star, in microns,
// from WCS-WAVE: a grid of the parent's pixels counted from one, X
// across and Y up, a pair at each point, interpolated between the four
// about the star.
func (cut *cutout) wavelength() (float64, float64, error) {
	table, err := fits.OpenTableNamed(cut.file, "WCS-WAVE")
	if err != nil {
		return 0, 0, err
	}
	row, err := table.Row(firstRow)
	if err != nil {
		return 0, 0, err
	}
	grid := map[string][]float64{}
	for _, name := range []string{"X", "Y", "VALUES"} {
		column, err := table.Column(name)
		if err != nil {
			return 0, 0, err
		}
		if grid[name], err = row.Floats(*column); err != nil {
			return 0, 0, err
		}
	}
	xs, ys, values := grid["X"], grid["Y"], grid["VALUES"]
	if len(values) != wavePair*len(xs)*len(ys) {
		return 0, 0, fmt.Errorf("%w: WCS-WAVE's %d values for a "+
			"grid of %d by %d", errRefused, len(values), len(xs),
			len(ys))
	}
	i, across, err := bracket(xs, cut.parentX+firstPixel)
	if err != nil {
		return 0, 0, err
	}
	j, up, err := bracket(ys, cut.parentY+firstPixel)
	if err != nil {
		return 0, 0, err
	}
	at := func(i, j, k int) float64 {
		return values[k+wavePair*(i+len(xs)*j)]
	}
	between := func(k int) float64 {
		lower := at(i, j, k)*(1-across) + at(i+nextOne, j, k)*across
		upper := at(i, j+nextOne, k)*(1-across) +
			at(i+nextOne, j+nextOne, k)*across
		return lower*(1-up) + upper*up
	}
	return between(waveAt), between(bandAt), nil
}

// bracket is the grid point at or below a place, and the share of the
// way to the next. It refuses a grid of fewer than two points or not
// rising at each, and a place off the grid.
func bracket(grid []float64, place float64) (int, float64, error) {
	if !rising(grid) {
		return 0, 0, fmt.Errorf("%w: a WCS-WAVE grid of %v, not two "+
			"or more points rising", errRefused, grid)
	}
	last := len(grid) - nextOne
	if place < grid[0] || place > grid[last] {
		return 0, 0, fmt.Errorf("%w: the parent's pixel %.2f is off "+
			"WCS-WAVE's grid, %g to %g", errRefused, place, grid[0],
			grid[last])
	}
	at := min(sort.SearchFloat64s(grid, place), last) - nextOne
	at = max(at, 0)
	return at, (place - grid[at]) / (grid[at+nextOne] - grid[at]), nil
}

// rising is whether a grid has two points or more, each past the one
// before it.
func rising(grid []float64) bool {
	if len(grid) <= nextOne {
		return false
	}
	for at := nextOne; at < len(grid); at++ {
		if !(grid[at] > grid[at-nextOne]) {
			return false
		}
	}
	return true
}

// tile is the cutout as psf takes it: its pixels and variances, the
// pixels flagged to be left out left out.
func (cut *cutout) tile() psf.Tile {
	tile := psf.Tile{Width: cut.width, Height: cut.height,
		Pixels: cut.pixels, Variance: cut.variances,
		Ignored: make([]bool, len(cut.pixels))}
	for at, bits := range cut.flags {
		tile.Ignored[at] = bits&leftOut != 0
	}
	return tile
}

// flagsNear is every flag set on the pixels within a radius of the
// star.
func (cut *cutout) flagsNear(x, y, radius float64) int64 {
	var near int64
	for at, bits := range cut.flags {
		column, row := at%cut.width, at/cut.width
		if math.Hypot(float64(column)-x, float64(row)-y) <= radius {
			near |= bits
		}
	}
	return near
}
