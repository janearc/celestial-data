package spherex_test

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"go.uber.org/mock/gomock"

	"github.com/janearc/celestial-data"
	"github.com/janearc/celestial-data/internal/replay"
	"github.com/janearc/celestial-data/mocks"
	"github.com/janearc/celestial-data/spherex"
)

// The place the recorded answer was asked at on 2026-11-06, and what
// was asked.
const (
	adapter = "spherex" // defined: the archive's name
	// recRA is the right ascension asked, in degrees. It's defined by
	// the recorded query.
	recRA = 287.50086
	// recDec is the declination asked, in degrees. It's defined by the
	// recorded query.
	recDec = -22.23481
	// recRadius is the radius asked, in degrees. It's defined by the
	// recorded query.
	recRadius = 0.01
	// most is the most images the recorded query asked for. It's
	// defined by the recorded query.
	most = 3
	// overRA is recRA to more decimals than the address writes. It's
	// defined.
	overRA = 287.50086 + 4.9e-7
	// overDec is recDec to more decimals than the address writes.
	// It's defined.
	overDec = -22.23481 - 4e-6
)

// Counts these tests make and compare. Each is defined by the test that
// uses it.
const (
	least  = 1  // defined: the fewest images a search may ask for
	none   = 0  // defined: no products, no bytes, no files
	once   = 1  // defined: a request made exactly once, a file kept once
	first  = 0  // defined: the first product
	unsaid = -1 // defined: the length web gives when none is said
	half   = 2  // defined: what halves a block
	nine   = 9  // defined: the blocks of the IMAGE header in the file
)

// The radii whose cutouts the bounds in numbers.yaml fix, and the sides
// asked for them, as the address writes them.
const (
	smallRadius  = 0.01 // defined: under the least side: the least
	middleRadius = 0.05 // defined: between the bounds: twice the radius
	largeRadius  = 0.3  // defined: over the most side: the most
)

// The recorded cutout, which is the first two units of a real one.
const (
	// block is a FITS block, in bytes. It's defined by the FITS
	// standard.
	block = 2880
	// primaryEnd is where the primary header ends in the recorded
	// cutout. It's defined by the file.
	primaryEnd = block
	// imageEnd is where the recorded cutout's IMAGE unit ends. It's
	// defined by the file.
	imageEnd = 34560
	// imageData is where the recorded cutout's IMAGE data begins: nine
	// blocks of header after the primary. It's defined by the file.
	imageData = primaryEnd + nine*block
)

// asked is the address the recorded query was asked at: the question, in
// ADQL, as TAP's sync endpoint takes it.
var asked = "https://irsa.ipac.caltech.edu/TAP/sync?" + url.Values{
	"REQUEST": {"doQuery"}, "LANG": {"ADQL"}, "FORMAT": {"votable"},
	"QUERY": {"SELECT TOP 3 a.uri, p.energy_bounds_lower, " +
		"p.energy_bounds_upper, p.time_bounds_lower, " +
		"p.provenance_version FROM spherex.artifact a JOIN " +
		"spherex.plane p ON a.planeid = p.planeid WHERE 1 = " +
		"INTERSECTS(p.poly, CIRCLE('ICRS', 287.50086, -22.23481, " +
		"0.01)) ORDER BY p.time_bounds_lower, a.uri"}}.Encode()

// The paths of images, as IRSA's artifact table writes them.
const (
	qr2First  = "ibe/data/spherex/qr2/level2/2025W35_2A/l2b-v20-2025-271/3/level2_2025W35_2A_0599_1D3_spx_l2b-v20-2025-271.fits"
	qr2Second = "ibe/data/spherex/qr2/level2/2025W35_2A/l2b-v20-2025-271/6/level2_2025W35_2A_0599_1D6_spx_l2b-v20-2025-271.fits"
	qr2Third  = "ibe/data/spherex/qr2/level2/2025W37_2A/l2b-v20-2025-267/6/level2_2025W37_2A_0342_1D6_spx_l2b-v20-2025-267.fits"
	qr3Image  = "ibe/data/spherex/qr3/level2/2026W37_2A/l2b-v27-2026-259/2/level2_2026W37_2A_0136_2D2_spx_l2b-v27-2026-259.fits"
	darkImage = "ibe/data/spherex/qr3/dark/cal-drk-v4-2025-161/1/dark_D1_spx_cal-drk-v4-2025-161.fits"
)

// The addresses and names these tests expect.
const (
	root   = "https://irsa.ipac.caltech.edu/"
	centre = "?center=287.50086,-22.23481"
	floor  = "&size=0.0300"
	tail   = "_ra287.50086_dec-22.23481_0.0300deg.fits"
)

