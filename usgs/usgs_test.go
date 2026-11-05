package usgs_test

import (
	"archive/zip"
	"bufio"
	"bytes"
	"errors"
	"io"
	"math"
	"os"
	"strconv"
	"strings"
	"testing"

	"github.com/janearc/celestial-data"
	"github.com/janearc/celestial-data/internal/replay"
	"github.com/janearc/celestial-data/mocks"
	"github.com/janearc/celestial-data/usgs"
	"go.uber.org/mock/gomock"
)

// The spectra in the excerpt of the archive, by the names Read takes.
const (
	seawater = "Seawater_Open_Ocean_SW2_lwch_BECKa_AREF"
	grass    = "Grass_dry.5+.5green_AMX28_BECKa_AREF"
)

// The excerpt of the real archive, and the folder its files sit under.
const (
	excerpt = "testdata/splib07a-excerpt.zip"
	folder  = "ASCIIdata_splib07a/"
	// channels is the Beckman instrument's wavelength file, as the
	// archive names it.
	channels = "splib07a_Wavelengths_BECK_Beckman_0.2-3.0_microns.txt"
)

// Values read by eye from the excerpt's text files. The line is the
// file's line, counting its title as the first. Wavelengths are the
// wavelength file's micrometres times a thousand.
const (
	// line 100 of the seawater file and of the wavelength file
	seaNm100    = 509.29999   // defined: 5.0929999e-001 micrometres
	seaValue100 = 0.031298865 // defined: 3.1298865e-002
	// lines 300 and 301 of the same files
	seaNm300    = 1203.5      // defined: 1.2035000e+000 micrometres
	seaNm301    = 1208.5      // defined: 1.2085000e+000 micrometres
	seaValue300 = 0.019330578 // defined: 1.9330578e-002
	seaValue301 = 0.019323122 // defined: 1.9323122e-002
	// line 481, the last of both
	// A hair inside it, since a channel's own wavelength is rounded
	// twice, once by the file's digits and once by the thousand.
	seaNmLast    = 2976.0000    // defined: 2.9760001e+000 micrometres, less
	seaValueLast = 0.0039714500 // defined: 3.9714500e-003
	// the grass file's first measured channel, line 11
	grassNm11    = 263.59999    // defined: 2.6359999e-001 micrometres
	grassValue11 = 0.0063838544 // defined: 6.3838544e-003
	// lines 54 and 58 of the grass file. Lines 55 to 57 between them
	// hold no measurement.
	grassNm54    = 405.09999   // defined: 4.0509999e-001 micrometres
	grassNm58    = 412.80001   // defined: 4.1280001e-001 micrometres
	grassValue54 = 0.038139164 // defined: 3.8139164e-002
	grassValue58 = 0.045906808 // defined: 4.5906808e-002
)

// Where the tests ask, all in nanometres.
const (
	quarter   = 0.25   // defined: a quarter of the way along a step
	half      = 0.5    // defined: half of the way
	nothing   = 0.0    // defined: what At gives with false
	up        = 1      // defined: the sign of positive infinity
	down      = -1     // defined: the sign of negative infinity
	leading   = 230.0  // defined: inside the grass file's leading gap
	belowAll  = 100.0  // defined: below every Beckman channel, 205.1
	aboveAll  = 4000.0 // defined: above every Beckman channel, 2976
	tolerance = 1e-9   // defined: the files carry eight digits
)

// The words a file is written in.
const (
	// noData is how the library's files write a channel with no
	// measurement.
	noData = "-1.2300000e+034" // defined: see sources.yaml
	title  = "splib07a Record=1: made for a test\n"
	named  = "Test_Sample_BECKa_AREF" // defined: a name of Beckman kind
	// archiveBytes is the archive's length as ScienceBase served it.
	archiveBytes = 21812828 // defined: sources.yaml, fetched 2026-11-02
	// tooLong is a line longer than the scanner reads, 64 KiB.
	tooLong = 70000 // defined: more than bufio.MaxScanTokenSize
)

// What a test asks of Library and New.
const (
	adapter = "usgs"                   // defined: the archive's name
	doi     = "10.5066/F7RR1WDJ"       // defined: the data release's DOI
	zipName = "ASCIIdata_splib07a.zip" // defined: the archive's file
	first   = 0                        // defined: the first product
	lone    = 1                        // defined: one product
	north   = 60                       // defined: a far declination
	east    = 10                       // defined: a far right ascension
	wide    = 0.1                      // defined: a radius, in degrees
)

// entry is one file of a zip made for a test.
type entry struct {
	name   string
	body   string
	broken bool
}

