package main

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/janearc/celestial-data"
	"github.com/janearc/celestial-data/irsa"
	"github.com/janearc/celestial-data/mocks"
	"github.com/janearc/celestial-data/web"
	"go.uber.org/mock/gomock"
)

// The addresses the fake archives are asked at. They are where the real
// archives keep these pages, so the real adapters read them.
const (
	meerkatPage = "https://doi.org/10.48479/fyst-hj47"
	meerkatData = "https://archive.test/10.48479/fyst-hj47/data/"
	ppmapFolder = "http://www.astro.cardiff.ac.uk/research/ViaLactea/" +
		"PPMAP_Results/"
	ppmapTile     = ppmapFolder + "l000_results/"
	fakeFile      = "https://fake.test/fake.jpg"
	chromaSources = "https://api.github.com/repos/janearc/chroma/" +
		"contents/colourways/sources?ref=main"
	aquaFile     = "https://raw.test/aqua.css"
	twilightFile = "https://raw.test/twilight.css"
)

// The wanted names, as the viewer reads them.
const (
	mosaic1   = "GLM_00000+0000_mosaic_I1.fits"
	mosaic4   = "GLM_00000+0000_mosaic_I4.fits"
	radioName = "MeerKAT_Galactic_Centre_1284MHz-StokesI.fits"
	dustName  = "l000_cdens.fits"
)

// The counts and places the tests work with.
const (
	readable  = 0o644 // defined: anyone may read a test file, its owner write
	origin    = 0     // defined: a place on the sky for a set that asks none
	degree    = 1     // defined: the radius of the circle round it
	oneByte   = 1     // defined: a byte more than a file holds
	keptLines = 14    // defined: twelve files, and the two colourways
)

// allKept is what the grissom set says when every one of its files is
// there.
const allKept = "fetch-set grissom: 0 fetched, 14 kept, in "

// chromaList is the listing of the chroma repository's folder, as the
// fake archives hold it, with what is not a colourway in it. Of GitHub's
// fields it keeps the three the adapter reads.
const chromaList = `[
{"name": "SOURCES.md", "type": "file", "download_url": "https://raw.test/s"},
{"name": "aqua.css", "type": "file", "download_url": "` + aquaFile + `"},
{"name": "samples", "type": "dir", "download_url": null},
{"name": "twilight.css", "type": "file", "download_url": "` + twilightFile +
	`"}
]`

// chromaMore is the listing after a colourway is added to the repository.
const chromaMore = `[
{"name": "aqua.css", "type": "file", "download_url": "` + aquaFile + `"},
{"name": "twilight.css", "type": "file", "download_url": "` + twilightFile +
	`"},
{"name": "zinc.css", "type": "file", "download_url": "https://raw.test/z"}
]`

// listingOf is a page that links each name.
func listingOf(names ...string) string {
	var body strings.Builder
	for _, name := range names {
		fmt.Fprintf(&body, "<a href=\"%s\">%s</a>\n", name, name)
	}
	return body.String()
}

// fakeArchives are the pages and files the fake archives hold, by
// address.
type fakeArchives map[string]string

// get is the web: a page or file held, or a failure.
func (held fakeArchives) get(address string) (io.ReadCloser, int64,
	error) {
	body, ok := held[address]
	if !ok {
		return nil, none, fmt.Errorf("nothing at %s", address)
	}
	return io.NopCloser(strings.NewReader(body)), int64(len(body)), nil
}

// colourwayPages are the pages the fake chroma holds and no others.
func colourwayPages() fakeArchives {
	return fakeArchives{
		chromaSources: chromaList,
		aquaFile:      "aqua body",
		twilightFile:  "twilight body",
	}
}