// near is the circle of a radius round a place, or the test stops.
func near(test *testing.T, ra, dec, radius float64) *celestialdata.Near {
	test.Helper()
	place, err := celestialdata.NewPosition(ra, dec)
	if err != nil {
		test.Fatal(err)
	}
	circle, err := celestialdata.NewNear(place, radius)
	if err != nil {
		test.Fatal(err)
	}
	return circle
}

// recorded is the circle the answer was recorded for.
func recorded(test *testing.T) *celestialdata.Near {
	test.Helper()
	return near(test, recRA, recDec, recRadius)
}

// made is the archive asked through get, or the test stops.
func made(test *testing.T, get *mocks.MockWeb) *spherex.Archive {
	test.Helper()
	archive, err := spherex.New(get, most)
	if err != nil {
		test.Fatal(err)
	}
	return archive
}

// search is a search of the circle through get, and what it finds.
func search(test *testing.T, get *mocks.MockWeb,
	circle *celestialdata.Near) ([]*celestialdata.Product, error) {
	test.Helper()
	archive := made(test, get)
	return archive.Find(circle)
}

// fields are the columns of the answer, in the order the query asks.
var fields = []string{"uri", "energy_bounds_lower", "energy_bounds_upper",
	"time_bounds_lower", "provenance_version"}

// answer is a VOTable of the columns given, with a row of the cells given
// for each, as TAP's sync endpoint writes one.
func answer(columns []string, rows ...[]string) []byte {
	var table strings.Builder
	table.WriteString(`<?xml version="1.0" encoding="utf-8"?>` +
		`<VOTABLE version="1.3" xmlns="http://www.ivoa.net/xml/` +
		`VOTable/v1.3"><RESOURCE type="results"><INFO name=` +
		`"QUERY_STATUS" value="OK"/><TABLE>`)
	for _, name := range columns {
		kind := `datatype="double"`
		if name == "uri" || name == "provenance_version" {
			kind = `datatype="char" arraysize="*"`
		}
		fmt.Fprintf(&table, `<FIELD name="%s" %s/>`, name, kind)
	}
	table.WriteString("<DATA><TABLEDATA>")
	for _, row := range rows {
		table.WriteString("<TR>")
		for _, cell := range row {
			fmt.Fprintf(&table, "<TD>%s</TD>", cell)
		}
		table.WriteString("</TR>")
	}
	table.WriteString("</TABLEDATA></DATA></TABLE></RESOURCE></VOTABLE>")
	return []byte(table.String())
}

// qr3Row is the row IRSA's tables gave for the QR3 image, detector 2.
var qr3Row = []string{qr3Image, "1.10330e-06", "1.64990e-06",
	"61294.155530", "7.0.5"}

// rowOf is a row of the cells given for the path, a band, a time and a
// pipeline.
func rowOf(path, low, high, mjd, pipeline string) []string {
	return []string{path, low, high, mjd, pipeline}
}

// saying is a web that answers its one question with content, and notes
// the address asked.
type saying struct {
	content   []byte
	addresses []string
}

// get notes the address and answers with the content, saying no length.
func (said *saying) get(address string) (io.ReadCloser, int64, error) {
	said.addresses = append(said.addresses, address)
	return io.NopCloser(bytes.NewReader(said.content)), unsaid, nil
}

// serving is a web that answers one question with content, or the test
// stops, and the saying it notes the address in.
func serving(test *testing.T, content []byte) (*mocks.MockWeb, *saying) {
	test.Helper()
	said := &saying{content: content}
	get := mocks.NewMockWeb(gomock.NewController(test))
	get.EXPECT().Get(gomock.Any()).DoAndReturn(said.get).Times(once)
	return get, said
}

// found is the products a search finds in an answer, or the test stops.
func found(test *testing.T, content []byte,
	circle *celestialdata.Near) []*celestialdata.Product {
	test.Helper()
	get, _ := serving(test, content)
	products, err := search(test, get, circle)
	if err != nil {
		test.Fatal(err)
	}
	return products
}

// the test: the archive is named, asked by name only, asks for at least
// one image, and refuses no web.
func TestTheArchiveIsAskedByName(test *testing.T) {
	get := mocks.NewMockWeb(gomock.NewController(test))
	archive, err := spherex.New(get, least)
	if err != nil || archive.Name() != adapter || !archive.ByNameOnly() {
		test.Errorf("%v, %v", archive, err)
	}
	refused, err := spherex.New(get, none)
	if refused != nil || !errors.Is(err, celestialdata.Refused()) ||
		!strings.Contains(err.Error(), "at least one image") {
		test.Errorf("most none: %v", err)
	}
	if nowhere, err := spherex.New(nil, most); nowhere != nil ||
		err == nil {
		test.Errorf("no web: %v, %v", nowhere, err)
	}
}

