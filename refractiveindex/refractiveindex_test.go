package refractiveindex_test

import (
	"archive/zip"
	"bytes"
	"errors"
	"io"
	"os"
	"strconv"
	"strings"
	"testing"

	"github.com/janearc/celestial-data"
	"github.com/janearc/celestial-data/internal/replay"
	"github.com/janearc/celestial-data/mocks"
	"github.com/janearc/celestial-data/refractiveindex"
	"go.uber.org/mock/gomock"
)

// What the tests ask of New and its one product.
const (
	adapter = "refractiveindex"                  // defined: the archive's name
	release = "v2026-05-24"                      // defined: the release's tag
	zipName = "rii-database-2026-05-24.zip"      // defined: the release's zip
	doi     = "10.1038/s41597-023-02898-2"       // defined: the data descriptor
	archive = "https://github.com/polyanskiy/" + // defined: where the release serves it
		"refractiveindex.info-database/releases/download/" +
		release + "/" + zipName
	// archiveBytes is the release zip's length as GitHub served it.
	archiveBytes = 13440092 // defined: numbers.yaml, fetched 2026-11-11
	// sum is the sum the release page gives the zip's bytes; a body of
	// other bytes is refused.
	sum   = "6cdebebd17fe02b6dc2b03189e15e6c689afc6619c558e1df7b9ac307b6d14eb" // defined: the release page
	first = 0                                                                  // defined: the first product
	lone  = 1                                                                  // defined: one product
)

// The excerpt of the real archive, and where its pages sit.
const (
	excerpt = "testdata/refractiveindex-excerpt.zip"
	// main is the shelf of simple inorganic materials, as the catalog
	// takes its name.
	main = "main"
)

// Gold, as Johnson and Christy 1972 measured it: the page the tests read
// in full detail. Every value below was read by eye from
// database/data/main/Au/nk/Johnson.yml in the excerpt, which is the
// file the release holds; the line is the file's line.
const (
	goldBook = "Au"      // defined: line 10 of catalog-nk.yml, BOOK: Au
	goldPage = "Johnson" // defined: line 14 of catalog-nk.yml, PAGE: Johnson
	// the file's REFERENCES, its three lines, 6 to 8, as one text
	goldReferences = "P. B. Johnson and R. W. Christy.\n" +
		"Optical constants of the noble metals.\n" +
		"<a href=\"https://doi.org/10.1103/PhysRevB.6.4370\">" +
		"<i>Phys. Rev. B</i> <b>6</b>, 4370-4379 (1972)</a>\n"
	// the file's COMMENTS, line 10
	goldComments = "Room temperature\n"
	// the file's rows: 49 of them, lines 14 to 62
	goldRows        = 49     // defined: lines 14 to 62 of the file
	goldMicrons1    = 0.1879 // defined: line 14, the first row's wavelength
	goldN1          = 1.28   // defined: line 14, its n
	goldK1          = 1.188  // defined: line 14, its k
	goldMicrons2    = 0.1916 // defined: line 15, the second row's wavelength
	goldN2          = 1.32   // defined: line 15, its n
	goldK2          = 1.203  // defined: line 15, its k
	goldMicrons48   = 1.3930 // defined: line 60, the third from last row
	goldN48         = 0.43   // defined: line 60, its n
	goldK48         = 9.519  // defined: line 60, its k
	goldMicronsLast = 1.9370 // defined: line 62, the last row's wavelength
	goldNLast       = 0.92   // defined: line 62, its n
	goldKLast       = 13.78  // defined: line 62, its k
)

// entry is one file of a zip made for a test.
type entry struct {
	name string
	body string
}

