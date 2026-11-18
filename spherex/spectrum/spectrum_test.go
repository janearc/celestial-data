package spectrum_test

import (
	"bytes"
	"cmp"
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/janearc/celestial-data/internal/fitstest"
	"github.com/janearc/celestial-data/spherex/spectrum"
	"github.com/janearc/celestial/psf"
)

// The made cutout's numbers. Each is defined here, for the test.
const (
	side      = 17          // a cutout of 17 pixels a side
	samples   = 33          // an ePSF of 33 samples a side, as SPHEREx's
	over      = 5           // five to a pixel
	middle    = 16          // the ePSF's middle sample
	narrow    = 0.8         // the near zone's sigma, in pixels
	wide      = 1.6         // the far zone's
	scale     = 6.15 / 3600 // a SPHEREx pixel, in degrees
	starRA    = 287.5       // the star's place, in degrees
	starDec   = -22.2       //
	crpix1    = 9.3         // the star at x 8.3 from the first pixel
	crpix2    = 8.6         // and y 7.6
	crpix1A   = -100        // the cutout's corner in its parent
	crpix2A   = -200        //
	light     = 50.0        // the star's light, MJy/sr over its pixels
	backdrop  = 0.4         // the background a pixel, MJy/sr
	spread    = 0.01        // every pixel's variance
	epoch     = 2016.0      // the star's epoch, a Julian year
	epochMJD  = 57388.5     // that epoch as an MJD, from J2000's 51544.5
	firstWave = 1.0         // the wavelength at the parent's first row
	perRow    = 0.0005      // and how far it rises a row
	bandwidth = 0.03        // every pixel's bandwidth
	lastPixel = 2040        // a SPHEREx detector's last pixel
	detector  = 2           //
	hotBit    = 1 << 10     // HOT, a flag the fit leaves out
	sourceBit = 1 << 21     // SOURCE, a flag it keeps
)

// The tolerances the made cutout is held to. Each is picked: singles
// keep about seven digits, and a pixel's value is written as one.
const (
	recovered = 1e-5 // a share of the star's light
	waveClose = 1e-6 // microns
	areaClose = 1e-6 // a share of the pixel's solid angle
)

// gaussian is an ePSF's samples: a round gaussian of a sigma in pixels.
func gaussian(sigma float64) []float64 {
	values := make([]float64, samples*samples)
	for j := range samples {
		for i := range samples {
			dx := float64(i-middle) / over
			dy := float64(j-middle) / over
			values[j*samples+i] = math.Exp(-(dx*dx + dy*dy) /
				(2 * sigma * sigma))
		}
	}
	return values
}

// shapeOf is a gaussian as psf reads it.
func shapeOf(test testing.TB, sigma float64) *psf.Shape {
	test.Helper()
	shape, err := psf.ShapeOf(gaussian(sigma), samples, over)
	if err != nil {
		test.Fatal(err)
	}
	return shape
}

// made is how a made cutout is laid: the stars in it, at x and y from
// the first pixel's centre, with their light; the flags set; whether it
// has its EPSF, and where its parent's grid ends.
type made struct {
	stars  [][3]float64
	flags  map[int]int32
	values map[int]float32
	noEPSF bool
	gridTo float64
	mjd    float64
	broken broken
}

// broken is how a made cutout is broken, for the anti-tests: an HDU or
// a card of IMAGE's left out, a column named otherwise or made text,
// the EPSF's TDIM or OVSMPX wrong or gone, its near zone dark, WCS-WAVE
// a pair short, its grid flat or of one point, or no row at all,
// VARIANCE narrower than IMAGE.
type broken struct {
	without, noCard, renamed, text, tdim string
	noOver, dark                         bool
	short, flat, single, empty, narrow   bool
}

// cell is one column of a made table: its name, its form, and how a row
// writes its value.
type cell struct {
	name, form string
	write      func(*fitstest.Row) *fitstest.Row
}