// image is a product as a search should give it.
type image struct{ id, name, address, about string }

// the test: the search IRSA answered with three images finds each as
// a cutout, named for its release, its image and its place, at the
// address the image server cuts it at, saying its detector's band and
// when it was taken. The earliest comes first.
func TestASearchNamesEachImageAsACutout(test *testing.T) {
	get := replay.Web(test, map[string]string{asked: "images-three.xml"})
	products, err := search(test, get, recorded(test))
	if err != nil {
		test.Fatal(err)
	}
	wanted := []image{{"2025W35_2A_0599_1D3",
		"spherex_qr2_2025W35_2A_0599_1D3_l2b-v20-2025-271" + tail,
		root + qr2First + centre + floor,
		"SPHEREx qr2 image 2025W35_2A_0599_1D3, 1.63 to 2.43 micron, " +
			"taken 2025-08-31 13:31 UTC (MJD 60918.5639), pipeline " +
			"6.4; a cutout 0.0300 degrees square"},
		{"2025W35_2A_0599_1D6",
			"spherex_qr2_2025W35_2A_0599_1D6_l2b-v20-2025-271" + tail,
			root + qr2Second + centre + floor,
			"SPHEREx qr2 image 2025W35_2A_0599_1D6, 4.41 to 5.01 " +
				"micron, taken 2025-08-31 13:31 UTC (MJD 60918.5639), " +
				"pipeline 6.4; a cutout 0.0300 degrees square"},
		{"2025W37_2A_0342_1D6",
			"spherex_qr2_2025W37_2A_0342_1D6_l2b-v20-2025-267" + tail,
			root + qr2Third + centre + floor,
			"SPHEREx qr2 image 2025W37_2A_0342_1D6, 4.41 to 5.01 " +
				"micron, taken 2025-09-13 01:25 UTC (MJD 60931.0596), " +
				"pipeline 6.4; a cutout 0.0300 degrees square"}}
	if len(products) != len(wanted) {
		test.Fatalf("%d products", len(products))
	}
	for at, want := range wanted {
		got := products[at]
		if got.ID() != want.id || got.Name() != want.name ||
			got.URL() != want.address || got.About() != want.about ||
			got.Archive() != adapter {
			test.Errorf("%d: %s %s %s %q %s", at, got.ID(), got.Name(),
				got.URL(), got.About(), got.Archive())
		}
	}
}

// the test: a QR3 image is told from a QR2 one by its release, its
// detector and its pipeline, all of which are in what the product says.
func TestAQr3ImageSaysItsReleaseAndDetector(test *testing.T) {
	products := found(test, answer(fields, qr3Row), recorded(test))
	if len(products) != once {
		test.Fatalf("%d products", len(products))
	}
	got := products[first]
	wantName := "spherex_qr3_2026W37_2A_0136_2D2_l2b-v27-2026-259" + tail
	wantAbout := "SPHEREx qr3 image 2026W37_2A_0136_2D2, 1.10 to 1.65 " +
		"micron, taken 2026-09-11 03:43 UTC (MJD 61294.1555), " +
		"pipeline 7.0.5; a cutout 0.0300 degrees square"
	if got.ID() != "2026W37_2A_0136_2D2" || got.Name() != wantName ||
		got.About() != wantAbout {
		test.Errorf("%s %s %q", got.ID(), got.Name(), got.About())
	}
}

// the test: the cutout is twice the radius a side, no less than the
// least and no more than the most numbers.yaml says, and the query asks
// for the circle as given.
func TestTheSideFollowsTheRadiusWithinItsBounds(test *testing.T) {
	for radius, side := range map[float64]string{
		smallRadius:  "0.0300",
		middleRadius: "0.1000",
		largeRadius:  "0.5000",
	} {
		get, said := serving(test, answer(fields, qr3Row))
		circle := near(test, recRA, recDec, radius)
		products, err := search(test, get, circle)
		if err != nil || len(products) != once {
			test.Fatalf("%v: %v, %v", radius, products, err)
		}
		got := products[first]
		want := centre + "&size=" + side
		circled := fmt.Sprintf("%v%%29%%29", radius)
		if !strings.HasSuffix(got.URL(), want) ||
			!strings.Contains(got.Name(), "_"+side+"deg.") ||
			!strings.Contains(said.addresses[first], circled) {
			test.Errorf("%v: %s, %s", radius, got.URL(),
				said.addresses[first])
		}
	}
}

