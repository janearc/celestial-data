package celestialdata_test

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/janearc/celestial-data"
	"github.com/janearc/celestial-data/mocks"
	"go.uber.org/mock/gomock"
)

// The adapter the base's tests are, and the file it names.
const (
	called = "nustar"      // defined: an adapter's name, one word
	file   = "a.fits"      // defined: a plain file name
	about  = "two modules" // defined: what a product is, in a few words
	lone   = 1             // defined: one product
)

// adapter is an adapter called called, asking through a web that must not
// be asked anything.
func adapter(t *testing.T) *celestialdata.Adapter {
	t.Helper()
	made, err := celestialdata.NewAdapter(called, silent(t))
	if err != nil {
		t.Fatal(err)
	}
	return made
}

// An adapter's products are its archive's, and say what they are when
// told; told nothing, they say nothing.
func TestAnAdaptersProductsCarryItsName(t *testing.T) {
	made := adapter(t)
	if made.Name() != called || made.Web() == nil {
		t.Fatalf("%q, %v", made.Name(), made.Web())
	}
	told, err := made.Product("obs", file, where, unsized, about)
	if err != nil || told.Archive() != called || told.ID() != "obs" ||
		told.Name() != file || told.URL() != where ||
		told.About() != about {
		t.Fatalf("%+v, %v", told, err)
	}
	quiet, err := made.Product("obs", file, where, unsized, "")
	if err != nil || quiet.About() != "" {
		t.Fatalf("%+v, %v", quiet, err)
	}
	if _, err := made.Product("obs", "../out", where, unsized,
		about); !errors.Is(err, celestialdata.Refused()) {
		t.Errorf("a file that is not plain: %v", err)
	}
}

// Anti-tests: a name that is not one word, and a missing web, are
// refused, and the refusal names what was asked.
func TestAnAdapterRefusesWhatItCannotBe(t *testing.T) {
	for _, name := range []string{"", "two words", "one,two", "tab\tbed"} {
		_, err := celestialdata.NewAdapter(name, silent(t))
		if !errors.Is(err, celestialdata.Refused()) ||
			!strings.Contains(err.Error(), "one word") {
			t.Errorf("%q: %v", name, err)
		}
	}
	_, err := celestialdata.NewAdapter(called, nil)
	if !errors.Is(err, celestialdata.Refused()) ||
		!strings.Contains(err.Error(), called+" needs a web") {
		t.Errorf("no web: %v", err)
	}
}

// A named adapter asks through nothing of its own.
func TestANamedAdapterHasNoWeb(t *testing.T) {
	named, err := celestialdata.Named("hips")
	if err != nil || named.Name() != "hips" || named.Web() != nil {
		t.Fatalf("%q, %v, %v", named.Name(), named.Web(), err)
	}
}

// Refusals and errors begin with the adapter's name, and keep what they
// wrap; no error wraps to none.
func TestRefusalsAndErrorsBeginWithTheName(t *testing.T) {
	made := adapter(t)
	refused := made.Refuse("needs %d of them", lone)
	if !errors.Is(refused, celestialdata.Refused()) ||
		!strings.Contains(refused.Error(), called+" needs 1 of them") {
		t.Errorf("refused: %v", refused)
	}
	down := errors.New("down")
	wrapped := made.Wrap(down)
	if !errors.Is(wrapped, down) || wrapped.Error() != called+": down" {
		t.Errorf("wrapped: %v", wrapped)
	}
	if made.Wrap(nil) != nil {
		t.Error("none wrapped to something")
	}
	failed := made.Fail("%s: %w", file, down)
	if !errors.Is(failed, down) || errors.Is(failed, celestialdata.Refused()) ||
		failed.Error() != called+": "+file+": down" {
		t.Errorf("failed: %v", failed)
	}
}

// EndsIn keeps the links that end in its suffix, and no others.
func TestEndsInKeepsItsSuffix(t *testing.T) {
	keep := celestialdata.EndsIn(".fits")
	for link, want := range map[string]bool{"a.fits": true,
		"a.fits.gz": false, "a.txt": false, "fits": false, ".fits": true} {
		if keep(link) != want {
			t.Errorf("%q: %v", link, !want)
		}
	}
}