// everything is every page and file a good fetch of the small set asks
// for.
func everything() fakeArchives {
	held := colourwayPages()
	held[irsa.GlimpseII] = listingOf("GLM_00000+0000_3.1x4.5.jpg",
		mosaic1, "GLM_00000+0000_mosaic_I2.fits", mosaic4)
	held[irsa.GlimpseII+mosaic1] = "one"
	held[irsa.GlimpseII+mosaic4] = "four"
	held[meerkatPage] = listingOf(meerkatData+radioName,
		meerkatData+"MeerKAT_Galactic_Centre_1284MHz-Alpha.fits")
	held[meerkatData+radioName] = "radio"
	held[ppmapFolder] = listingOf("l000_results/")
	held[ppmapTile] = listingOf(dustName, "l000_temp.fits")
	held[ppmapTile+dustName] = "dust"
	held[fakeFile] = "earth"
	return held
}

// serving is a web that holds these, and fails on anything else.
func serving(t *testing.T, have fakeArchives) *mocks.MockWeb {
	t.Helper()
	get := quiet(t)
	get.EXPECT().Get(gomock.Any()).DoAndReturn(have.get).AnyTimes()
	return get
}

// An env is a person's environment.
type env map[string]string

// lookup is the environment's value of a name.
func (given env) lookup(name string) string { return given[name] }

// inside is celestial-data asking through a web, in an environment, with
// the sets it knows and the small one.
func inside(t *testing.T, get *mocks.MockWeb, given env) command {
	tool := through(t, get)
	tool.lookup = given.lookup
	tool.sets = standardSets()
	tool.sets["tiny"] = tiny
	return tool
}

// tiny is the grissom set with a small fake archive for the earth, in
// place of the real ones, whose lengths are pinned and are megabytes, and
// without the sky.
func tiny(tool command, get web.Web) (*set, error) {
	made, err := grissomSet(tool, get)
	if err != nil {
		return nil, err
	}
	file := celestialdata.NewFile("fake", "fake.jpg", fakeFile,
		celestialdata.Unsized, "a fake")
	fixed, err := celestialdata.NewFixed("fake", file)
	if err != nil {
		return nil, err
	}
	var parts []part
	for _, each := range made.parts {
		if each.only != nil || each.into == "colourways" {
			parts = append(parts, each)
		}
	}
	made.parts = append(parts, part{"earth", fixed, nil})
	return made, nil
}

// fetchTiny fetches the small set into a folder through a web.
func fetchTiny(t *testing.T, get *mocks.MockWeb, folder string,
	more ...string) (string, string, int) {
	t.Helper()
	words := append([]string{"fetch-set", "tiny", "--into", folder}, more...)
	tool := inside(t, get, env{})
	return ran(tool, words...)
}

// fetchEverything fetches the small set into a folder, from archives that
// hold everything.
func fetchEverything(t *testing.T, folder string, more ...string) (string,
	string, int) {
	t.Helper()
	have := everything()
	get := serving(t, have)
	return fetchTiny(t, get, folder, more...)
}

// A set fetches every file into its place, one line a file, and says the
// count. It brings only the files it wants of what an archive lists, and
// every .css file the colourways listing holds.
func TestEverythingInASetIsFetched(t *testing.T) {
	folder := t.TempDir()
	out, errs, code := fetchEverything(t, folder)
	want := "fetched  sgra/glimpse/" + mosaic1 + "\n" +
		"fetched  sgra/glimpse/" + mosaic4 + "\n" +
		"fetched  sgra/meerkat/" + radioName + "\n" +
		"fetched  sgra/ppmap/" + dustName + "\n" +
		"fetched  colourways/aqua.css\n" +
		"fetched  colourways/twilight.css\n" +
		"fetched  earth/fake.jpg\n" +
		"fetch-set tiny: 7 fetched, 0 kept, in " + folder + "\n"
	if code != ranOK || out != want {
		t.Errorf("%d it said\n%s\nnot\n%s\nand %q", code, out, want, errs)
	}
	for path, body := range map[string]string{
		"sgra/glimpse/" + mosaic1:   "one",
		"sgra/glimpse/" + mosaic4:   "four",
		"sgra/meerkat/" + radioName: "radio",
		"sgra/ppmap/" + dustName:    "dust",
		"colourways/aqua.css":       "aqua body",
		"colourways/twilight.css":   "twilight body",
		"earth/fake.jpg":            "earth",
	} {
		kept, err := os.ReadFile(filepath.Join(folder, path))
		if err != nil || string(kept) != body {
			t.Errorf("%s holds %q, %v, not %q", path, kept, err, body)
		}
	}
	for _, left := range []string{
		"sgra/glimpse/GLM_00000+0000_mosaic_I2.fits",
		"sgra/meerkat/MeerKAT_Galactic_Centre_1284MHz-Alpha.fits",
		"sgra/ppmap/l000_temp.fits", "colourways/SOURCES.md",
		"colourways/samples"} {
		if _, err := os.Stat(filepath.Join(folder, left)); err == nil {
			t.Errorf("%s was fetched, and was not wanted", left)
		}
	}
}