// the test: the centre is written to the places numbers.yaml says in the
// address and the name, and to every digit in the question.
func TestTheCentreIsWrittenToFivePlaces(test *testing.T) {
	get, said := serving(test, answer(fields, qr3Row))
	circle := near(test, overRA, overDec, recRadius)
	products, err := search(test, get, circle)
	if err != nil || len(products) != once {
		test.Fatalf("%v, %v", products, err)
	}
	got := products[first]
	if !strings.Contains(got.URL(), centre+"&") ||
		!strings.Contains(got.Name(), "_ra287.50086_dec-22.23481_") {
		test.Errorf("%s, %s", got.URL(), got.Name())
	}
	question, err := url.QueryUnescape(said.addresses[first])
	if err != nil || !strings.Contains(question, "287.5008604") ||
		!strings.Contains(question, "-22.234814") {
		test.Errorf("%q, %v", question, err)
	}
}

// the test: a place no image covers finds nothing, and is not an error.
func TestAPlaceNoImageCoversFindsNothing(test *testing.T) {
	products := found(test, answer(fields), recorded(test))
	if len(products) != none {
		test.Errorf("%d products", len(products))
	}
}

// bad is a row that is not an image's, and what the error should say.
type bad struct {
	row  []string
	says string
}

// Anti-tests: a row whose path is not an image's, or whose parts
// disagree, or whose numbers are not numbers, is an error that names
// what is wrong, and finds nothing.
func TestARowThatIsNotAnImageIsAnError(test *testing.T) {
	detector := strings.Replace(qr3Image, "D2_spx", "D3_spx", once)
	version := strings.Replace(qr3Image, "259.fits", "260.fits", once)
	for name, each := range map[string]bad{
		"not a path": {rowOf("x", "1", "2", "3", "4"), `"x"`},
		"two detectors": {rowOf(detector, "1", "2", "3", "4"),
			detector},
		"two processings": {rowOf(version, "1", "2", "3", "4"),
			version},
		"a calibration": {rowOf(darkImage, "1", "2", "3", "4"),
			"dark_D1"},
		"no time": {rowOf(qr3Image, "1", "2", "", "4"),
			"time_bounds_lower"},
		"no band": {rowOf(qr3Image, "", "2", "3", "4"),
			"energy_bounds_lower"},
		"a band that is not a number": {rowOf(qr3Image, "x", "2", "3",
			"4"), "energy_bounds_lower"},
	} {
		get, _ := serving(test, answer(fields, each.row))
		products, err := search(test, get, recorded(test))
		if products != nil || err == nil ||
			!strings.Contains(err.Error(), each.says) ||
			!strings.HasPrefix(err.Error(), adapter+": ") {
			test.Errorf("%s: %v, %v", name, products, err)
		}
	}
}

// rerun is a real image path whose processing is a rerun, as IRSA's
// table gave it over our star on 2026-11-06.
const rerun = "ibe/data/spherex/qr3/level2/2026W37_2A/l2b_retry-v27-2026-261/6/level2_2026W37_2A_0235_1D6_spx_l2b_retry-v27-2026-261.fits"

// the test: an image of a rerun processing is an image, named for its
// processing as it is written.
func TestARerunImageIsAnImage(test *testing.T) {
	row := rowOf(rerun, "1", "2", "3", "4")
	get, _ := serving(test, answer(fields, row))
	products, err := search(test, get, recorded(test))
	if err != nil || len(products) != once ||
		!strings.Contains(products[none].Name(),
			"2026W37_2A_0235_1D6_l2b_retry-v27-2026-261") {
		test.Errorf("%v, %v", products, err)
	}
}

// Anti-tests: an answer that lacks a column the archive reads is an
// error that names it, for each column.
func TestAnAnswerThatLacksAColumnIsAnError(test *testing.T) {
	for at, name := range fields {
		kept := slices.Delete(slices.Clone(fields), at, at+once)
		row := slices.Delete(slices.Clone(qr3Row), at, at+once)
		get, _ := serving(test, answer(kept, row))
		_, err := search(test, get, recorded(test))
		if err == nil || !strings.Contains(err.Error(), name) {
			test.Errorf("without %s: %v", name, err)
		}
	}
}