// tableOf is a made table of rows of cells, each column named, or named
// otherwise, or made text, as the cutout is broken.
func (cut made) tableOf(name string, extra []string,
	rows ...[]cell) []byte {
	var columns []fitstest.Column
	for _, each := range rows[0] {
		form := each.form
		if each.name == cut.broken.text {
			form = fmt.Sprintf("%dA", bytesOf(each.form))
		}
		columns = append(columns, fitstest.NewColumn(cut.named(each.name),
			form))
	}
	var written []*fitstest.Row
	for _, cells := range rows {
		row := fitstest.NewRow()
		for _, each := range cells {
			if each.name == cut.broken.text {
				row = row.Text("text", bytesOf(each.form))
				continue
			}
			row = each.write(row)
		}
		written = append(written, row)
	}
	return fitstest.Table(name, columns, extra, written...)
}

// bytesOf is the bytes a form of numbers takes: its count of fours,
// for E and J, or of eights, for D.
func bytesOf(form string) int {
	count, letter := 1, form[len(form)-1]
	if len(form) > 1 {
		count, _ = strconv.Atoi(form[:len(form)-1])
	}
	if letter == 'D' {
		return 8 * count
	}
	return 4 * count
}

// named is a column's name, or otherwise when the cutout is broken by
// renaming it.
func (cut made) named(name string) string {
	if cut.broken.renamed == name {
		return name + "_"
	}
	return name
}

// skyCards are the made cutout's TAN grid and its other cards, less the
// one it is broken by leaving out.
func (cut made) skyCards() []string {
	var kept []string
	for _, card := range cut.allCards() {
		key := strings.TrimSpace(card[:8])
		if key != cut.broken.noCard {
			kept = append(kept, card)
		}
	}
	return kept
}

// allCards are every card of the made cutout's IMAGE header.
func (cut made) allCards() []string {
	mjd := cut.mjd
	if mjd == 0 {
		mjd = epochMJD
	}
	return []string{fitstest.Text("CTYPE1", "RA---TAN"),
		fitstest.Text("CTYPE2", "DEC--TAN"),
		fitstest.Card("CRVAL1", fmt.Sprint(starRA)),
		fitstest.Card("CRVAL2", fmt.Sprint(starDec)),
		fitstest.Card("CRPIX1", fmt.Sprint(crpix1)),
		fitstest.Card("CRPIX2", fmt.Sprint(crpix2)),
		fitstest.Card("CDELT1", fmt.Sprint(-scale)),
		fitstest.Card("CDELT2", fmt.Sprint(scale)),
		fitstest.Text("CUNIT1", "deg"), fitstest.Text("CUNIT2", "deg"),
		fitstest.Text("RADESYS", "ICRS"),
		fitstest.Card("CRPIX1A", fmt.Sprint(crpix1A)),
		fitstest.Card("CRPIX2A", fmt.Sprint(crpix2A)),
		fitstest.Card("MJD-OBS", fmt.Sprint(mjd)),
		fitstest.Card("DETECTOR", fmt.Sprint(detector))}
}

// file is the made cutout's bytes, laid out as SPHEREx's are.
func (cut made) file(test testing.TB) []byte {
	test.Helper()
	near := shapeOf(test, narrow)
	pixels := make([]float32, side*side)
	variances := make([]float32, side*side)
	flags := make([]int32, side*side)
	for y := range side {
		for x := range side {
			value := backdrop
			for _, star := range cut.stars {
				value += star[2] * near.Share(float64(x)-star[0],
					float64(y)-star[1])
			}
			pixels[y*side+x] = float32(value)
			variances[y*side+x] = spread
		}
	}
	for at, value := range cut.values {
		pixels[at] = value
	}
	for at, bits := range cut.flags {
		flags[at] = bits
	}
	varianceSide := side
	if cut.broken.narrow {
		varianceSide--
	}
	parts := map[string][]byte{
		"IMAGE": fitstest.Image("IMAGE", side, side, pixels,
			cut.skyCards()...),
		"FLAGS": fitstest.IntImage("FLAGS", side, side, flags),
		"VARIANCE": fitstest.Image("VARIANCE", varianceSide, side,
			variances[:varianceSide*side]),
		"EPSF": cut.epsf(), "WCS-WAVE": cut.wave()}
	laid := [][]byte{fitstest.Primary()}
	for _, name := range []string{"IMAGE", "FLAGS", "VARIANCE", "EPSF",
		"WCS-WAVE"} {
		gone := name == cut.broken.without || name == "EPSF" && cut.noEPSF
		if !gone {
			laid = append(laid, parts[name])
		}
	}
	return fitstest.Join(laid...)
}