// A second run keeps every file and says so. It asks no archive for a
// file, and asks chroma only for its listing.
func TestASecondRunKeepsEverything(t *testing.T) {
	folder := t.TempDir()
	if _, errs, code := fetchEverything(t, folder); code != ranOK {
		t.Fatalf("%d %q", code, errs)
	}
	listed := colourwayPages()
	out, errs, code := fetchTiny(t, serving(t, listed), folder)
	want := "kept     sgra/glimpse/" + mosaic1 + " (already there)\n" +
		"kept     sgra/glimpse/" + mosaic4 + " (already there)\n" +
		"kept     sgra/meerkat/" + radioName + " (already there)\n" +
		"kept     sgra/ppmap/" + dustName + " (already there)\n" +
		"kept     colourways/aqua.css (already there)\n" +
		"kept     colourways/twilight.css (already there)\n" +
		"kept     earth/fake.jpg (already there)\n" +
		"fetch-set tiny: 0 fetched, 7 kept, in " + folder + "\n"
	if code != ranOK || out != want {
		t.Errorf("%d it said\n%s\nnot\n%s\nand %q", code, out, want, errs)
	}
}

// A colourway added to the listing later is fetched and the rest kept.
func TestALaterColourwayComesWithNoChange(t *testing.T) {
	folder := t.TempDir()
	if _, errs, code := fetchEverything(t, folder); code != ranOK {
		t.Fatalf("%d %q", code, errs)
	}
	listed := everything()
	listed[chromaSources] = chromaMore
	listed["https://raw.test/z"] = "zinc body"
	out, errs, code := fetchTiny(t, serving(t, listed), folder)
	if code != ranOK || !strings.Contains(out, "fetched  colourways/"+
		"zinc.css\n") || !strings.Contains(out, "7 kept") ||
		!strings.Contains(out, "1 fetched") {
		t.Errorf("%d %q %q", code, out, errs)
	}
}

// A file that is there is kept and the rest of its archive is fetched.
func TestAFileThereIsKeptAndTheRestAreFetched(t *testing.T) {
	folder := t.TempDir()
	dir := filepath.Join(folder, "sgra", "glimpse")
	if err := os.MkdirAll(dir, written); err != nil {
		t.Fatal(err)
	}
	mine := filepath.Join(dir, mosaic1)
	if err := os.WriteFile(mine, []byte("mine"), readable); err != nil {
		t.Fatal(err)
	}
	out, errs, code := fetchEverything(t, folder)
	got, err := os.ReadFile(mine)
	if code != ranOK || err != nil || string(got) != "mine" ||
		!strings.Contains(out, "kept     sgra/glimpse/"+mosaic1+
			" (already there)\n") ||
		!strings.Contains(out, "fetched  sgra/glimpse/"+mosaic4+"\n") ||
		!strings.Contains(out, "6 fetched, 1 kept") {
		t.Errorf("%d %q %q, %v, %q", code, out, errs, err, got)
	}
}