// Anti-tests: a web that fails, and a service that says the query
// failed, are errors that begin with the archive's name.
func TestAFailedQuestionIsAnError(test *testing.T) {
	down := errors.New("down")
	get := mocks.NewMockWeb(gomock.NewController(test))
	get.EXPECT().Get(gomock.Any()).Return(nil, int64(none), down)
	_, err := search(test, get, recorded(test))
	if !errors.Is(err, down) || !strings.HasPrefix(err.Error(),
		adapter+": ") {
		test.Errorf("down: %v", err)
	}
	refusal := []byte(`<VOTABLE version="1.3" xmlns="http://www.ivoa.` +
		`net/xml/VOTable/v1.3"><RESOURCE type="results"><INFO name=` +
		`"QUERY_STATUS" value="ERROR">UsageFault: BAD_REQUEST</INFO>` +
		`</RESOURCE></VOTABLE>`)
	refused, _ := serving(test, refusal)
	_, err = search(test, refused, recorded(test))
	if err == nil || !strings.Contains(err.Error(), "BAD_REQUEST") {
		test.Errorf("refused: %v", err)
	}
}

// Anti-test: a path whose observation would lead out of the folder is
// not an image's, so no search can make a product whose name does.
func TestNoPathMakesAnUnplainName(test *testing.T) {
	sneaky := strings.Replace(qr3Image, "2026W37_2A_0136_2D2",
		"../x", once)
	row := rowOf(sneaky, "1", "2", "3", "4")
	get, _ := serving(test, answer(fields, row))
	products, err := search(test, get, recorded(test))
	if products != nil || err == nil {
		test.Errorf("%v, %v", products, err)
	}
}

// fetchOf is a fetch of the QR3 image's cutout, served with content and
// no length, into a folder of its own; the folder and the error.
func fetchOf(test *testing.T, content []byte) (string, error) {
	test.Helper()
	return fetchRowOf(test, qr3Row, content)
}

// releasedAs is the QR3 row as if its image were of another release.
func releasedAs(release string) []string {
	row := slices.Clone(qr3Row)
	row[first] = strings.Replace(row[first], "/qr3/", "/"+release+"/", 1)
	return row
}

// fetchRowOf is a fetch of the cutout of a row's image, served with
// content and no length, into a folder of its own; the folder and the
// error.
func fetchRowOf(test *testing.T, row []string, content []byte) (string,
	error) {
	test.Helper()
	products := found(test, answer(fields, row), recorded(test))
	product := products[first]
	get := mocks.NewMockWeb(gomock.NewController(test))
	body := io.NopCloser(bytes.NewReader(content))
	get.EXPECT().Get(product.URL()).Return(body, int64(unsaid), nil)
	into := test.TempDir()
	_, err := celestialdata.Fetch(get, product, into)
	return into, err
}

// cutout is the recorded cutout's bytes: the first two units of a real
// cutout of 2026-11-06, the empty primary and the IMAGE.
func cutout(test *testing.T) []byte {
	test.Helper()
	content, err := os.ReadFile(filepath.Join("testdata",
		"cutout-qr3-D2-image.fits"))
	if err != nil || len(content) != imageEnd {
		test.Fatalf("%d bytes, %v", len(content), err)
	}
	return content
}

// lock is the mode that lets no one read a file. It's defined.
const lock = 0o000

// superuser is the superuser's user id. It's defined.
const superuser = 0

// locking is a body that, as the fetch reads it, makes the file it is
// being written to unreadable, so the check cannot read it back.
type locking struct {
	part    string
	content io.Reader
}

// Read locks the part and gives the content.
func (shut *locking) Read(into []byte) (int, error) {
	if err := os.Chmod(shut.part, lock); err != nil {
		return none, err
	}
	return shut.content.Read(into)
}

// Close is nothing to do.
func (*locking) Close() error { return nil }

// Anti-test: a file the check cannot read back is not kept, and the
// reason is the file system's.
func TestAFileThatCannotBeReadBackIsNotKept(test *testing.T) {
	if os.Getuid() == superuser {
		test.Skip("the superuser reads any file")
	}
	products := found(test, answer(fields, qr3Row), recorded(test))
	product := products[first]
	into := test.TempDir()
	part := filepath.Join(into, product.Name()+".part")
	body := &locking{part: part, content: bytes.NewReader(cutout(test))}
	get := mocks.NewMockWeb(gomock.NewController(test))
	get.EXPECT().Get(product.URL()).Return(body, int64(unsaid), nil)
	_, err := celestialdata.Fetch(get, product, into)
	kept, readErr := os.ReadDir(into)
	if !errors.Is(err, fs.ErrPermission) || readErr != nil ||
		len(kept) != none {
		test.Errorf("%v, %v, %d files", err, readErr, len(kept))
	}
}
