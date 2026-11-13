package bsc5_test

import (
	"bytes"
	"compress/gzip"
	"math"
	"os"
	"strings"
	"testing"

	"github.com/janearc/celestial-data/bsc5"
)

// sample is six rows of the catalogue as Harvard served it, byte for
// byte: HR 1 and 2, the first two rows; HR 92, Tycho's nova, and HR 95,
// 47 Tucanae, which have no place; Sirius, HR 2491; and Vega, HR 7001.
func sample(test *testing.T) []byte {
	test.Helper()
	raw, err := os.ReadFile("testdata/sample.txt")
	if err != nil {
		test.Fatal(err)
	}
	return raw
}

// near is whether got is within tolerance of want; a NaN never is.
func near(got, want, tolerance float64) bool {
	return math.Abs(got-want) <= tolerance
}

// the test: the placed rows read as the readme lays them out. HR 1 is
// at 00h 05m 09.9s, +45 13 45; HR 2 at 00h 05m 03.8s, -00 30 11, south
// though its degrees are none; Sirius at 06h 45m 08.9s, -16 42 58; Vega
// at 18h 36m 56.3s, +38 47 01. A right ascension is 15 (h + m/60 + s/3600).
func TestTheRowsReadAsTheReadmeLaysThemOut(test *testing.T) {
	stars, err := bsc5.Read(bytes.NewReader(sample(test)))
	if err != nil {
		test.Fatal(err)
	}
	if len(stars) != 6 {
		test.Fatalf("%d stars from six rows", len(stars))
	}
	cases := []struct {
		at, hr, hd int
		name, kind string
		ra, dec, v float64
		bv         float64
	}{
		{0, 1, 3, "", "A1Vn", 1.29125, 45.229167, 6.70, 0.07},
		{1, 2, 6, "", "gG9", 1.265833, -0.503056, 6.29, 1.10},
		{4, 2491, 48915, "9Alp CMa", "A1Vm", 101.287083, -16.716111,
			-1.46, 0},
		{5, 7001, 172167, "3Alp Lyr", "A0Va", 279.234583, 38.783611,
			0.03, 0},
	}
	for _, want := range cases {
		star := stars[want.at]
		hd, hasHD := star.HD()
		visual, hasVisual := star.Visual()
		bv, hasBV := star.BV()
		if star.HR() != want.hr || hd != want.hd || !hasHD ||
			star.Name() != want.name || star.Spectral() != want.kind ||
			!hasVisual || !near(visual, want.v, 1e-9) || !hasBV ||
			!near(bv, want.bv, 1e-9) {
			test.Errorf("row %d: HR %d HD %d %q %q V %v B-V %v", want.at,
				star.HR(), hd, star.Name(), star.Spectral(), visual, bv)
		}
		place := star.Place()
		if place == nil ||
			!near(place.Ascension().InDegrees(), want.ra, 1e-6) ||
			!near(place.Declination().InDegrees(), want.dec, 1e-6) {
			test.Errorf("HR %d is at %v", want.hr, place)
		}
	}
}

// the test: the rows with no place, Tycho's nova long faded and 47
// Tucanae, a cluster, read as their number and name and nothing else.
func TestTheRowsWithNoPlaceHoldOnlyTheirNames(test *testing.T) {
	stars, err := bsc5.Read(bytes.NewReader(sample(test)))
	if err != nil {
		test.Fatal(err)
	}
	cases := []struct {
		at, hr int
		name   string
	}{{2, 92, "NOVA 1572"}, {3, 95, "47    Tuc"}}
	for _, want := range cases {
		star := stars[want.at]
		_, hasHD := star.HD()
		_, hasVisual := star.Visual()
		_, hasBV := star.BV()
		if star.HR() != want.hr || star.Name() != want.name ||
			star.Place() != nil || hasHD || hasVisual || hasBV ||
			star.Spectral() != "" {
			test.Errorf("HR %d reads as %d %q at %v, HD %v V %v B-V %v %q",
				want.hr, star.HR(), star.Name(), star.Place(), hasHD,
				hasVisual, hasBV, star.Spectral())
		}
	}
}