// epsf is the EPSF table of two zones: the narrow one about the star's
// parent pixel, the wide one far off.
func (cut made) epsf() []byte {
	tdim := cmp.Or(cut.broken.tdim, "(33,33)")
	var extra []string
	if cut.broken.text != "EPSF" {
		extra = append(extra, fitstest.Text("TDIM3", tdim))
	}
	if !cut.broken.noOver {
		extra = append(extra, fitstest.Card("OVSMPX", fmt.Sprint(over)))
	}
	near := gaussian(narrow)
	if cut.broken.dark {
		near = make([]float64, samples*samples)
	}
	zone := func(x, y float32, shape []float64) []cell {
		return []cell{{"XCENTER", "E", single(x)},
			{"YCENTER", "E", single(y)},
			{"EPSF", fmt.Sprintf("%dD", samples*samples),
				func(row *fitstest.Row) *fitstest.Row {
					return row.Doubles(shape)
				}}}
	}
	return cut.tableOf("EPSF", extra, zone(1500, 1500, gaussian(wide)),
		zone(110, 210, near))
}

// single writes a single.
func single(value float32) func(*fitstest.Row) *fitstest.Row {
	return func(row *fitstest.Row) *fitstest.Row { return row.Single(value) }
}

// ints writes a column of many ints.
func ints(values []int32) func(*fitstest.Row) *fitstest.Row {
	return func(row *fitstest.Row) *fitstest.Row { return row.Ints(values) }
}

// wave is the WCS-WAVE table: a grid at the parent's corners, the
// wavelength rising up the rows, the bandwidth the same everywhere.
func (cut made) wave() []byte {
	last := float64(lastPixel)
	if cut.gridTo != 0 {
		last = cut.gridTo
	}
	top := float32(firstWave + perRow*(last-1))
	pairs := []float32{firstWave, bandwidth, firstWave, bandwidth, top,
		bandwidth, top, bandwidth}
	across, tdim := []int32{1, int32(last)}, "(2,2,2)"
	switch {
	case cut.broken.short:
		// a TDIM that agrees with the six, so the grid's own count is
		// what refuses them
		pairs, tdim = pairs[:6], "(2,3)"
	case cut.broken.flat:
		across = []int32{1, 1}
	case cut.broken.single:
		across, tdim = []int32{1}, "(2,1,2)"
		pairs = []float32{firstWave, bandwidth, top, bandwidth}
	}
	grid := []cell{{"X", fmt.Sprintf("%dJ", len(across)), ints(across)},
		{"Y", "2J", ints([]int32{1, int32(last)})},
		{"VALUES", fmt.Sprintf("%dE", len(pairs)),
			func(row *fitstest.Row) *fitstest.Row {
				return row.Singles(pairs)
			}}}
	var extra []string
	if cut.broken.text != "VALUES" {
		extra = append(extra, fitstest.Text("TDIM3", tdim))
	}
	if cut.broken.empty {
		columns := []fitstest.Column{}
		for _, each := range grid {
			columns = append(columns, fitstest.NewColumn(each.name,
				each.form))
		}
		return fitstest.Table("WCS-WAVE", columns, extra)
	}
	return cut.tableOf("WCS-WAVE", extra, grid)
}