// A folder's files are those its page lists that keep holds, each under
// the folder. Links that lead out of it are passed over.
func TestFilesAreTheFoldersFilesKept(t *testing.T) {
	page := []byte(`<a href="a.fits">a</a> <a href="b.txt">b</a> ` +
		`<a href="../c.fits">c</a> <a href="sub/d.fits">d</a>`)
	get := served(t, page, lengthOf(page))
	made, err := celestialdata.NewAdapter(called, get)
	if err != nil {
		t.Fatal(err)
	}
	kept, err := made.Files(where, celestialdata.EndsIn(".fits"), "obs", about)
	if err != nil || len(kept) != lone {
		t.Fatalf("%d files, %v", len(kept), err)
	}
	if got := kept[firstOf]; got.Name() != file || got.URL() != where+file ||
		got.ID() != "obs" || got.About() != about ||
		got.Archive() != called {
		t.Errorf("%+v", got)
	}
}

// firstOf is the first of what is found. It's defined.
const firstOf = 0

// The archive does not say the size of a file its folder lists, so the
// file is fetched whole, whatever its length.
func TestAFoldersFileIsFetchedWholeWhateverItsLength(t *testing.T) {
	page := []byte(`<a href="a.fits">a</a>`)
	content := []byte("more than one byte")
	get := mocks.NewMockWeb(gomock.NewController(t))
	get.EXPECT().Get(where).Return(bodyOf(page), lengthOf(page),
		nil).Times(once)
	get.EXPECT().Get(where+file).Return(bodyOf(content), int64(unsaid),
		nil).Times(once)
	made, err := celestialdata.NewAdapter(called, get)
	if err != nil {
		t.Fatal(err)
	}
	kept, err := made.Files(where, celestialdata.EndsIn(".fits"), "obs", about)
	if err != nil || len(kept) != lone {
		t.Fatalf("%d files, %v", len(kept), err)
	}
	if _, err := celestialdata.Fetch(get, kept[firstOf],
		t.TempDir()); err != nil {
		t.Errorf("a file of no stated size: %v", err)
	}
}

// Anti-test: a folder that will not list is an error that names the
// adapter, and finds nothing.
func TestAFolderThatWillNotListIsAnError(t *testing.T) {
	get := mocks.NewMockWeb(gomock.NewController(t))
	get.EXPECT().Get(where).Return(nil, int64(unsized),
		errors.New("down")).Times(once)
	made, err := celestialdata.NewAdapter(called, get)
	if err != nil {
		t.Fatal(err)
	}
	kept, err := made.Files(where, celestialdata.EndsIn(".fits"), "obs", about)
	if err == nil || kept != nil ||
		!strings.HasPrefix(err.Error(), called+": ") {
		t.Errorf("%v, %v", kept, err)
	}
}

// The field the Field tests use: two degrees square round right ascension
// 10 on the equator, and circles a tenth of a degree across, on it and
// far from it.
const (
	fieldAt  = 10  // defined: the field's centre, in degrees
	fieldOff = 50  // defined: a right ascension far from the field
	fieldTo  = 1   // defined: the field's half width, in degrees
	across   = 0.1 // defined: the circles' radius, in degrees
)

// circleAt is the small circle round a right ascension on the equator.
func circleAt(t *testing.T, ra float64) *celestialdata.Near {
	t.Helper()
	centre, err := celestialdata.NewPosition(ra, equator)
	if err != nil {
		t.Fatal(err)
	}
	circle, err := celestialdata.NewNear(centre, across)
	if err != nil {
		t.Fatal(err)
	}
	return circle
}

// A field reaches a circle on its footprint and not one far from it.
func TestAFieldReachesWhatIsOnItsFootprint(t *testing.T) {
	patch, err := celestialdata.NewPatch(celestialdata.InICRS, fieldAt,
		equator, fieldTo, fieldTo)
	if err != nil {
		t.Fatal(err)
	}
	field, err := celestialdata.NewField(called, silent(t), patch)
	if err != nil || field.Name() != called || field.Footprint() != patch {
		t.Fatalf("%v, %v", field.Name(), err)
	}
	if !field.Reaches(circleAt(t, fieldAt)) {
		t.Error("a circle on the field is not reached")
	}
	if field.Reaches(circleAt(t, fieldOff)) {
		t.Error("a circle far from the field is reached")
	}
}

// An adapter that is not a field has no footprint, and reaches every
// circle.
func TestAnAdapterThatIsNoFieldReachesEverywhere(t *testing.T) {
	made := adapter(t)
	if made.Footprint() != nil {
		t.Errorf("a footprint: %v", made.Footprint())
	}
	for _, ra := range []float64{fieldAt, fieldOff} {
		if !made.Reaches(circleAt(t, ra)) {
			t.Errorf("at %v: not reached", ra)
		}
	}
}