// A dry call says what it would bring, asks the archives only for the
// listing of what it is not told, and makes no folder.
func TestADryCallBringsNothing(t *testing.T) {
	folder := filepath.Join(t.TempDir(), "later")
	get := serving(t, colourwayPages())
	out, errs, code := fetchTiny(t, get, folder, "--dry-run")
	want := "would fetch sgra/glimpse/" + mosaic1 + "\n" +
		"would fetch sgra/glimpse/" + mosaic4 + "\n" +
		"would fetch sgra/meerkat/" + radioName + "\n" +
		"would fetch sgra/ppmap/" + dustName + "\n" +
		"would fetch colourways/aqua.css\n" +
		"would fetch colourways/twilight.css\n" +
		"would fetch earth/fake.jpg\n" +
		"fetch-set tiny: 7 to fetch, 0 kept, in " + folder + "\n"
	if code != ranOK || out != want {
		t.Errorf("%d it said\n%s\nnot\n%s\nand %q", code, out, want, errs)
	}
	if _, err := os.Stat(folder); err == nil {
		t.Errorf("a dry call made %s", folder)
	}
}

// A dry call keeps what is there and says what is not.
func TestADryCallKeepsWhatIsThere(t *testing.T) {
	folder := t.TempDir()
	dir := filepath.Join(folder, "sgra", "glimpse")
	if err := os.MkdirAll(dir, written); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, mosaic4), nil,
		readable); err != nil {
		t.Fatal(err)
	}
	get := serving(t, colourwayPages())
	out, errs, code := fetchTiny(t, get, folder, "--dry-run")
	for _, want := range []string{
		"would fetch sgra/glimpse/" + mosaic1 + "\n",
		"kept     sgra/glimpse/" + mosaic4 + " (already there)\n",
		"fetch-set tiny: 6 to fetch, 1 kept, in "} {
		if code != ranOK || !strings.Contains(out, want) {
			t.Errorf("no %q in %d %q %q", want, code, out, errs)
		}
	}
}

// Anti-tests: a dry call says a file it cannot look for, and goes on.
func TestADryCallSaysWhatItCannotLookInto(t *testing.T) {
	folder := t.TempDir()
	blocker := filepath.Join(folder, "sgra")
	if err := os.WriteFile(blocker, nil, readable); err != nil {
		t.Fatal(err)
	}
	get := serving(t, colourwayPages())
	out, errs, code := fetchTiny(t, get, folder, "--dry-run")
	if code != failed || !strings.Contains(errs, "sgra/glimpse/"+mosaic1+
		": ") || !strings.Contains(errs, "fetch-set tiny: 4 failed") ||
		!strings.Contains(out, "would fetch earth/fake.jpg\n") {
		t.Errorf("%d %q %q", code, out, errs)
	}
}

// An archive that cannot be asked fails its own files, says which, and
// the rest are fetched.
func TestAnArchiveThatErrors(t *testing.T) {
	have := everything()
	delete(have, irsa.GlimpseII)
	get := serving(t, have)
	out, errs, code := fetchTiny(t, get, t.TempDir())
	for _, want := range []string{"sgra/glimpse/: glimpse: ",
		"nothing at " + irsa.GlimpseII, "fetch-set tiny: 1 failed. " +
			"What came is kept. Fix what is said above and run it again"} {
		if code != failed || !strings.Contains(errs, want) {
			t.Errorf("no %q in %d %q", want, code, errs)
		}
	}
	if !strings.Contains(out, "fetched  sgra/ppmap/"+dustName) ||
		!strings.Contains(out, "5 fetched, 0 kept") {
		t.Errorf("the rest were not fetched: %s", out)
	}
}

// A colourways listing GitHub will not give fails its folder alone.
func TestTheColourwaysThatDoNotCome(t *testing.T) {
	have := everything()
	delete(have, chromaSources)
	get := serving(t, have)
	out, errs, code := fetchTiny(t, get, t.TempDir())
	if code != failed || !strings.Contains(errs, "colourways/: chroma: "+
		"nothing at "+chromaSources) ||
		!strings.Contains(out, "5 fetched, 0 kept") {
		t.Errorf("%d %q %q", code, out, errs)
	}
}