// written is a made cutout in a test's folder, its path.
func written(test testing.TB, cut made, name string) string {
	test.Helper()
	path := filepath.Join(test.TempDir(), name)
	if err := os.WriteFile(path, cut.file(test), 0o600); err != nil {
		test.Fatal(err)
	}
	return path
}

// star is the made star at its epoch, still.
var star = spectrum.Star{RA: starRA, Dec: starDec, Epoch: epoch}

// measured is a made cutout measured, or the test stops.
func measured(test testing.TB, cut made) *spectrum.Point {
	test.Helper()
	measure, err := spectrum.New()
	if err != nil {
		test.Fatal(err)
	}
	point, err := measure.Cutout(written(test, cut, "made.fits"), star)
	if err != nil {
		test.Fatal(err)
	}
	return point
}

// theStar is the made star alone, where the grid puts Gaia's place.
var theStar = [3]float64{crpix1 - 1, crpix2 - 1, light}

// the test: in a made cutout, the star's light comes back as it was
// made, in micro-Jy by a pixel's solid angle; the wavelength is the
// grid's at the star's parent pixel; the narrow zone nearest the star
// is the shape fitted; the background, the place and the detector are
// the cutout's.
func TestAMadeStarComesBackAsItWasMade(test *testing.T) {
	point := measured(test, made{stars: [][3]float64{theStar}})
	area := scale * 3600 * scale * 3600
	if math.Abs(point.SolidAngle-area) > areaClose*area {
		test.Errorf("a pixel spans %g square arcseconds, not %g",
			point.SolidAngle, area)
	}
	steradian := math.Pow(180*3600/math.Pi, 2)
	want := light * 1e12 * point.SolidAngle / steradian
	if math.Abs(point.Flux-want) > recovered*want {
		test.Errorf("the star came back %g micro-Jy, not %g", point.Flux,
			want)
	}
	parentY := 1 + (crpix2 - 1) - crpix2A + 1
	wave := firstWave + perRow*(parentY-1)
	if math.Abs(point.Wavelength-wave) > waveClose ||
		math.Abs(point.Bandwidth-bandwidth) > waveClose {
		test.Errorf("seen at %g microns, %g wide, not %g and %g",
			point.Wavelength, point.Bandwidth, wave, bandwidth)
	}
	if math.Abs(point.Background-backdrop) > recovered ||
		point.Detector != detector || point.Neighbours != 0 ||
		math.Abs(point.X-theStar[0]) > recovered ||
		math.Abs(point.Y-theStar[1]) > recovered {
		test.Errorf("the background, detector, neighbours or place: %+v",
			point)
	}
	if point.Error < spectrumSystematic*point.Flux {
		test.Errorf("an error of %g, under the systematic share of %g",
			point.Error, point.Flux)
	}
}

// spectrumSystematic is numbers.yaml's systematic share, as the test
// holds the error to it.
const spectrumSystematic = 0.03

// the test: a neighbour in the cutout is found among its peaks and
// fitted beside the star, and the star's light comes back as when it is
// alone; a hot pixel is left out, its flag kept, and a source flag does
// not leave a pixel out.
func TestANeighbourAndAHotPixelLeaveTheStarAsItWas(test *testing.T) {
	alone := measured(test, made{stars: [][3]float64{theStar}})
	neighbour := [3]float64{13, 11, 2 * light}
	beside := measured(test, made{stars: [][3]float64{theStar,
		neighbour}})
	if beside.Neighbours != 1 ||
		math.Abs(beside.Flux-alone.Flux) > recovered*alone.Flux {
		test.Errorf("beside a neighbour, %d found and %g, not %g alone",
			beside.Neighbours, beside.Flux, alone.Flux)
	}
	peak := 7*side + 8
	hot := measured(test, made{stars: [][3]float64{theStar},
		flags:  map[int]int32{peak: hotBit, peak + 1: sourceBit},
		values: map[int]float32{peak: 1e6}})
	if math.Abs(hot.Flux-alone.Flux) > recovered*alone.Flux ||
		hot.Flags&hotBit == 0 || hot.Used != alone.Used-1 {
		test.Errorf("with a hot pixel: %g from %g, flags %#x, %d used",
			hot.Flux, alone.Flux, hot.Flags, hot.Used)
	}
}