// archiveOf is a zip of entries.
func archiveOf(t *testing.T, entries ...entry) *bytes.Reader {
	t.Helper()
	var made bytes.Buffer
	writer := zip.NewWriter(&made)
	for _, each := range entries {
		into, err := writer.Create(each.name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := into.Write([]byte(each.body)); err != nil {
			t.Fatal(err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	return bytes.NewReader(made.Bytes())
}

// excerptOf is the excerpt of the archive, read.
func excerptOf(t *testing.T) *bytes.Reader {
	t.Helper()
	raw, err := os.ReadFile(excerpt)
	if err != nil {
		t.Fatal(err)
	}
	return bytes.NewReader(raw)
}

// read is the table of a page in the excerpt.
func read(t *testing.T, shelf, book, page string) refractiveindex.Table {
	t.Helper()
	excerpted := excerptOf(t)
	table, err := refractiveindex.Read(excerpted, excerpted.Size(), shelf,
		book, page)
	if err != nil {
		t.Fatalf("%s/%s/%s: %v", shelf, book, page, err)
	}
	return table
}

// readFrom is the error Read gives for a page in an archive.
func readFrom(archive io.ReaderAt, size int64, shelf, book,
	page string) error {
	table, err := refractiveindex.Read(archive, size, shelf, book, page)
	if err == nil {
		return errors.New("a table came back")
	}
	if table.Rows() != nil {
		return errors.New("a table came back")
	}
	return err
}

// row is a row as the files write it: the wavelength in micrometres,
// then n, then k.
func row(micrometres, n, k float64) refractiveindex.Row {
	return refractiveindex.Row{Micrometres: micrometres, N: n, K: k}
}

// Gold is found by its shelf, book and page, and its rows are the file's
// own: the first, the second, one from the end and the last, and their
// count. The file's REFERENCES and COMMENTS come with them, as the file
// writes them.
func TestReadFindsGoldByShelfBookAndPage(t *testing.T) {
	table := read(t, main, goldBook, goldPage)
	if table.References() != goldReferences {
		t.Errorf("references %q", table.References())
	}
	if table.Comments() != goldComments {
		t.Errorf("comments %q", table.Comments())
	}
	rows := table.Rows()
	if len(rows) != goldRows {
		t.Fatalf("%d rows, want %d", len(rows), goldRows)
	}
	for _, each := range []struct {
		why  string
		got  refractiveindex.Row
		want refractiveindex.Row
	}{
		{"line 14, the first", rows[0], row(goldMicrons1, goldN1, goldK1)},
		{"line 15, the second", rows[1], row(goldMicrons2, goldN2, goldK2)},
		{"line 60, the third from last", rows[goldRows-3],
			row(goldMicrons48, goldN48, goldK48)},
		{"line 62, the last", rows[goldRows-1],
			row(goldMicronsLast, goldNLast, goldKLast)},
	} {
		if each.got != each.want {
			t.Errorf("%s: %+v, want %+v", each.why, each.got, each.want)
		}
	}
}

// Every page the excerpt holds reads, and its first and last rows are
// the file's own. The row counts and the rows were read from each file
// in the excerpt; the metals asked for are gold, silver, aluminium,
// beryllium and titanium, by Johnson and Christy and by Rakic's fits.
func TestReadReadsEveryMetalPageOfTheExcerpt(t *testing.T) {
	for _, each := range []struct {
		book, page  string
		rows        int
		first, last refractiveindex.Row
	}{
		// gold, database/data/main/Au/nk/Johnson.yml: 49 rows, lines
		// 14 to 62; first line 14, last line 62
		{"Au", "Johnson", 49, row(0.1879, 1.28, 1.188),
			row(1.9370, 0.92, 13.78)},
		// gold, Au/nk/Rakic-BB.yml: 200 rows; first "2.4797e-01
		// 1.4943e+00 1.9575e+00", last "6.1992e+00 5.1922e+00
		// 3.8728e+01"
		{"Au", "Rakic-BB", 200, row(0.24797, 1.4943, 1.9575),
			row(6.1992, 5.1922, 38.728)},
		// gold, Au/nk/Rakic-LD.yml: 200 rows; first "2.4797e-01
		// 1.0941e+00 1.5478e+00", last "6.1992e+00 5.0974e+00
		// 3.8118e+01"
		{"Au", "Rakic-LD", 200, row(0.24797, 1.0941, 1.5478),
			row(6.1992, 5.0974, 38.118)},
		// silver, Ag/nk/Johnson.yml: 49 rows; first line 14 "0.1879
		// 1.07 1.212", last line 62 "1.9370 0.24 14.08"
		{"Ag", "Johnson", 49, row(0.1879, 1.07, 1.212),
			row(1.9370, 0.24, 14.08)},
		// silver, Ag/nk/Rakic-BB.yml: 200 rows; first "2.4797e-01
		// 8.4863e-01 1.1920e+00", last "1.2398e+01 1.7540e+01
		// 7.5326e+01"
		{"Ag", "Rakic-BB", 200, row(0.24797, 0.84863, 1.1920),
			row(12.398, 17.540, 75.326)},
		// silver, Ag/nk/Rakic-LD.yml: 200 rows; first "2.4797e-01
		// 4.4265e-01 1.1737e+00", last "1.2398e+01 1.7485e+01
		// 7.6626e+01"
		{"Ag", "Rakic-LD", 200, row(0.24797, 0.44265, 1.1737),
			row(12.398, 17.485, 76.626)},
		// aluminium, Al/nk/Rakic.yml: 206 rows; first "1.2399E-04
		// 9.999946E-01 8.2410E-08", last "2.0000E+02 4.2396E+02
		// 4.8370E+02"
		{"Al", "Rakic", 206, row(1.2399e-04, 9.999946e-01, 8.2410e-08),
			row(2.0000e+02, 4.2396e+02, 4.8370e+02)},
		// aluminium, Al/nk/Rakic-BB.yml: 200 rows; first "6.1992e-02
		// 6.6415e-01 9.2039e-03", last "2.4797e+02 4.7349e+02
		// 5.2458e+02"
		{"Al", "Rakic-BB", 200, row(6.1992e-02, 6.6415e-01, 9.2039e-03),
			row(2.4797e+02, 4.7349e+02, 5.2458e+02)},
		// aluminium, Al/nk/Rakic-LD.yml: 1000 rows; first "6.1992e-02
		// 6.6397e-01 9.4537e-03", last "2.4797e+02 4.7220e+02
		// 5.2302e+02"
		{"Al", "Rakic-LD", 1000, row(6.1992e-02, 6.6397e-01, 9.4537e-03),
			row(2.4797e+02, 4.7220e+02, 5.2302e+02)},
		// beryllium, Be/nk/Rakic-BB.yml: 200 rows; first "2.4797e-01
		// 1.6444e+00 2.9702e+00", last "6.1992e+01 9.4075e+01
		// 1.6042e+02"
		{"Be", "Rakic-BB", 200, row(0.24797, 1.6444, 2.9702),
			row(61.992, 94.075, 160.42)},
		// beryllium, Be/nk/Rakic-LD.yml: 1000 rows; first "2.4797e-01
		// 1.7010e+00 3.2089e+00", last "6.1992e+01 9.5713e+01
		// 1.6347e+02"
		{"Be", "Rakic-LD", 1000, row(0.24797, 1.7010, 3.2089),
			row(61.992, 95.713, 163.47)},
		// titanium, Ti/nk/Johnson.yml: 49 rows; first line 14 "0.188
		// 1.10 1.62", last line 62 "1.937 3.51 5.19"
		{"Ti", "Johnson", 49, row(0.188, 1.10, 1.62),
			row(1.937, 3.51, 5.19)},
		// titanium, Ti/nk/Rakic-BB.yml: 200 rows; first "2.4797e-01
		// 1.8885e-01 1.4035e+00", last "3.0996e+01 2.4653e+01
		// 3.9849e+01"
		{"Ti", "Rakic-BB", 200, row(0.24797, 0.18885, 1.4035),
			row(30.996, 24.653, 39.849)},
		// titanium, Ti/nk/Rakic-LD.yml: 1000 rows; first "2.4797e-01
		// 4.4635e-01 1.5069e+00", last "3.0996e+01 2.5227e+01
		// 3.8632e+01"
		{"Ti", "Rakic-LD", 1000, row(0.24797, 0.44635, 1.5069),
			row(30.996, 25.227, 38.632)},
	} {
		table := read(t, main, each.book, each.page)
		rows := table.Rows()
		if len(rows) != each.rows {
			t.Fatalf("%s/%s: %d rows, want %d", each.book, each.page,
				len(rows), each.rows)
		}
		if rows[0] != each.first || rows[len(rows)-1] != each.last {
			t.Errorf("%s/%s: first %+v last %+v; want %+v and %+v",
				each.book, each.page, rows[0], rows[len(rows)-1],
				each.first, each.last)
		}
	}
}

// A page that tabulates n alone has no k, and its rows say so with zero.
// The page is MoSe2's extraordinary ray as Pruszynska-Karbownik et al.
// 2026 measured it, database/data/main/MoSe2/nk/Pruszynska-Karbownik-e.yml
// in the excerpt: 1084 rows, lines 14 to 1097 of the file.
func TestReadGivesAPageOfTabulatedNAKOfZero(t *testing.T) {
	table := read(t, main, "MoSe2", "Pruszynska-Karbownik-e")
	rows := table.Rows()
	if len(rows) != 1084 { // defined: lines 14 to 1097 of the file
		t.Fatalf("%d rows", len(rows))
	}
	// the file's line 14, its first row: wavelength and n, no k
	if rows[0] != row(0.193, 4.552, 0) {
		t.Errorf("first %+v", rows[0])
	}
	// the file's line 1097, its last row
	if rows[1083] != row(1.690, 2.816, 0) {
		t.Errorf("last %+v", rows[1083])
	}
	// the file's COMMENTS, line 10
	if table.Comments() != "MBE-grown 42-nm layer; room temperature. "+
		"Extraordinary ray (e).\n" {
		t.Errorf("comments %q", table.Comments())
	}
}

// Anti-tests: Read refuses what the catalog does not hold, by name, and
// a page whose data file the archive does not hold, though the catalog
// names it.
func TestReadRefusesWhatTheArchiveDoesNotHold(t *testing.T) {
	file := excerptOf(t)
	for _, each := range []struct {
		why               string
		shelf, book, page string
		saying            string
	}{
		{"no shelf", "fictional", "Au", "Johnson",
			"refractiveindex holds no shelf fictional"},
		{"no book on the shelf", main, "metals", "gold",
			"refractiveindex holds no book metals on shelf main"},
		{"no page in the book", main, "Au", "Choi",
			"refractiveindex holds no page Choi in book Au"},
		{"no data file", main, "Cu", "Johnson",
			"refractiveindex holds no database/data/main/Cu/nk/Johnson.yml"},
	} {
		err := readFrom(file, file.Size(), each.shelf, each.book, each.page)
		if !errors.Is(err, celestialdata.Refused()) ||
			!strings.Contains(err.Error(), each.saying) {
			t.Errorf("%s: %v", each.why, err)
		}
	}
}

// Anti-tests: a formula page is refused by name, since its constants are
// coefficients, not rows. The page is spinel as Tropf and Thomas 1991
// give it, database/data/main/MgAl2O4/nk/Tropf.yml in the excerpt, whose
// DATA says "type: formula 1" on line 10.
func TestReadRefusesAFormulaPageByItsName(t *testing.T) {
	file := excerptOf(t)
	err := readFrom(file, file.Size(), main, "MgAl2O4", "Tropf")
	if !errors.Is(err, celestialdata.Refused()) ||
		!strings.Contains(err.Error(),
			"refractiveindex page Tropf is a formula 1, "+
				"not tabulated n and k") {
		t.Errorf("%v", err)
	}
}

// The catalog and data file a test's own archive holds, written in the
// database's own format: the catalog's lines are as catalog-nk.yml
// writes the Au book's Johnson page, and the data file's header is as
// the page's own file writes it.
const (
	catalogSnippet = `- SHELF: main
  name: "MAIN - simple inorganic materials"
  content:
    - BOOK: Au
      name: "Au (Gold)"
      content:
        - PAGE: Johnson
          name: "Johnson and Christy 1972: n,k 0.188-1.94 um"
          data: main/Au/nk/Johnson.yml
`
	// rowsOf begins a data file whose rows a test writes after it.
	rowsOf = "REFERENCES: |\n    P. B. Johnson and R. W. Christy.\n" +
		"DATA:\n  - type: tabulated nk\n    data: |\n"
)

// Anti-tests: an archive that is no zip is an error that names the
// archive and wraps what the standard library said, and a row that is
// no number is an error that names the file.
func TestReadSaysWhyAFileWillNotRead(t *testing.T) {
	notZip := bytes.NewReader([]byte("this is not a zip file"))
	notNumber := archiveOf(t,
		entry{name: "database/catalog-nk.yml", body: catalogSnippet},
		entry{name: "database/data/main/Au/nk/Johnson.yml",
			body: rowsOf + "        0.1879 1.28 wide\n"})
	for _, each := range []struct {
		why    string
		made   *bytes.Reader
		wrap   error
		saying string
	}{
		{"not a zip", notZip, zip.ErrFormat, "refractiveindex: "},
		{"row not a number", notNumber, strconv.ErrSyntax,
			"refractiveindex: database/data/main/Au/nk/Johnson.yml"},
	} {
		err := readFrom(each.made, each.made.Size(), main, "Au", "Johnson")
		if !errors.Is(err, each.wrap) ||
			!strings.Contains(err.Error(), each.saying) {
			t.Errorf("%s: %v", each.why, err)
		}
	}
}

// Anti-tests: an archive with no catalog is refused by name, and a data
// file that cannot be read as a table of n and k is refused by name: an
// unknown type, a row of the wrong count, and no type at all. A
// COMMENTS written as a quoted sentence, not a block, is read as it is.
func TestReadRefusesWhatADataFileCannotGive(t *testing.T) {
	for _, each := range []struct {
		why    string
		made   *bytes.Reader
		saying string
	}{
		{"no catalog", archiveOf(t,
			entry{name: "database/data/main/Au/nk/Johnson.yml",
				body: rowsOf}),
			"refractiveindex holds no database/catalog-nk.yml"},
		{"unknown type", archiveOf(t,
			entry{name: "database/catalog-nk.yml", body: catalogSnippet},
			entry{name: "database/data/main/Au/nk/Johnson.yml",
				body: "DATA:\n  - type: tabulated k\n    data: |\n" +
					"        0.1879 1.212\n"}),
			"refractiveindex page Johnson is a tabulated k, " +
				"not tabulated n and k"},
		{"row of the wrong count", archiveOf(t,
			entry{name: "database/catalog-nk.yml", body: catalogSnippet},
			entry{name: "database/data/main/Au/nk/Johnson.yml",
				body: rowsOf + "        0.1879 1.28\n"}),
			"refractiveindex page Johnson has a row of 2 numbers, not 3"},
		{"no type in DATA", archiveOf(t,
			entry{name: "database/catalog-nk.yml", body: catalogSnippet},
			entry{name: "database/data/main/Au/nk/Johnson.yml",
				body: "DATA:\n    data: |\n        0.1879 1.28 1.188\n"}),
			"refractiveindex page Johnson gives no type in its DATA"},
	} {
		err := readFrom(each.made, each.made.Size(), main, "Au", "Johnson")
		if !errors.Is(err, celestialdata.Refused()) ||
			!strings.Contains(err.Error(), each.saying) {
			t.Errorf("%s: %v", each.why, err)
		}
	}
}

// A COMMENTS written as a quoted sentence on its own line, as two files
// of the database write it, is read as the sentence it is.
func TestReadReadsACommentOfOneLine(t *testing.T) {
	made := archiveOf(t,
		entry{name: "database/catalog-nk.yml", body: catalogSnippet},
		entry{name: "database/data/main/Au/nk/Johnson.yml",
			body: "REFERENCES: |\n    P. B. Johnson and R. W. Christy.\n" +
				"COMMENTS: \"WSe<sub>2</sub> nanoparticles\"\n" +
				"DATA:\n  - type: tabulated nk\n    data: |\n" +
				"        0.1879 1.28 1.188\n"})
	table, err := refractiveindex.Read(made, made.Size(), main, "Au",
		"Johnson")
	if err != nil {
		t.Fatal(err)
	}
	if table.Comments() != "WSe<sub>2</sub> nanoparticles" {
		t.Errorf("comments %q", table.Comments())
	}
	if len(table.Rows()) != 1 {
		t.Errorf("%d rows", len(table.Rows()))
	}
}

// databaseOf is the one product New holds.
func databaseOf(t *testing.T) *celestialdata.Product {
	t.Helper()
	database, err := refractiveindex.New()
	if err != nil {
		t.Fatal(err)
	}
	found, err := database.Find(replay.Asked(t))
	if err != nil || len(found) != lone {
		t.Fatalf("%d products, %v", len(found), err)
	}
	return found[first]
}

// The database is one product: the release zip at GitHub, with the
// length the numbers file gives, held to the sum the release page gives.
// A fetch that brings another length is refused, and one whose bytes
// have another sum is refused and leaves nothing behind, neither the
// zip nor its .part.
func TestTheDatabaseIsTheReleaseZipHeldToItsSum(t *testing.T) {
	database := databaseOf(t)
	if database.Archive() != adapter || database.ID() != release ||
		database.Name() != zipName || database.URL() != archive ||
		!strings.Contains(database.About(), "CC0 1.0") ||
		!strings.Contains(database.About(), doi) {
		t.Errorf("%s %s %s %s %q", database.Archive(), database.ID(),
			database.Name(), database.URL(), database.About())
	}
	const body = "short"
	get := mocks.NewMockWeb(gomock.NewController(t))
	reply := io.NopCloser(strings.NewReader(body))
	get.EXPECT().Get(database.URL()).Return(reply, int64(len(body)), nil)
	_, err := celestialdata.Fetch(get, database, t.TempDir())
	want := strconv.Itoa(len(body)) + " bytes came, the archive said " +
		strconv.Itoa(archiveBytes)
	if err == nil || !strings.Contains(err.Error(), want) {
		t.Errorf("fetch of a short body: %v", err)
	}

	// the sum's own words: bytes of the archive's length that sum to
	// something else
	var other bytes.Buffer
	for other.Len() < archiveBytes {
		other.WriteString("no sum of the release's bytes")
	}
	other.Truncate(archiveBytes)
	summed := mocks.NewMockWeb(gomock.NewController(t))
	summed.EXPECT().Get(database.URL()).Return(
		io.NopCloser(bytes.NewReader(other.Bytes())),
		int64(archiveBytes), nil)
	into := t.TempDir()
	_, err = celestialdata.Fetch(summed, database, into)
	if err == nil || !strings.Contains(err.Error(), "sha256") ||
		!strings.Contains(err.Error(), sum) {
		t.Errorf("fetch of another sum: %v", err)
	}
	if held, readErr := os.ReadDir(into); readErr != nil || len(held) != 0 {
		t.Errorf("left behind %v, %v", held, readErr)
	}
}

// New is a fixed archive of the database, asked only by name, and it
// finds the same for every circle.
func TestNewHoldsTheDatabaseForEveryCircle(t *testing.T) {
	database, err := refractiveindex.New()
	if err != nil {
		t.Fatal(err)
	}
	if database.Name() != adapter || !database.ByNameOnly() {
		t.Errorf("%q, by name only %v", database.Name(),
			database.ByNameOnly())
	}
	var _ celestialdata.ByName = database
	held := databaseOf(t)
	north, east := 60.0, 10.0 // defined: a far declination and right ascension
	pole, err := celestialdata.NewPosition(east, north)
	if err != nil {
		t.Fatal(err)
	}
	far, err := celestialdata.NewNear(pole, 0.1) // defined: a radius, in degrees
	if err != nil {
		t.Fatal(err)
	}
	for _, circle := range []*celestialdata.Near{far, replay.Asked(t)} {
		found, err := database.Find(circle)
		if err != nil || len(found) != lone || *found[first] != *held {
			t.Errorf("%v, %v", found, err)
		}
	}
}

// Every number the code asks the numbers file for is in it: Value panics
// on one it lacks, so New and Read running through is the check.
func TestEveryNumberTheCodeAsksForIsInTheFile(t *testing.T) {
	if _, err := refractiveindex.New(); err != nil {
		t.Errorf("New: %v", err)
	}
	read(t, main, goldBook, goldPage)
}
