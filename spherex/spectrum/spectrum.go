package spectrum

import (
	_ "embed"
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sort"

	"github.com/janearc/celestial-data"
	"github.com/janearc/celestial/fits"
	"github.com/janearc/celestial/psf"
)

// numbersFile is the numbers file the measure is picked and read from.
const numbersFile = "numbers.yaml"

//go:embed numbers.yaml
var numbersRaw []byte

// The names of the numbers in numbers.yaml.
const (
	systematicName = "systematic"
	radiusName     = "quality-radius"
	peakName       = "peak-sigma"
	ownPeakName    = "own-peak"
)

// The times a star's place is moved by. Each is defined.
const (
	// j2000JD is j2000.0's Julian date, defined by the IAU, as
	// celestial's sky package keeps it.
	j2000JD = 2451545.0
	// mjdZero is the Julian date the MJD counts from.
	mjdZero = 2400000.5
	// julianYear is a Julian year's days, defined by the IAU.
	julianYear = 365.25
	// j2000Year is the Julian year j2000.0 is.
	j2000Year = 2000.0
)

// The sky's units a place and a flux are turned by. Each is defined.
const (
	masInDegree    = 3.6e6 // a degree's milliarcseconds
	arcsecInDegree = 3600  // a degree's arcseconds
	halfTurn       = 180   // a half turn's degrees
	microInMega    = 1e12  // MJy in micro-Jy
	halfPixel      = 0.5   // a pixel's half, either side of its centre
	firstPixel     = 1     // the first pixel, as FITS counts them
)

// steradian is the square arcseconds in a steradian. It's defined.
const steradian = (halfTurn * arcsecInDegree / math.Pi) *
	(halfTurn * arcsecInDegree / math.Pi)

// errRefused is what every refusal of the package wraps.
var errRefused = errors.New("refused")

// Refused is the error every refusal wraps, for errors.Is.
func Refused() error { return errRefused }

// Star is where a star is, as Gaia gives it: its place in degrees at
// its epoch, a Julian year, and its proper motion in milliarcseconds a
// year, the one in right ascension along the sky, as Gaia's pmra is.
type Star struct {
	RA, Dec     float64
	PMRA, PMDec float64
	Epoch       float64
}

// Point is a star's light in one cutout: the wavelength and bandwidth
// in microns where the cutout saw it, and its flux in micro-Jy with the
// systematic share added to its error, and without it.
//
// The rest say how the fit went: the background a pixel, in MJy/sr; the
// chi-squared a degree of freedom over the pixels used; the quality
// over the star's circle; the flags of the pixels in that circle; the
// neighbours fitted beside it.
//
// And where: the image's time, its detector, the star's place in the
// cutout counted from the first pixel's centre, a pixel's solid angle
// there in square arcseconds, and the file.
type Point struct {
	Wavelength, Bandwidth    float64
	Flux, Error, Statistical float64
	Background               float64
	ChiSquared               float64
	Used                     int
	Quality                  float64
	Flags                    int64
	Neighbours               int
	MJD                      float64
	Detector                 int
	X, Y                     float64
	SolidAngle               float64
	File                     string
}

// Measure is the measure of a star in SPHEREx cutouts, by the numbers
// in numbers.yaml.
type Measure struct {
	systematic, radius float64
	peakSigma, ownPeak float64
}

// New is the measure, its numbers read from numbers.yaml. It refuses a
// numbers file that does not read or lacks a number.
func New() (*Measure, error) {
	kept := celestialdata.NewMeasured(numbersFile, numbersRaw)
	measure := &Measure{}
	for name, into := range map[string]*float64{
		systematicName: &measure.systematic,
		radiusName:     &measure.radius,
		peakName:       &measure.peakSigma,
		ownPeakName:    &measure.ownPeak} {
		value, err := kept.Get(name)
		if err != nil {
			return nil, err
		}
		*into = value
	}
	return measure, nil
}