// the test: a star with a proper motion is moved to the image's time
// before it is placed: ten years at 6.15 arcseconds a year in
// declination is ten pixels up.
func TestAMovingStarIsPlacedWhereItIsThen(test *testing.T) {
	tenYears := epochMJD + 10*365.25
	moving := spectrum.Star{RA: starRA, Dec: starDec - 61.5/3600,
		PMDec: 6150, Epoch: epoch}
	measure, err := spectrum.New()
	if err != nil {
		test.Fatal(err)
	}
	path := written(test, made{stars: [][3]float64{theStar},
		mjd: tenYears}, "moving.fits")
	point, err := measure.Cutout(path, moving)
	if err != nil {
		test.Fatal(err)
	}
	if math.Abs(point.Y-theStar[1]) > 0.01 ||
		math.Abs(point.X-theStar[0]) > 0.01 {
		test.Errorf("a moving star placed at %g, %g, not %g, %g",
			point.X, point.Y, theStar[0], theStar[1])
	}
}

// the anti-test: a cutout is refused, saying why, when it has no EPSF,
// as a QR2 cutout has none; when the star falls off it; when the star's
// parent pixel is off its wavelength grid; and when it is no FITS.
func TestACutoutThatCannotBeMeasuredIsRefused(test *testing.T) {
	measure, err := spectrum.New()
	if err != nil {
		test.Fatal(err)
	}
	far := spectrum.Star{RA: starRA, Dec: starDec + 0.1, Epoch: epoch}
	cases := []struct {
		cut  made
		star spectrum.Star
		says string
	}{
		{made{stars: [][3]float64{theStar}, noEPSF: true}, star,
			"no EPSF"},
		{made{stars: [][3]float64{theStar}}, far, "off the cutout"},
		{made{stars: [][3]float64{theStar}, gridTo: 150}, star,
			"off WCS-WAVE's grid"},
	}
	for _, each := range cases {
		path := written(test, each.cut, "refused.fits")
		_, err := measure.Cutout(path, each.star)
		refusedSaying(test, err, each.says)
	}
	notFITS := filepath.Join(test.TempDir(), "not.fits")
	if err := os.WriteFile(notFITS, []byte("no"), 0o600); err != nil {
		test.Fatal(err)
	}
	if _, err := measure.Cutout(notFITS, star); err == nil {
		test.Error("a file of no FITS was measured")
	}
}

// refusedSaying fails the test unless err is a refusal holding the
// words said.
func refusedSaying(test testing.TB, err error, said string) {
	test.Helper()
	if !errors.Is(err, spectrum.Refused()) ||
		!strings.Contains(err.Error(), said) {
		test.Errorf("refused with %v, not saying %q", err, said)
	}
}