// A wanted file the listing does not hold is an error that names it, and
// the others it lists are fetched.
func TestAWantedFileNotListed(t *testing.T) {
	have := everything()
	have[irsa.GlimpseII] = listingOf("GLM_00000+0000_3.1x4.5.jpg", mosaic1)
	get := serving(t, have)
	out, errs, code := fetchTiny(t, get, t.TempDir())
	want := "sgra/glimpse/" + mosaic4 + ": glimpse does not list it"
	if code != failed || !strings.Contains(errs, want) ||
		!strings.Contains(out, "fetched  sgra/glimpse/"+mosaic1) {
		t.Errorf("%d %q %q", code, out, errs)
	}
}

// A file the archive lists but will not give is an error that names it.
func TestAFileThatWillNotCome(t *testing.T) {
	have := everything()
	delete(have, meerkatData+radioName)
	get := serving(t, have)
	_, errs, code := fetchTiny(t, get, t.TempDir())
	want := "sgra/meerkat/" + radioName + ": nothing at " + meerkatData +
		radioName
	if code != failed || !strings.Contains(errs, want) {
		t.Errorf("%d %q", code, errs)
	}
}

// A .part left by a fetch that was cut off is said, with what to do.
func TestAPartLeftBehind(t *testing.T) {
	folder := t.TempDir()
	dir := filepath.Join(folder, "sgra", "meerkat")
	if err := os.MkdirAll(dir, written); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, radioName+".part"), nil,
		readable); err != nil {
		t.Fatal(err)
	}
	_, errs, code := fetchEverything(t, folder)
	if code != failed || !strings.Contains(errs, "sgra/meerkat/"+
		radioName+": refused: ") || !strings.Contains(errs,
		"remove the .part file there and run it again") {
		t.Errorf("%d %q", code, errs)
	}
}

// A folder that cannot be looked into is an error that names the file.
func TestAFolderThatCannotBeLookedInto(t *testing.T) {
	folder := t.TempDir()
	if err := os.WriteFile(filepath.Join(folder, "sgra"), nil,
		readable); err != nil {
		t.Fatal(err)
	}
	_, errs, code := fetchEverything(t, folder)
	if code != failed || !strings.Contains(errs,
		"sgra/meerkat/"+radioName+": lstat ") ||
		!strings.Contains(errs, "fetch-set tiny: 4 failed") {
		t.Errorf("%d %q", code, errs)
	}
}

// shortening is a web that says a file is a byte longer than it is.
type shortening struct{ fakeArchives }

// get is the web, with one file's length overstated.
func (held shortening) get(address string) (io.ReadCloser, int64, error) {
	body, length, err := held.fakeArchives.get(address)
	if address == meerkatData+radioName {
		length += oneByte
	}
	return body, length, err
}

// A file that does not come whole is not kept, and is said as it is.
func TestAFileThatComesShort(t *testing.T) {
	folder := t.TempDir()
	have := shortening{everything()}
	get := quiet(t)
	get.EXPECT().Get(gomock.Any()).DoAndReturn(have.get).AnyTimes()
	_, errs, code := fetchTiny(t, get, folder)
	home := filepath.Join(folder, "sgra", "meerkat", radioName)
	if _, err := os.Stat(home); err == nil || code != failed ||
		!strings.Contains(errs, "sgra/meerkat/"+radioName+": ") {
		t.Errorf("%d %q, %v", code, errs, err)
	}
}

// failing is an archive that cannot be asked.
type failing struct{}

// Name is the archive's name.
func (failing) Name() string { return "failing" }

// Find fails.
func (failing) Find(*celestialdata.Near) ([]*celestialdata.Product,
	error) {
	return nil, errors.New("asked and refused")
}