// archiveOf is a zip of entries. A broken entry has a compression the
// zip package has no reader for, so it will not open.
func archiveOf(t *testing.T, entries ...entry) *bytes.Reader {
	t.Helper()
	var made bytes.Buffer
	writer := zip.NewWriter(&made)
	for _, each := range entries {
		var err error
		var into io.Writer
		if each.broken {
			into, err = writer.CreateRaw(&zip.FileHeader{Name: each.name,
				Method: unknownMethod})
		} else {
			into, err = writer.Create(each.name)
		}
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

// unknownMethod is a compression method the zip package has no reader
// for. It's defined: the zip format leaves it to no one.
const unknownMethod = 99

// pair is the two files a spectrum needs: its own and its channels'.
func pair(t *testing.T, spectrum, wavelengths string) *bytes.Reader {
	t.Helper()
	return archiveOf(t,
		entry{name: folder + "splib07a_" + named + ".txt", body: spectrum},
		entry{name: folder + channels, body: wavelengths})
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

// read is the spectrum of a name in the excerpt.
func read(t *testing.T, name string) *usgs.Spectrum {
	t.Helper()
	excerpted := excerptOf(t)
	spectrum, err := usgs.Read(excerpted, excerpted.Size(), name)
	if err != nil {
		t.Fatalf("%s: %v", name, err)
	}
	return spectrum
}

// near is whether two reflectances are the same to the tolerance.
func near(got, want float64) bool {
	return math.Abs(got-want) <= tolerance
}

// middle is the number halfway between two.
func middle(low, high float64) float64 { return low + (high-low)*half }

// At a channel it is the channel's value, and between two channels it is
// drawn straight. The numbers are read from the files' own text.
func TestAtDrawsStraightBetweenChannels(t *testing.T) {
	sea := read(t, seawater)
	if sea.Name() != seawater {
		t.Errorf("name %q", sea.Name())
	}
	step := seaNm301 - seaNm300
	drop := seaValue301 - seaValue300
	for _, each := range []struct {
		nm, want float64
	}{
		{seaNm100, seaValue100},
		{seaNmLast, seaValueLast},
		{seaNm300, seaValue300},
		{seaNm301, seaValue301},
		{seaNm300 + quarter*step, seaValue300 + quarter*drop},
		{middle(seaNm300, seaNm301), middle(seaValue300, seaValue301)},
	} {
		got, held := sea.At(each.nm)
		if !held || !near(got, each.want) {
			t.Errorf("At(%v) = %v, %v; want %v", each.nm, got, held,
				each.want)
		}
	}
}

// A channel with no measurement is left out of a spectrum. The line is
// drawn across it, and where the gap begins the spectrum there is none.
func TestAtAGapIsLeftOutAndSpannedOrNone(t *testing.T) {
	dry := read(t, grass)
	got, held := dry.At(grassNm11)
	if !held || !near(got, grassValue11) {
		t.Errorf("first channel: %v, %v", got, held)
	}
	got, held = dry.At(leading)
	if held {
		t.Errorf("At(%v) = %v inside the leading gap", leading, got)
	}
	span := middle(grassNm54, grassNm58)
	want := middle(grassValue54, grassValue58)
	got, held = dry.At(span)
	if !held || !near(got, want) {
		t.Errorf("across the gap: At(%v) = %v, %v; want %v", span, got,
			held, want)
	}
}

// Anti-tests: outside the measured span there is no value, and so for a
// wavelength that is no number.
func TestAtOutsideTheSpanIsNone(t *testing.T) {
	sea := read(t, seawater)
	for _, nm := range []float64{belowAll, aboveAll, math.NaN(),
		math.Inf(up), math.Inf(down)} {
		if got, held := sea.At(nm); held || got != nothing {
			t.Errorf("At(%v) = %v, %v", nm, got, held)
		}
	}
}

// A spectrum is drawn by its channels, so a spectrum with none has no
// value anywhere, and two channels at one wavelength give the first.
func TestAtOnAThinOrRepeatedSpectrum(t *testing.T) {
	const (
		one      = "1.0000000e+000\n"
		two      = "2.0000000e+000\n"
		low      = "2.5000000e-001\n"
		high     = "5.0000000e-001\n"
		atOne    = 1000 // defined: one micrometre in nanometres
		lowValue = 0.25 // defined: the value of low, written above
	)
	empty := pair(t, title, title)
	none, err := usgs.Read(empty, empty.Size(), named)
	if err != nil {
		t.Fatal(err)
	}
	if got, held := none.At(atOne); held {
		t.Errorf("empty spectrum: %v", got)
	}
	twice := pair(t, title+low+high+low, title+one+one+two)
	same, err := usgs.Read(twice, twice.Size(), named)
	if err != nil {
		t.Fatal(err)
	}
	if got, held := same.At(atOne); !held || !near(got, lowValue) {
		t.Errorf("repeated channel: %v, %v", got, held)
	}
}

// A channel whose wavelength is the no-data value is left out as well.
func TestAGapInTheWavelengthsIsLeftOut(t *testing.T) {
	made := pair(t, title+"1.0e+000\n2.0e+000\n3.0e+000\n",
		title+"1.0e+000\n"+noData+"\n3.0e+000\n")
	spectrum, err := usgs.Read(made, made.Size(), named)
	if err != nil {
		t.Fatal(err)
	}
	const (
		midway      = 2000 // defined: halfway between the 1 and 3 micrometres
		beyond      = 3001 // defined: past the 3 micrometre channel, in nm
		acrossValue = 2    // defined: halfway from value 1 to value 3
	)
	if got, held := spectrum.At(midway); !held || !near(got, acrossValue) {
		t.Errorf("across the gap: %v, %v", got, held)
	}
	if _, held := spectrum.At(beyond); held {
		t.Errorf("beyond the last channel kept")
	}
}

// Anti-tests: Read refuses what the archive does not hold, in the
// package's words.
func TestReadRefusesWhatTheArchiveDoesNotHold(t *testing.T) {
	wavelengthsOnly := archiveOf(t, entry{name: folder + channels,
		body: title})
	spectrumOnly := archiveOf(t, entry{name: folder + "splib07a_" + named +
		".txt", body: title})
	file := excerptOf(t)
	for _, each := range []struct {
		why    string
		got    error
		saying string
	}{
		{"absent", readFrom(file, file.Size(), "Nothing_BECKa_AREF"),
			"usgs holds no spectrum Nothing_BECKa_AREF"},
		{"no wavelengths", readFrom(spectrumOnly, spectrumOnly.Size(),
			named), "usgs holds no channels for " + named},
		{"no spectrum", readFrom(wavelengthsOnly, wavelengthsOnly.Size(),
			named), "usgs holds no spectrum " + named},
		{"one part", readFrom(file, file.Size(), "Seawater"),
			"usgs knows no instrument in the name Seawater"},
		{"no instrument", readFrom(file, file.Size(), "Water_XYZ_AREF"),
			"usgs knows no instrument in the name Water_XYZ_AREF"},
		{"no name", readFrom(file, file.Size(), ""),
			"usgs knows no instrument in the name"},
	} {
		if !errors.Is(each.got, celestialdata.Refused()) ||
			!strings.Contains(each.got.Error(), each.saying) {
			t.Errorf("%s: %v", each.why, each.got)
		}
	}
}

// A name of two parts, the instrument's code and the kind of reflectance,
// is a name whose instrument is known.
func TestReadKnowsTheInstrumentOfANameOfTwoParts(t *testing.T) {
	const short = "BECKa_AREF" // defined: the least a name can be
	made := archiveOf(t,
		entry{name: folder + "splib07a_" + short + ".txt",
			body: title + "1.0e+000\n"},
		entry{name: folder + channels, body: title + "1.0e+000\n"})
	spectrum, err := usgs.Read(made, made.Size(), short)
	if err != nil || spectrum.Name() != short {
		t.Fatalf("%v, %v", spectrum, err)
	}
}

// readFrom is the error Read gives for a name in an archive.
func readFrom(archive io.ReaderAt, size int64, name string) error {
	spectrum, err := usgs.Read(archive, size, name)
	if spectrum != nil {
		return errors.New("a spectrum came back")
	}
	return err
}

// Anti-tests: a spectrum of other length than its channels is refused.
func TestReadRefusesValuesThatDoNotMatchChannels(t *testing.T) {
	made := pair(t, title+"1.0e+000\n2.0e+000\n", title+"1.0e+000\n")
	err := readFrom(made, made.Size(), named)
	if !errors.Is(err, celestialdata.Refused()) || !strings.Contains(
		err.Error(), "usgs spectrum "+named+" has 2 values for 1 channels") {
		t.Errorf("%v", err)
	}
}

// Anti-tests: files that will not read are errors that name the archive
// and the file, and wrap what the standard library said.
func TestReadSaysWhyAFileWillNotRead(t *testing.T) {
	spectrum := folder + "splib07a_" + named + ".txt"
	wavelengths := folder + channels
	good := title + "1.0e+000\n"
	long := title + strings.Repeat("1", tooLong) + "\n"
	notZip := bytes.NewReader([]byte("this is not a zip file"))
	for _, each := range []struct {
		why    string
		made   *bytes.Reader
		wrap   error
		saying string
	}{
		{"not a zip", notZip, zip.ErrFormat, "usgs: "},
		{"spectrum will not open", archiveOf(t,
			entry{name: spectrum, body: good, broken: true},
			entry{name: wavelengths, body: good}), zip.ErrAlgorithm,
			"usgs: " + spectrum},
		{"wavelengths will not open", archiveOf(t,
			entry{name: spectrum, body: good},
			entry{name: wavelengths, body: good, broken: true}),
			zip.ErrAlgorithm, "usgs: " + wavelengths},
		{"spectrum not a number", archiveOf(t,
			entry{name: spectrum, body: title + "much\n"},
			entry{name: wavelengths, body: good}), strconv.ErrSyntax,
			"usgs: " + spectrum},
		{"wavelength not a number", archiveOf(t,
			entry{name: spectrum, body: good},
			entry{name: wavelengths, body: title + "wide\n"}),
			strconv.ErrSyntax, "usgs: " + wavelengths},
		{"line too long", archiveOf(t,
			entry{name: spectrum, body: long},
			entry{name: wavelengths, body: good}), bufio.ErrTooLong,
			"usgs: " + spectrum},
	} {
		err := readFrom(each.made, each.made.Size(), named)
		if !errors.Is(err, each.wrap) ||
			!strings.Contains(err.Error(), each.saying) {
			t.Errorf("%s: %v", each.why, err)
		}
	}
}

// libraryOf is the one product New holds.
func libraryOf(t *testing.T) *celestialdata.Product {
	t.Helper()
	archive, err := usgs.New()
	if err != nil {
		t.Fatal(err)
	}
	found, err := archive.Find(replay.Asked(t))
	if err != nil || len(found) != lone {
		t.Fatalf("%d products, %v", len(found), err)
	}
	return found[first]
}

// The library is one product: the archive at ScienceBase, with the length
// the numbers file gives. A fetch that brings another length is refused,
// so the length is tested by what Fetch says of it.
func TestLibraryIsTheArchiveAtScienceBase(t *testing.T) {
	library := libraryOf(t)
	if library.Archive() != adapter || library.ID() != doi ||
		library.Name() != zipName ||
		!strings.HasPrefix(library.URL(), "https://www.sciencebase.gov/") ||
		!strings.HasSuffix(library.URL(), "?name="+zipName) ||
		!strings.Contains(library.About(), "public domain") ||
		!strings.Contains(library.About(), doi) {
		t.Errorf("%s %s %s %s %q", library.Archive(), library.ID(),
			library.Name(), library.URL(), library.About())
	}
	const body = "short"
	get := mocks.NewMockWeb(gomock.NewController(t))
	reply := io.NopCloser(strings.NewReader(body))
	promised := int64(len(body))
	get.EXPECT().Get(library.URL()).Return(reply, promised, nil)
	_, err := celestialdata.Fetch(get, library, t.TempDir())
	want := strconv.Itoa(len(body)) + " bytes came, the archive said " +
		strconv.Itoa(archiveBytes)
	if err == nil || !strings.Contains(err.Error(), want) {
		t.Errorf("fetch of a short body: %v", err)
	}
}

// New is a fixed archive of the library, asked only by name, and it
// finds the same for every circle.
func TestNewHoldsTheLibraryForEveryCircle(t *testing.T) {
	archive, err := usgs.New()
	if err != nil {
		t.Fatal(err)
	}
	if archive.Name() != adapter || !archive.ByNameOnly() {
		t.Errorf("%q, by name only %v", archive.Name(), archive.ByNameOnly())
	}
	var _ celestialdata.ByName = archive
	library := libraryOf(t)
	pole, err := celestialdata.NewPosition(east, north)
	if err != nil {
		t.Fatal(err)
	}
	far, err := celestialdata.NewNear(pole, wide)
	if err != nil {
		t.Fatal(err)
	}
	for _, circle := range []*celestialdata.Near{far, replay.Asked(t)} {
		found, err := archive.Find(circle)
		if err != nil || len(found) != lone || *found[first] != *library {
			t.Errorf("%v, %v", found, err)
		}
	}
}

// Every number the code asks the numbers file for is in it: Value panics
// on one it lacks, so New and Read running through is the check.
func TestEveryNumberTheCodeAsksForIsInTheFile(t *testing.T) {
	if _, err := usgs.New(); err != nil {
		t.Errorf("New: %v", err)
	}
	read(t, seawater)
}
