package main

import (
	"bytes"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/janearc/celestial-data"
	"github.com/janearc/celestial-data/internal/replay"
	"github.com/janearc/celestial-data/mocks"
	"github.com/janearc/celestial-data/tap"
	"go.uber.org/mock/gomock"
)

// The codes a run ends with, as pzm gives them.
const (
	ranOK  = 0 // defined by pzm: the verb ran
	failed = 1 // defined by pzm: the verb ran and failed

	// files is how many files find names round Sgr A* from the recorded
	// reply, each on a line after the circle. It's defined: two modules'
	// files for each of five observations.
	files = 10
	// nameOnly is the words on a line that says only a name. It's
	// defined.
	nameOnly = 1
)

// The place every test asks about, as words on the command line: the
// circle round Sgr A* the recorded replies asked about.
var sgrA = []string{"--ra", "266.4168371", "--dec", "-29.0078106",
	"--radius", "0.1"}

// fits is a FITS file's first card, all a test cutout holds.
const fits = "SIMPLE  =                    T"

// cutout is the address of the cutout celestial-data asks for round Sgr A* with
// its picked settings.
const cutout = "https://alasky.cds.unistra.fr/hips-image-services/" +
	"hips2fits?coordsys=icrs&dec=-29.0078106&format=fits&fov=0.2&" +
	"height=1024&hips=CDS%2FP%2FDSS2%2Fred&projection=TAN&" +
	"ra=266.4168371&width=1024"

// cutoutName is the cutout's file name.
const cutoutName = "CDS_P_DSS2_red_266.4168371_-29.0078106_r0.1.fits"

// picked is a numbers file read, failing the test when it will not read.
func picked(t *testing.T, raw []byte) *celestialdata.Measured {
	t.Helper()
	kept := celestialdata.NewMeasured("test.yaml", raw)
	return kept
}

// ran runs a command with words, and gives what it said and its code.
func ran(tool command, words ...string) (string, string, int) {
	var out, errs bytes.Buffer
	code := tool.run(words, strings.NewReader(""), &out, &errs)
	return out.String(), errs.String(), code
}

// through is celestial-data asking through a mock web.
func through(t *testing.T, get *mocks.MockWeb) command {
	return command{get: get, picked: picked(t, numbersRaw)}
}

// quiet is a mock web that must not be asked anything.
func quiet(t *testing.T) *mocks.MockWeb {
	return mocks.NewMockWeb(gomock.NewController(t))
}

// numaster is the address nustar asks HEASARC, for the asked circle.
func numaster(t *testing.T) string {
	adql := "SELECT TOP 5 obsid, name, exposure_a, ra, dec FROM numaster " +
		"WHERE " + tap.Within(replay.Asked(t), "ra", "dec") +
		" AND exposure_a > 0 ORDER BY exposure_a DESC"
	return tap.URL("https://heasarc.gsfc.nasa.gov/xamin/vo/tap/sync", adql,
		"votable")
}

// find names each file an archive holds by its archive, id and name. Then
// it says what the file is and where.
func TestFindNamesEachFile(t *testing.T) {
	get := replay.Web(t, map[string]string{numaster(t): "numaster-sgra.xml"})
	words := append([]string{"find", "--archive", "nustar"}, sgrA...)
	out, errs, code := ran(through(t, get), words...)
	want := "nustar 30001002001 nu30001002001A01_cl.evt.gz\n" +
		"    SgrAstar, module A, 156789 s\n" +
		"    https://heasarc.gsfc.nasa.gov/FTP/nustar/data/obs/00/3/" +
		"30001002001/event_cl/nu30001002001A01_cl.evt.gz\n"
	if code != ranOK || !strings.Contains(out, want) ||
		strings.Count(out, "\nnustar ") != files {
		t.Errorf("%d %q %q", code, out, errs)
	}
}

// The cutout's host, survey and size are the call's when it gives them.
func TestFindTakesTheCallsSettings(t *testing.T) {
	words := append([]string{"find", "--archive", "hips", "--hips-host",
		"mirror.example", "--hips", "CDS/P/2MASS/K", "--pixels", "64"},
		sgrA...)
	tool := through(t, quiet(t))
	out, errs, code := ran(tool, words...)
	for _, want := range []string{"https://mirror.example/",
		"hips=CDS%2FP%2F2MASS%2FK", "width=64", "64 pixels a side"} {
		if code != ranOK || !strings.Contains(out, want) {
			t.Errorf("no %s in %d %q %q", want, code, out, errs)
		}
	}
}