// the test: the catalogue gzipped, as Harvard serves it, reads as the
// same rows.
func TestTheGzippedCatalogueReadsTheSame(test *testing.T) {
	plain, err := bsc5.Read(bytes.NewReader(sample(test)))
	if err != nil {
		test.Fatal(err)
	}
	unzipped, err := bsc5.Read(gzipped(test, sample(test)))
	if err != nil {
		test.Fatal(err)
	}
	if len(unzipped) != len(plain) || unzipped[4].HR() != plain[4].HR() {
		test.Errorf("gzipped: %d stars; plain: %d", len(unzipped), len(plain))
	}
}

// gzipped is bytes gzipped, as a reader.
func gzipped(test *testing.T, raw []byte) *bytes.Buffer {
	test.Helper()
	var zipped bytes.Buffer
	writer := gzip.NewWriter(&zipped)
	if _, err := writer.Write(raw); err != nil {
		test.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		test.Fatal(err)
	}
	return &zipped
}

// spliced is a row with its bytes from first to last, as the readme
// numbers them from one, put in place of its own.
func spliced(row string, first, last int, put string) string {
	if len(put) != last-first+1 {
		panic("a splice must fill its bytes")
	}
	return row[:first-1] + put + row[last:]
}

// the test: a row with any field that is not what the readme says it
// is, or with half a place, is refused, naming its line. Anti-tests of
// the reader, a field at a time.
func TestARowThatCannotBeReadIsRefused(test *testing.T) {
	lines := strings.Split(string(sample(test)), "\n")
	sirius := lines[4]
	broken := map[string]string{
		"an HR that is no number":  spliced(sirius, 1, 4, "abcd"),
		"an HR of nought":          spliced(sirius, 1, 4, "   0"),
		"an HR with a sign":        spliced(sirius, 1, 4, "+123"),
		"an HD that is no number":  spliced(sirius, 26, 31, " 48x15"),
		"seconds that are none":    spliced(sirius, 80, 83, "0x.9"),
		"a sign that is neither":   spliced(sirius, 84, 84, "*"),
		"minutes that are none":    spliced(sirius, 87, 88, "4x"),
		"a declination past 90":    spliced(sirius, 85, 86, "95"),
		"a V that is no number":    spliced(sirius, 103, 107, "-1.x6"),
		"a V of NaN":               spliced(sirius, 103, 107, "  NaN"),
		"a V of Inf":               spliced(sirius, 103, 107, "  Inf"),
		"a V with an exponent":     spliced(sirius, 103, 107, " 1e+0"),
		"a V in hexadecimal":       spliced(sirius, 103, 107, "0x1p1"),
		"a B-V that is no number":  spliced(sirius, 110, 114, "+0.x0"),
		"no declination":           spliced(sirius, 84, 90, "       "),
		"no right ascension":       spliced(sirius, 76, 83, "        "),
		"a right ascension, hours": spliced(sirius, 76, 77, "  "),
	}
	for why, row := range broken {
		text := lines[0] + "\n" + row + "\n"
		if _, err := bsc5.Read(strings.NewReader(text)); err == nil ||
			!strings.Contains(err.Error(), "line 2") {
			test.Errorf("%s: %v", why, err)
		}
	}
}

// the test: a file that is not the catalogue is refused: none at all, a
// byte of something else, a gzip that is no gzip past its first two
// bytes, and the catalogue gzipped and cut short, as a download can be.
func TestAFileThatIsNotTheCatalogueIsRefused(test *testing.T) {
	whole := gzipped(test, sample(test)).Bytes()
	files := map[string][]byte{
		"nothing":            nil,
		"a byte":             []byte("x"),
		"a gzip that is not": []byte("\x1f\x8bnot gzip at all"),
		"a gzip cut short":   whole[:len(whole)/2],
	}
	for why, file := range files {
		if stars, err := bsc5.Read(bytes.NewReader(file)); err == nil {
			test.Errorf("%s read as %d stars", why, len(stars))
		}
	}
}