// oddSet is a set whose one archive cannot be asked.
func oddSet(tool command, get web.Web) (*set, error) {
	centre, err := celestialdata.NewPosition(origin, origin)
	if err != nil {
		return nil, err
	}
	near, err := celestialdata.NewNear(centre, degree)
	if err != nil {
		return nil, err
	}
	return &set{near: near, parts: []part{{"odd", failing{}, nil}}}, nil
}

// Anti-tests: an archive that cannot be asked is said, whether the call
// is dry or not.
func TestAPartThatCannotBeListed(t *testing.T) {
	tool := inside(t, quiet(t), env{})
	tool.sets["odd"] = oddSet
	for _, more := range [][]string{nil, {"--dry-run"}} {
		words := append([]string{"fetch-set", "odd", "--into",
			t.TempDir()}, more...)
		out, errs, code := ran(tool, words...)
		if code != failed || !strings.Contains(errs,
			"odd/: asked and refused") ||
			!strings.Contains(errs, "fetch-set odd: 1 failed") ||
			!strings.Contains(out, "fetch-set odd: 0 ") {
			t.Errorf("%v: %d %q %q", more, code, out, errs)
		}
	}
}

// namesOf is the names of every file a part holds.
func namesOf(t *testing.T, each part, near *celestialdata.Near) []string {
	t.Helper()
	if each.only != nil {
		return each.only
	}
	found, err := each.from.Find(near)
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, product := range found {
		names = append(names, product.Name())
	}
	return names
}

// keptAll is a folder that has every file of the grissom set, empty:
// nothing is left to fetch but the colourways listing, which is asked of
// the fake chroma.
func keptAll(t *testing.T) string {
	t.Helper()
	get := serving(t, colourwayPages())
	tool := through(t, get)
	made, err := grissomSet(tool, get)
	if err != nil {
		t.Fatal(err)
	}
	folder := t.TempDir()
	for _, each := range made.parts {
		dir := filepath.Join(folder, each.into)
		if err := os.MkdirAll(dir, written); err != nil {
			t.Fatal(err)
		}
		for _, name := range namesOf(t, each, made.near) {
			path := filepath.Join(dir, name)
			if err := os.WriteFile(path, nil, readable); err != nil {
				t.Fatal(err)
			}
		}
	}
	return folder
}

// --into names the folder, and the variable is not looked at when it is
// given. The real archives are not asked: every file is there.
func TestIntoBeatsTheVariable(t *testing.T) {
	folder := keptAll(t)
	given := env{"GRISSOM_DATA": t.TempDir()}
	get := serving(t, colourwayPages())
	tool := inside(t, get, given)
	out, errs, code := ran(tool, "fetch-set", "grissom", "--into", folder)
	if code != ranOK || !strings.HasSuffix(out, allKept+folder+"\n") {
		t.Errorf("%d %q %q", code, out, errs)
	}
}

// Without --into the folder is the variable named for the set.
func TestTheVariableNamesTheFolder(t *testing.T) {
	folder := keptAll(t)
	given := env{"GRISSOM_DATA": folder, "TINY_DATA": "nowhere"}
	get := serving(t, colourwayPages())
	tool := inside(t, get, given)
	out, errs, code := ran(tool, "fetch-set", "grissom")
	lines := strings.Count(out, "(already there)\n")
	if code != ranOK || !strings.HasSuffix(out, allKept+folder+"\n") ||
		lines != keptLines {
		t.Errorf("%d %q %q", code, out, errs)
	}
}