// the test: a folder's cutouts are each measured, a file that is not
// one refused alone, and the table written a point a line under its
// columns, the wavelength rising; a folder that is not there is an
// error.
func TestAFolderIsMeasuredAndWritten(test *testing.T) {
	folder := test.TempDir()
	for name, cut := range map[string]made{
		"b.fits": {stars: [][3]float64{theStar}},
		"a.fits": {stars: [][3]float64{theStar}, noEPSF: true},
		"c.fits": {stars: [][3]float64{theStar}}} {
		if err := os.WriteFile(filepath.Join(folder, name),
			cut.file(test), 0o600); err != nil {
			test.Fatal(err)
		}
	}
	measure, err := spectrum.New()
	if err != nil {
		test.Fatal(err)
	}
	points, refused, err := measure.Folder(folder, star)
	if err != nil || len(points) != 2 || len(refused) != 1 ||
		!strings.Contains(refused[0].Error(), "a.fits") {
		test.Fatalf("%d points, refused %v, %v", len(points), refused, err)
	}
	var out bytes.Buffer
	if err := spectrum.Write(&out, points); err != nil {
		test.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(out.String()), "\n")
	if len(lines) != 3 || !strings.HasPrefix(lines[0], "# wavelength_um") ||
		!strings.HasSuffix(lines[1], "b.fits") {
		test.Errorf("the table written:\n%s", out.String())
	}
	if _, _, err := measure.Folder(filepath.Join(folder, "none"),
		star); err == nil {
		test.Error("a folder that is not there was measured")
	}
}

// The real cutout's numbers.
const (
	// noahsCutout is a QR3 cutout of Gaia DR3 4081136684118965760,
	// detector 1, fetched through the spherex archive on 2026-11-06.
	noahsCutout = "testdata/noahs-star-qr3-D1-0.966um.fits"
	// noahsY is Pan-STARRS DR2's mean y for the star, measured:
	// yMeanPSFMag of objID 81312875008568574, its MAST mean table as
	// the panstarrs archive fetched it, 2026-11-06.
	noahsY = 12.362199783325195
	// abZero is the AB scale's zero, 3631 Jy as micro-Jy. It's defined.
	abZero = 3631e6
	// agreement is how near the two must be, as a share: SPHEREx's 3%
	// systematic and the y band's width over a slope near a percent a
	// hundredth of a micron. It's picked.
	agreement = 0.05
)

// the test: Noah's star in a real QR3 cutout at 0.966 microns is
// Pan-STARRS's y within the agreement: two instruments, two methods;
// its neighbours are found, and Gaia's place falls near the cutout's
// middle, as IRSA cut it.
func TestNoahsStarAtNearOneMicronIsPanSTARRSY(test *testing.T) {
	measure, err := spectrum.New()
	if err != nil {
		test.Fatal(err)
	}
	noah := spectrum.Star{RA: 287.50086365607245, Dec: -22.234814871161884,
		PMRA: 6.824, PMDec: -3.703, Epoch: epoch}
	point, err := measure.Cutout(noahsCutout, noah)
	if err != nil {
		test.Fatal(err)
	}
	want := abZero * math.Pow(10, -0.4*noahsY)
	if math.Abs(point.Flux-want) > agreement*want ||
		point.Wavelength < 0.95 || point.Wavelength > 0.98 {
		test.Errorf("%g micro-Jy at %g microns, not y's %g",
			point.Flux, point.Wavelength, want)
	}
	if point.Neighbours == 0 || point.Detector != 1 ||
		math.Abs(point.X-8.5) > 1.5 || math.Abs(point.Y-8.5) > 1.5 {
		test.Errorf("neighbours %d, detector %d, at %g, %g",
			point.Neighbours, point.Detector, point.X, point.Y)
	}
}