// Folder is the star measured in every FITS file in a folder, in the
// files' order by name, and why each file it could not measure was
// refused. It refuses a folder it cannot list.
func (measure *Measure) Folder(folder string, star Star) ([]Point,
	[]error, error) {
	if _, err := os.Stat(folder); err != nil {
		return nil, nil, err
	}
	names, err := filepath.Glob(filepath.Join(folder, "*.fits"))
	if err != nil {
		return nil, nil, err
	}
	sort.Strings(names)
	var points []Point
	var refused []error
	for _, name := range names {
		point, err := measure.Cutout(name, star)
		if err != nil {
			refused = append(refused, fmt.Errorf("%s: %w",
				filepath.Base(name), err))
			continue
		}
		points = append(points, *point)
	}
	return points, refused, nil
}

// Cutout is the star measured in one cutout. It refuses a file that is
// not a SPHEREx cutout as the user guide lays one out, a QR2 cutout,
// a star the cutout does not hold, and a fit past solving.
func (measure *Measure) Cutout(path string, star Star) (*Point, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	cut, err := read(file)
	if err != nil {
		return nil, err
	}
	x, y, err := cut.place(star)
	if err != nil {
		return nil, err
	}
	point, err := measure.fit(cut, x, y)
	if err != nil {
		return nil, err
	}
	point.File = filepath.Base(path)
	return point, nil
}

// moved is a star's place at an MJD: its place at its epoch moved by
// its proper motion over the years between.
func moved(star Star, mjd float64) (float64, float64) {
	epochMJD := j2000JD - mjdZero + (star.Epoch-j2000Year)*julianYear
	years := (mjd - epochMJD) / julianYear
	cosDec := math.Cos(star.Dec * math.Pi / halfTurn)
	ra := star.RA + star.PMRA*years/masInDegree/cosDec
	dec := star.Dec + star.PMDec*years/masInDegree
	return ra, dec
}

// solidAngle is a pixel's solid angle at a pixel, in square arcseconds:
// the area on the sky its two steps span there, as the projection puts
// them, SIP's distortion and all.
func solidAngle(sky *fits.Sky, px, py float64) float64 {
	westRA, westDec := sky.Place(px-halfPixel, py)
	eastRA, eastDec := sky.Place(px+halfPixel, py)
	lowRA, lowDec := sky.Place(px, py-halfPixel)
	highRA, highDec := sky.Place(px, py+halfPixel)
	cosDec := math.Cos(westDec * math.Pi / halfTurn)
	ax, ay := (eastRA-westRA)*cosDec, eastDec-westDec
	bx, by := (highRA-lowRA)*cosDec, highDec-lowDec
	return math.Abs(ax*by-ay*bx) * arcsecInDegree * arcsecInDegree
}

// fit is the star's point in a cutout: the star at x and y, its
// neighbours and the background fitted, the light turned to micro-Jy by
// the pixel's solid angle.
func (measure *Measure) fit(cut *cutout, x, y float64) (*Point, error) {
	tile := cut.tile()
	star := psf.Star{X: x, Y: y, Shape: cut.shape}
	others := measure.neighbours(tile, x, y, cut.shape)
	tile.Stars = append([]psf.Star{star}, others...)
	fitted := psf.Fit([]psf.Tile{tile})[0]
	if fitted.Err != nil {
		return nil, fitted.Err
	}
	toMicro := microInMega * cut.omega / steradian
	flux := fitted.Fluxes[0] * toMicro
	statistical := fitted.Errors[0] * toMicro
	freedom := max(fitted.Used-len(tile.Stars)-background, background)
	return &Point{Wavelength: cut.wave, Bandwidth: cut.band, Flux: flux,
		Error:       math.Hypot(statistical, measure.systematic*flux),
		Statistical: statistical, Background: fitted.Background,
		ChiSquared: fitted.ChiSquared / float64(freedom),
		Used:       fitted.Used, Quality: measure.quality(tile, fitted),
		Flags:      cut.flagsNear(x, y, measure.radius),
		Neighbours: len(others), MJD: cut.mjd, Detector: cut.detector,
		X: x, Y: y, SolidAngle: cut.omega}, nil
}