// Anti-tests: a field refuses no footprint, and what an adapter refuses.
func TestAFieldRefusesNoFootprint(t *testing.T) {
	_, err := celestialdata.NewField(called, silent(t), nil)
	if !errors.Is(err, celestialdata.Refused()) ||
		!strings.Contains(err.Error(), called+" needs its footprint") {
		t.Errorf("no footprint: %v", err)
	}
	patch, err := celestialdata.NewPatch(celestialdata.InICRS, fieldAt,
		equator, fieldTo, fieldTo)
	if err != nil {
		t.Fatal(err)
	}
	_, err = celestialdata.NewField(called, nil, patch)
	if !errors.Is(err, celestialdata.Refused()) {
		t.Errorf("no web: %v", err)
	}
}

// A fixed adapter finds its files wherever it is asked, gives a copy so
// what it holds cannot be changed, and is asked only by name.
func TestAFixedAdapterFindsItsFilesAnywhere(t *testing.T) {
	july := celestialdata.NewFile(julyID, file, where, sized, about)
	fixed, err := celestialdata.NewFixed("bluemarble", july)
	if err != nil {
		t.Fatal(err)
	}
	var asked celestialdata.ByName = fixed
	if !asked.ByNameOnly() || asked.Name() != "bluemarble" {
		t.Errorf("%q asked by name only: %v", asked.Name(),
			asked.ByNameOnly())
	}
	var held *celestialdata.Product
	for _, ra := range []float64{fieldAt, fieldOff} {
		found, err := fixed.Find(circleAt(t, ra))
		if err != nil || len(found) != lone {
			t.Fatalf("at %v: %v, %v", ra, found, err)
		}
		got := found[firstOf]
		if got.Archive() != "bluemarble" || got.ID() != julyID ||
			got.Name() != file || got.URL() != where ||
			got.About() != about || (held != nil && got != held) {
			t.Errorf("at %v: %+v", ra, got)
		}
		held = got
		found[firstOf] = nil
	}
	if again, _ := fixed.Find(nil); again[firstOf] != held {
		t.Error("what was found changed what the adapter holds")
	}
}

// The file a fixed adapter's tests describe ahead.
const (
	julyID = "2004-07" // defined: what the file belongs to
	sized  = 1024      // defined: a length in bytes, any past none
)

// Anti-tests: a fixed adapter refuses no files, a file Product refuses,
// and a name that is not one word, each by the adapter's name.
func TestAFixedAdapterRefusesWhatItCannotHold(t *testing.T) {
	plain := celestialdata.NewFile("one", file, where, celestialdata.Unsized,
		"")
	outside := celestialdata.NewFile("two", "../out", where,
		celestialdata.Unsized, "")
	for why, attempt := range map[string]struct {
		name  string
		files []celestialdata.File
	}{
		"bluemarble needs its files": {"bluemarble", nil},
		"bluemarble: refused": {"bluemarble",
			[]celestialdata.File{plain, outside}},
		"one word": {"blue marble", []celestialdata.File{plain}},
	} {
		_, err := celestialdata.NewFixed(attempt.name, attempt.files...)
		if !errors.Is(err, celestialdata.Refused()) ||
			!strings.Contains(err.Error(), why) {
			t.Errorf("%s: %v", why, err)
		}
	}
}

// A measured number is its value; a name the numbers file does not hold
// is a mistake in the adapter's own code, and asking panics, naming it.
func TestAMeasuredValueIsItsNumberOrAPanic(t *testing.T) {
	kept := celestialdata.NewMeasured("numbers.yaml", []byte("numbers:\n"+
		"  - name: size\n    value: 1024\n    unit: bytes\n"+
		"    says: a file's length\n    kind: picked\n    by: a test\n"))
	if got := kept.Value("size"); got != sized {
		t.Errorf("size: %v", got)
	}
	if why := panicOf(kept, "absent"); !strings.Contains(why, "absent") {
		t.Errorf("no panic for an absent name: %q", why)
	}
}

// panicOf is what asking kept for a name panics with, or nothing.
func panicOf(kept *celestialdata.Measured, name string) (why string) {
	defer recovered(&why)
	kept.Value(name)
	return ""
}

// recovered puts what a panic gave, said, into why.
func recovered(why *string) {
	if given := recover(); given != nil {
		*why = fmt.Sprint(given)
	}
}