// the anti-test: a cutout missing a part, a card or a column a
// measure reads, or with one it cannot read, is refused, saying what
// it lacks; the refusal is the fits package's when the file is not
// FITS as the standard lays it, and the package's own otherwise.
func TestABrokenCutoutIsRefusedSayingWhatItLacks(test *testing.T) {
	measure, err := spectrum.New()
	if err != nil {
		test.Fatal(err)
	}
	cases := []struct {
		broken broken
		says   string
	}{
		{broken{without: "IMAGE"}, `no HDU named "IMAGE"`},
		{broken{without: "FLAGS"}, `no HDU named "FLAGS"`},
		{broken{without: "VARIANCE"}, `no HDU named "VARIANCE"`},
		{broken{without: "WCS-WAVE"}, `no HDU named "WCS-WAVE"`},
		{broken{noCard: "MJD-OBS"}, "MJD-OBS"},
		{broken{noCard: "CRPIX2A"}, "CRPIX2A"},
		{broken{noCard: "DETECTOR"}, "DETECTOR"},
		{broken{noCard: "CTYPE1"}, "CTYPE1"},
		{broken{renamed: "XCENTER"}, "no column XCENTER"},
		{broken{renamed: "EPSF"}, "no column EPSF"},
		{broken{renamed: "VALUES"}, "no column VALUES"},
		{broken{text: "XCENTER"}, "XCENTER holds no numbers"},
		{broken{text: "YCENTER"}, "YCENTER holds no numbers"},
		{broken{text: "EPSF"}, "EPSF holds no numbers"},
		{broken{text: "VALUES"}, "VALUES holds no numbers"},
		{broken{empty: true}, "row 0 of 0"},
		{broken{tdim: "(1089)"}, "not a square"},
		{broken{noOver: true}, "OVSMPX"},
		{broken{dark: true}, "samples summing to 0"},
		{broken{short: true}, "6 values for a grid of 2 by 2"},
		{broken{flat: true}, "not two or more points rising"},
		{broken{single: true}, "not two or more points rising"},
		{broken{narrow: true}, "different sizes"},
	}
	for _, each := range cases {
		cut := made{stars: [][3]float64{theStar}, broken: each.broken}
		_, err := measure.Cutout(written(test, cut, "broken.fits"), star)
		if err == nil || !strings.Contains(err.Error(), each.says) {
			test.Errorf("broken %+v: refused with %v, not saying %q",
				each.broken, err, each.says)
		}
	}
	if _, err := measure.Cutout(filepath.Join(test.TempDir(),
		"none.fits"), star); err == nil {
		test.Error("a cutout that is not there was measured")
	}
}

// the test: a star whose every pixel within the quality circle is left
// out is still fitted on its wings, and its quality is not a number,
// not a made-up one; an image with no number in it is refused.
func TestAStarWithNoMiddleHasNoQuality(test *testing.T) {
	flags := map[int]int32{}
	for y := range side {
		for x := range side {
			if math.Hypot(float64(x)-theStar[0],
				float64(y)-theStar[1]) <= 2.5 {
				flags[y*side+x] = hotBit
			}
		}
	}
	point := measured(test, made{stars: [][3]float64{theStar},
		flags: flags})
	if !math.IsNaN(point.Quality) || !(point.Flux > 0) {
		test.Errorf("with its middle left out: quality %g, flux %g",
			point.Quality, point.Flux)
	}
	dark := map[int]float32{}
	for at := range side * side {
		dark[at] = float32(math.NaN())
	}
	measure, err := spectrum.New()
	if err != nil {
		test.Fatal(err)
	}
	path := written(test, made{stars: [][3]float64{theStar},
		values: dark}, "dark.fits")
	if _, err := measure.Cutout(path, star); !errors.Is(err,
		psf.Refused()) {
		test.Errorf("an image of no numbers: %v", err)
	}
}

// failing is a writer that takes so many writes and refuses the rest.
type failing struct{ left int }

// Write takes a write while it has any left.
func (out *failing) Write(text []byte) (int, error) {
	if out.left == 0 {
		return 0, errors.New("the writer is full")
	}
	out.left--
	return len(text), nil
}

// the anti-test: a table whose writer refuses its columns' line, or a
// point's, is not written whole, and says so.
func TestATableItCannotWriteSaysSo(test *testing.T) {
	point := measured(test, made{stars: [][3]float64{theStar}})
	for _, left := range []int{0, 1} {
		err := spectrum.Write(&failing{left: left},
			[]spectrum.Point{*point})
		if err == nil || !strings.Contains(err.Error(), "full") {
			test.Errorf("a writer full after %d writes: %v", left, err)
		}
	}
}