// Archives that cannot be asked are said at the end. The others are
// still asked.
func TestFindGoesOnPastAnArchiveThatFails(t *testing.T) {
	get := quiet(t)
	get.EXPECT().Get(numaster(t)).Return(nil, int64(none),
		errors.New("down"))
	words := append([]string{"find", "--archive", "nustar,hips"}, sgrA...)
	out, errs, code := ran(through(t, get), words...)
	if code != failed || !strings.Contains(out, cutoutName) ||
		!strings.Contains(errs, "nustar: ") {
		t.Errorf("%d %q %q", code, out, errs)
	}
}

// fetchInto fetches the cutout round Sgr A* into a folder through get.
func fetchInto(t *testing.T, get *mocks.MockWeb, folder string,
	more ...string) (string, string, int) {
	words := append([]string{"fetch", "--archive", "hips", "--into",
		folder}, sgrA...)
	return ran(through(t, get), append(words, more...)...)
}

// answering is a mock web that answers the cutout once.
func answering(t *testing.T) *mocks.MockWeb {
	get := quiet(t)
	body := io.NopCloser(strings.NewReader(fits))
	length := int64(len(fits))
	get.EXPECT().Get(cutout).Return(body, length, nil)
	return get
}

// fetch brings each file into a folder for its archive. Fetching again
// keeps what is there and says so.
func TestFetchBringsFilesHomeOnce(t *testing.T) {
	folder := t.TempDir()
	home := filepath.Join(folder, "hips", cutoutName)
	out, errs, code := fetchInto(t, answering(t), folder)
	got, err := os.ReadFile(home)
	if code != ranOK || out != "fetched "+home+"\n" || err != nil ||
		string(got) != fits {
		t.Fatalf("%d %q %q %v", code, out, errs, err)
	}
	out, errs, code = fetchInto(t, quiet(t), folder)
	if code != ranOK || out != "kept "+home+", already there\n" {
		t.Errorf("%d %q %q", code, out, errs)
	}
}

// --match keeps to the files whose name holds it.
func TestFetchKeepsToWhatMatches(t *testing.T) {
	out, errs, code := fetchInto(t, quiet(t), t.TempDir(), "--match",
		"2MASS")
	if code != ranOK || out != "" {
		t.Errorf("%d %q %q", code, out, errs)
	}
}

// Anti-tests: a file that does not come is an error, and so is one some
// other fetch is bringing.
func TestFetchSaysWhatDidNotCome(t *testing.T) {
	get := quiet(t)
	get.EXPECT().Get(cutout).Return(nil, int64(none), errors.New("down"))
	_, errs, code := fetchInto(t, get, t.TempDir())
	if code != failed || !strings.Contains(errs, cutoutName+": down") {
		t.Errorf("%d %q", code, errs)
	}
	folder := t.TempDir()
	part := filepath.Join(folder, "hips", cutoutName+".part")
	if err := os.MkdirAll(filepath.Dir(part), written); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(part, nil, written); err != nil {
		t.Fatal(err)
	}
	_, errs, code = fetchInto(t, quiet(t), folder)
	if code != failed || !strings.Contains(errs, "already there") {
		t.Errorf("%d %q", code, errs)
	}
}

// written is the mode test folders and files are made with. It's
// defined: anyone may read them and their owner may write.
const written = 0o755

// Anti-tests: words that do not say a search are refused before anything
// is asked.
func TestWordsThatSayNoSearchAreRefused(t *testing.T) {
	for says, words := range map[string][]string{
		"needs --into":           append([]string{"fetch"}, sgrA...),
		"numbers of degrees":     {"find", "--dec", "0", "--radius", "1"},
		"declination 91":         {"find", "--ra", "0", "--dec", "91", "--radius", "1"},
		"radius 0 is not":        {"find", "--ra", "0", "--dec", "0", "--radius", "0"},
		"--most is a whole":      append([]string{"find", "--most", "many"}, sgrA...),
		"--pixels is a whole":    append([]string{"find", "--pixels", "x"}, sgrA...),
		"search, not 0":          append([]string{"find", "--most", "0"}, sgrA...),
		"no archive is called":   append([]string{"find", "--archive", "nope"}, sgrA...),
		"--ra, --dec and --radi": {"fetch", "--into", "sky", "--dec", "0", "--radius", "1"},
	} {
		tool := through(t, quiet(t))
		_, errs, code := ran(tool, words...)
		if code != failed || !strings.Contains(errs, says) {
			t.Errorf("%s: %d %q", says, code, errs)
		}
	}
}

// numbersWith is a numbers file holding the picked numbers named, each
// with the value given.
func numbersWith(values map[string]string) []byte {
	text := "numbers:\n"
	for name, value := range values {
		text += "  - name: " + name + "\n    value: " + value +
			"\n    says: a test\n    kind: picked\n    by: a test\n"
	}
	return []byte(text)
}