// Anti-tests: words and settings that name no set, or no folder, are
// refused before anything is asked, and the refusal says what to do.
func TestASetNotNamedOrFolderNotGivenIsRefused(t *testing.T) {
	type refusal struct {
		given env
		words []string
	}
	for says, each := range map[string]refusal{
		"takes the name of one set, not 0 words": {env{},
			[]string{"fetch-set"}},
		"takes the name of one set, not 2 words": {env{},
			[]string{"fetch-set", "grissom", "tiny"}},
		"no set is called nope. The sets are: alpha, grissom, tiny": {
			env{}, []string{"fetch-set", "nope"}},
		"The sets are: alpha, grissom, tiny": {env{},
			[]string{"fetch-set"}},
		"fetch-set grissom needs --into, the folder to bring its " +
			"files into, or GRISSOM_DATA. Set it in your game config " +
			"as: set GRISSOM_DATA /some/folder": {env{},
			[]string{"fetch-set", "grissom"}},
		"or TINY_DATA. Set it in your game config as: set TINY_DATA " +
			"/some/folder": {env{"GRISSOM_DATA": "here"},
			[]string{"fetch-set", "tiny"}},
	} {
		tool := inside(t, quiet(t), each.given)
		tool.sets["alpha"] = oddSet
		out, errs, code := ran(tool, each.words...)
		if code != failed || !strings.Contains(errs, says) ||
			out != "" {
			t.Errorf("%q: %d %q %q", says, code, out, errs)
		}
	}
}

// Anti-tests: numbers a set needs and cannot pick are an error that
// names them, and so is the wait the real web needs.
func TestAMissingNumberStopsASet(t *testing.T) {
	every := map[string]string{"grissom-radius": "0.1",
		"grissom-sky-width": "3600", "grissom-iris-width": "1800",
		"wait": "1"}
	for _, name := range []string{"grissom-radius", "grissom-sky-width",
		"grissom-iris-width", "wait"} {
		given := copied(every)
		delete(given, name)
		tool := command{picked: picked(t, numbersWith(given)),
			lookup: env{}.lookup, sets: standardSets()}
		if name != "wait" {
			tool.get = quiet(t)
		}
		_, errs, code := ran(tool, "fetch-set", "grissom", "--into",
			t.TempDir())
		if code != failed || !strings.Contains(errs, "no number "+
			"called "+name) {
			t.Errorf("without %s: %d %q", name, code, errs)
		}
	}
}

// copied is a copy of a map of strings.
func copied(from map[string]string) map[string]string {
	made := map[string]string{}
	for name, value := range from {
		made[name] = value
	}
	return made
}

// Anti-tests: an archive that will not be made is refused, whatever it
// is asked of, and nothing is fetched.
func TestASetRefusesWithoutAWeb(t *testing.T) {
	tool := through(t, quiet(t))
	_, err := grissomSet(tool, nil)
	if err == nil || !strings.Contains(err.Error(),
		"glimpse needs a web") || !strings.Contains(err.Error(),
		"chroma needs a web") {
		t.Errorf("with no web, the set gave %v", err)
	}
}

// Anti-tests: a skyview width below two pixels is not a set.
func TestASetRefusesAnArchiveThatWillNotBeMade(t *testing.T) {
	values := map[string]string{"grissom-radius": "0.1",
		"grissom-sky-width": "1", "grissom-iris-width": "1800",
		"wait": "1"}
	tool := command{get: quiet(t), picked: picked(t,
		numbersWith(values)), lookup: env{}.lookup,
		sets: standardSets()}
	_, errs, code := ran(tool, "fetch-set", "grissom", "--into",
		t.TempDir())
	if code != failed || !strings.Contains(errs, "skyview needs") {
		t.Errorf("%d %q", code, errs)
	}
}

// Every name a set has is one word of lower case letters, so the variable
// named for it is a name a shell takes.
func TestEverySetIsNamedByOneWord(t *testing.T) {
	tool := command{sets: standardSets()}
	for name := range tool.sets {
		for _, letter := range name {
			if letter < 'a' || letter > 'z' {
				t.Errorf("the set %q has %q in its name", name, letter)
			}
		}
		if name == "" {
			t.Error("a set has no name")
		}
	}
	if names := tool.setNames(); names != "grissom" {
		t.Errorf("the sets are %q", names)
	}
}