// Anti-tests: numbers celestial-data cannot pick from are an error in every verb
// that needs them.
func TestMissingNumbersAreAnError(t *testing.T) {
	for says, values := range map[string]map[string]string{
		"no number called most": {},
		"no number called wait": {"most": "5", "pixels": "64",
			"dasch-whole": "1"},
		"search, not 0": {"most": "0", "pixels": "64", "wait": "1",
			"dasch-whole": "1"},
	} {
		tool := command{picked: picked(t, numbersWith(values))}
		for _, words := range [][]string{{"archives"},
			append([]string{"find", "--archive", "hips"}, sgrA...)} {
			_, errs, code := ran(tool, words...)
			if code != failed || !strings.Contains(errs, says) {
				t.Errorf("%s, %s: %d %q", says, words[none], code, errs)
			}
		}
	}
}

// Commands given no web ask the network. They wait for an answer
// however long it takes to come whole.
func TestTheRealWebAsks(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(answerFits))
	defer server.Close()
	get, err := command{picked: picked(t, numbersRaw)}.web()
	if err != nil {
		t.Fatal(err)
	}
	body, _, err := get.Get(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	defer body.Close()
	if got, err := io.ReadAll(body); err != nil || string(got) != fits {
		t.Errorf("%q, %v", got, err)
	}
}

// answerFits answers with a FITS file's first card.
func answerFits(writer http.ResponseWriter, _ *http.Request) {
	io.WriteString(writer, fits)
}

// Every archive celestial-data asks has a line saying what it holds.
func TestEveryArchiveSaysWhatItHolds(t *testing.T) {
	tool := through(t, quiet(t))
	out, errs, code := ran(tool, "archives")
	lines := strings.Split(strings.TrimSpace(out), "\n")
	if code != ranOK || len(lines) != len(holds)+len(holdsByName) {
		t.Fatalf("%d %q %q", code, out, errs)
	}
	for _, line := range lines {
		if len(strings.Fields(line)) <= nameOnly {
			t.Errorf("%q says nothing", line)
		}
	}
}

// The Blue Marble archive is on the shelf and names its two files when it
// is named. It is Fixed, so the mock web, which allows no call, is not
// asked.
func TestFindNamesTheTwoMarbleFiles(t *testing.T) {
	words := append([]string{"find", "--archive", "bluemarble"}, sgrA...)
	tool := through(t, quiet(t))
	out, errs, code := ran(tool, words...)
	for _, want := range []string{
		"bluemarble 2004-07 world.200407.3x5400x2700.jpg\n",
		"bluemarble 2016-night BlackMarble_2016_01deg.jpg\n"} {
		if code != ranOK || !strings.Contains(out, want) {
			t.Errorf("no %q in %d %q %q", want, code, out, errs)
		}
	}
}

// Calls that name no archive ask every one.
func TestNamingNoArchiveChoosesEvery(t *testing.T) {
	with, err := through(t, quiet(t)).defaults()
	if err != nil {
		t.Fatal(err)
	}
	every, err := known(quiet(t), with)
	if err != nil {
		t.Fatal(err)
	}
	picked, err := chosen(every, nil)
	if err != nil || len(picked) != len(holds) {
		t.Errorf("%d archives, %v", len(picked), err)
	}
}

// fixedArchive is an archive asked only by name, holding one file.
func fixedArchive(t *testing.T) *celestialdata.Fixed {
	t.Helper()
	one := celestialdata.NewFile("one", "one.fits", "https://x/one.fits",
		celestialdata.Unsized, "")
	fixed, err := celestialdata.NewFixed("fixed", one)
	if err != nil {
		t.Fatal(err)
	}
	return fixed
}

// An archive asked only by name is left out when no archive is named,
// and asked when it is.
func TestAnArchiveAskedByNameIsAskedOnlyByName(t *testing.T) {
	fixed := fixedArchive(t)
	with, err := through(t, quiet(t)).defaults()
	if err != nil {
		t.Fatal(err)
	}
	every, err := known(quiet(t), with)
	if err != nil {
		t.Fatal(err)
	}
	every = append(every, fixed)
	picked, err := chosen(every, nil)
	if err != nil || len(picked) != len(holds) ||
		slices.Contains(picked, celestialdata.Archive(fixed)) {
		t.Errorf("none named: %d archives, %v", len(picked), err)
	}
	picked, err = chosen(every, []string{"fixed"})
	if err != nil || len(picked) != oneArchive || picked[none] != fixed {
		t.Errorf("named: %v, %v", picked, err)
	}
}

// oneArchive is a call that names one archive. It's defined.
const oneArchive = 1
