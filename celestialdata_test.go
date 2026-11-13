package celestialdata_test

import (
	"bytes"
	"compress/gzip"
	"errors"
	"io"
	"io/fs"
	"math"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"

	"github.com/janearc/celestial-data"
	"github.com/janearc/celestial-data/mocks"
	"go.uber.org/mock/gomock"
	"pgregory.net/rapid"
)

// where is the address every product here is held at.
const where = "https://archive.example/file"

// The numbers these tests use.
const (
	once        = 1     // defined: a call that happens exactly once
	unsaid      = -1    // defined: web's length when the server says none
	unsized     = 0     // defined: a size the archive does not say
	more        = 1     // defined: one byte past what came
	half        = 2     // defined: what cuts a stream in two
	shut        = 0o000 // defined: a folder no one may enter
	readOnly    = 0o555 // defined: a folder no one may write in
	writeOnly   = 0o200 // defined: a file no one may read
	open        = 0o755 // defined: a folder anyone may enter
	written     = 0o644 // defined: a file anyone may read
	root        = 0     // defined: the superuser's user id
	nothingRead = 0     // defined: the count a failed read gives
	raStart     = 0     // defined: where right ascension begins
	raEnd       = 360   // defined: right ascension goes once round
	southPole   = -90   // defined: the declination of the south pole
	northPole   = 90    // defined: the declination of the north pole
	equator     = 0     // defined: the declination of the equator
	pointlike   = 0     // defined: a circle that is no circle
	widest      = 180   // defined: a circle on the sphere at its widest
	positive    = 1     // defined: math.Inf's sign for +Inf
	negative    = -1    // defined: math.Inf's sign for -Inf
)

// productOf is a product named name and held at where, of a size.
func productOf(t *testing.T, name string, size int64) *celestialdata.Product {
	t.Helper()
	product, err := tester(t).Product("1", name, where, size, "")
	if err != nil {
		t.Fatal(err)
	}
	return product
}

// tester is an adapter called test, with no web, that makes the tests'
// products.
func tester(check stopper) *celestialdata.Adapter {
	check.Helper()
	made, err := celestialdata.Named("test")
	if err != nil {
		check.Fatal(err)
	}
	return made
}

// stopper is what tester needs of a test, rapid's or testing's.
type stopper interface {
	Helper()
	Fatal(args ...any)
}

// bodyOf is what a Get hands back for some bytes.
func bodyOf(content []byte) io.ReadCloser {
	reader := bytes.NewReader(content)
	return io.NopCloser(reader)
}

// lengthOf is the length of some bytes, as web gives lengths.
func lengthOf(content []byte) int64 { return int64(len(content)) }

// served is a mock web that answers one Get of where with content, saying
// length.
func served(reporter gomock.TestReporter, content []byte,
	length int64) *mocks.MockWeb {
	get := mocks.NewMockWeb(gomock.NewController(reporter))
	body := bodyOf(content)
	get.EXPECT().Get(where).Return(body, length, nil).Times(once)
	return get
}

// silent is a mock web that must not be asked anything.
func silent(t *testing.T) *mocks.MockWeb {
	t.Helper()
	return mocks.NewMockWeb(gomock.NewController(t))
}

// zipped is text gzipped.
func zipped(t *testing.T, text string) []byte {
	t.Helper()
	var packed bytes.Buffer
	writer := gzip.NewWriter(&packed)
	if _, err := writer.Write([]byte(text)); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	return packed.Bytes()
}

// nothingBut says whether a folder holds exactly the names given.
func nothingBut(t *testing.T, folder string, names ...string) bool {
	t.Helper()
	found, err := os.ReadDir(folder)
	if err != nil {
		t.Fatal(err)
	}
	if len(found) != len(names) {
		return false
	}
	for i, entry := range found {
		if entry.Name() != names[i] {
			return false
		}
	}
	return true
}

func TestFetchBringsAFileHome(t *testing.T) {
	into := filepath.Join(t.TempDir(), "new", "folder")
	content := []byte("photons")
	product := productOf(t, "events.txt", lengthOf(content))
	get := served(t, content, lengthOf(content))
	home, err := celestialdata.Fetch(get, product, into)
	if err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(home)
	if err != nil || !bytes.Equal(got, content) {
		t.Fatalf("%s holds %q, %v", home, got, err)
	}
	if home != filepath.Join(into, "events.txt") ||
		!nothingBut(t, into, "events.txt") {
		t.Errorf("home is %s, and the folder holds more", home)
	}
}

func TestFetchTakesAServerThatDoesNotSayItsLength(t *testing.T) {
	get := served(t, []byte("abc"), unsaid)
	product := productOf(t, "a.fits", unsized)
	if _, err := celestialdata.Fetch(get, product, t.TempDir()); err != nil {
		t.Fatal(err)
	}
}

func TestFetchReadsAGzipFileToItsEnd(t *testing.T) {
	content := zipped(t, "events")
	get := served(t, content, lengthOf(content))
	product := productOf(t, "evt2.fits.gz", unsized)
	if _, err := celestialdata.Fetch(get, product, t.TempDir()); err != nil {
		t.Fatal(err)
	}
}

// failure is a fetch that must fail: what the server sends and says, and
// the product's name and size.
type failure struct {
	content  []byte
	promised int64
	name     string
	size     int64
}

// leavesNothing fetches and checks that the fetch failed and left nothing.
func (each failure) leavesNothing(t *testing.T) {
	into := t.TempDir()
	get := served(t, each.content, each.promised)
	product := productOf(t, each.name, each.size)
	if _, err := celestialdata.Fetch(get, product, into); err == nil {
		t.Fatal("fetched")
	}
	if !nothingBut(t, into) {
		t.Error("the folder holds something")
	}
}

// Anti-tests: when a fetch goes wrong, nothing is left at home. Neither
// the file nor its .part stays.
func TestFetchLeavesNothingWhenItFails(t *testing.T) {
	abc := []byte("abc")
	stream := zipped(t, "a stream long enough to be cut short")
	cut := stream[:len(stream)/half]
	for name, each := range map[string]failure{
		"the server sent less than it said": {abc,
			lengthOf(abc) + more, "a.txt", unsized},
		"the archive said another size": {abc, unsaid, "a.txt",
			lengthOf(abc) + more},
		"a gzip file cut short":        {cut, unsaid, "a.gz", unsized},
		"a gzip file that is not gzip": {abc, unsaid, "a.gz", unsized},
	} {
		t.Run(name, each.leavesNothing)
	}
}

// dropping is a body that gives some bytes and then an error.
type dropping struct{ given bool }

// Read gives three bytes once, then fails.
func (body *dropping) Read(into []byte) (int, error) {
	if body.given {
		return nothingRead, errors.New("the line dropped")
	}
	body.given = true
	return copy(into, "abc"), nil
}

// Close does nothing.
func (*dropping) Close() error { return nil }

func TestFetchLeavesNothingWhenTheLineDrops(t *testing.T) {
	into := t.TempDir()
	get := mocks.NewMockWeb(gomock.NewController(t))
	get.EXPECT().Get(where).Return(&dropping{}, int64(unsaid),
		nil).Times(once)
	if _, err := celestialdata.Fetch(get, productOf(t, "a.txt", unsized),
		into); err == nil {
		t.Fatal("fetched")
	}
	if !nothingBut(t, into) {
		t.Error("the folder holds something")
	}
}

func TestFetchLeavesNothingWhenTheArchiveRefuses(t *testing.T) {
	into := t.TempDir()
	get := mocks.NewMockWeb(gomock.NewController(t))
	refusal := errors.New("GET: 404 Not Found")
	get.EXPECT().Get(where).Return(nil, int64(unsized), refusal).Times(once)
	if _, err := celestialdata.Fetch(get, productOf(t, "a.txt", unsized),
		into); !errors.Is(err, refusal) {
		t.Fatalf("%v", err)
	}
	if !nothingBut(t, into) {
		t.Error("the folder holds something")
	}
}

// crooked are names that are not one plain file name.
var crooked = []string{"", ".", "..", "a/b", "../escape", "/etc/passwd",
	"events.part"}

// Anti-tests: no product is made with a name that is not one plain file
// name. None is made without an address. None is made with a size below
// nothing.
func TestAProductRefusesWhatCannotBeFetched(t *testing.T) {
	made := tester(t)
	for _, name := range crooked {
		_, err := made.Product("1", name, where, unsized, "")
		if !errors.Is(err, celestialdata.Refused()) {
			t.Errorf("%q: %v", name, err)
		}
	}
	_, err := made.Product("1", "a.txt", "", unsized, "")
	if !errors.Is(err, celestialdata.Refused()) ||
		!strings.Contains(err.Error(), "needs an address") {
		t.Errorf("no address: %v", err)
	}
	if _, err := made.Product("1", "a.txt", where, unsaid,
		""); !errors.Is(err, celestialdata.Refused()) {
		t.Errorf("a negative size: %v", err)
	}
}

func TestAProductSaysWhatItIs(t *testing.T) {
	size := lengthOf([]byte("photons"))
	chandra, err := celestialdata.Named("chandra")
	if err != nil {
		t.Fatal(err)
	}
	product, err := chandra.Product("3392", "evt2.fits.gz", where, size,
		"events")
	if err != nil {
		t.Fatal(err)
	}
	if product.Archive() != "chandra" || product.ID() != "3392" ||
		product.Name() != "evt2.fits.gz" || product.URL() != where ||
		product.About() != "events" {
		t.Errorf("%+v", product)
	}
}

// Anti-tests: Fetch refuses a product NewProduct did not make. It refuses
// no product at all too. Nothing is asked of the web first.
func TestFetchRefusesAProductThatWasNeverMade(t *testing.T) {
	for _, never := range []*celestialdata.Product{nil, {}} {
		into := filepath.Join(t.TempDir(), "sky")
		_, err := celestialdata.Fetch(silent(t), never, into)
		_, made := os.Stat(into)
		if !errors.Is(err, celestialdata.Refused()) ||
			!errors.Is(made, os.ErrNotExist) {
			t.Errorf("%v: %v, and the folder: %v", never, err, made)
		}
	}
}

// Anti-tests: a circle is not made without a centre.
func TestNewNearRefusesNoCentre(t *testing.T) {
	if _, err := celestialdata.NewNear(nil, widest); !errors.Is(err,
		celestialdata.Refused()) {
		t.Errorf("%v", err)
	}
}

// Anti-tests: Fetch never replaces a file already at home. It leaves a
// .part there alone too, since that may be another fetch's.
func TestFetchNeverReplacesWhatIsThere(t *testing.T) {
	for _, there := range []string{"a.txt", "a.txt.part"} {
		into := t.TempDir()
		at := filepath.Join(into, there)
		if err := os.WriteFile(at, []byte("mine"), written); err != nil {
			t.Fatal(err)
		}
		_, err := celestialdata.Fetch(silent(t), productOf(t, "a.txt", unsized),
			into)
		if !errors.Is(err, celestialdata.Refused()) {
			t.Errorf("%s there: %v", there, err)
		}
		kept, _ := os.ReadFile(at)
		if string(kept) != "mine" || !nothingBut(t, into, there) {
			t.Errorf("%s there: it was changed", there)
		}
	}
}

func TestFetchReportsAFolderItCannotMake(t *testing.T) {
	blocked := filepath.Join(t.TempDir(), "a file")
	if err := os.WriteFile(blocked, nil, written); err != nil {
		t.Fatal(err)
	}
	inside := filepath.Join(blocked, "in")
	product := productOf(t, "a.txt", unsized)
	_, err := celestialdata.Fetch(silent(t), product, inside)
	var failed *fs.PathError
	if !errors.As(err, &failed) || failed.Op != "mkdir" {
		t.Errorf("%v", err)
	}
}

// folder is a folder a test changes the mode of and opens again after.
type folder string

// reopen gives the folder back the mode of an ordinary one.
func (path folder) reopen() { os.Chmod(string(path), open) }

// madeWith is a new folder of a mode, opened again when the test ends.
func madeWith(t *testing.T, mode os.FileMode) string {
	t.Helper()
	if os.Getuid() == root {
		t.Skip("the superuser goes anywhere")
	}
	path := filepath.Join(t.TempDir(), "folder")
	if err := os.Mkdir(path, mode); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(folder(path).reopen)
	return path
}

func TestFetchReportsAFolderItCannotLookIn(t *testing.T) {
	into := madeWith(t, shut)
	_, err := celestialdata.Fetch(silent(t), productOf(t, "a.txt", unsized), into)
	if err == nil || errors.Is(err, celestialdata.Refused()) {
		t.Errorf("%v", err)
	}
}

func TestFetchReportsAFolderItCannotWriteIn(t *testing.T) {
	into := madeWith(t, readOnly)
	_, err := celestialdata.Fetch(silent(t), productOf(t, "a.txt", unsized), into)
	if err == nil || errors.Is(err, celestialdata.Refused()) {
		t.Errorf("%v", err)
	}
}

// meanwhile is a Get that does something to the folder while it answers,
// as another process might.
type meanwhile struct {
	t       *testing.T
	act     func(*testing.T)
	content []byte
}

// get acts, then answers with the content.
func (during meanwhile) get(string) (io.ReadCloser, int64, error) {
	during.act(during.t)
	return bodyOf(during.content), lengthOf(during.content), nil
}

// answering is a mock web whose one Get of where is during's.
func answering(t *testing.T, during meanwhile) *mocks.MockWeb {
	get := mocks.NewMockWeb(gomock.NewController(t))
	get.EXPECT().Get(where).DoAndReturn(during.get).Times(once)
	return get
}

// takenAt is an act that makes a folder where a file was to go.
type takenAt string

// take makes the folder, with something inside so it cannot be replaced.
func (path takenAt) take(t *testing.T) {
	inside := filepath.Join(string(path), "inside")
	if err := os.MkdirAll(inside, open); err != nil {
		t.Fatal(err)
	}
}

// When something else takes the name while the file comes in, Fetch does
// not replace it. The rename fails and the .part goes.
func TestFetchLeavesNothingOfItsOwnWhenTheNameIsTakenMeanwhile(t *testing.T) {
	into := t.TempDir()
	during := meanwhile{t, takenAt(filepath.Join(into, "a.txt")).take,
		[]byte("abc")}
	_, err := celestialdata.Fetch(answering(t, during),
		productOf(t, "a.txt", unsized), into)
	if err == nil {
		t.Fatal("fetched over a folder")
	}
	if !nothingBut(t, into, "a.txt") {
		t.Error("its .part is still there")
	}
}

// lockedAt is an act that makes a file unreadable.
type lockedAt string

// lock leaves the file writable and unreadable.
func (path lockedAt) lock(t *testing.T) {
	if err := os.Chmod(string(path), writeOnly); err != nil {
		t.Fatal(err)
	}
}

// Fetch reports a gzip file it cannot read back once written.
func TestFetchReportsAGzipFileItCannotReadBack(t *testing.T) {
	if os.Getuid() == root {
		t.Skip("the superuser reads any file")
	}
	into := t.TempDir()
	part := filepath.Join(into, "a.gz.part")
	during := meanwhile{t, lockedAt(part).lock, zipped(t, "events")}
	_, err := celestialdata.Fetch(answering(t, during),
		productOf(t, "a.gz", unsized), into)
	if !errors.Is(err, fs.ErrPermission) {
		t.Fatalf("fetched a file it could not check: %v", err)
	}
	if !nothingBut(t, into) {
		t.Error("the folder holds something")
	}
}

// trailer is the end of a gzip member. It's defined by RFC 1952: a CRC32
// and the length, four bytes each.
const trailer = 8

// Anti-test: a gzip file cut before its end is not kept, even when the
// server and the archive both said its cut length.
func TestFetchLeavesAGzipFileCutShort(t *testing.T) {
	whole := zipped(t, "events")
	cut := whole[:len(whole)-trailer]
	get := served(t, cut, lengthOf(cut))
	into := t.TempDir()
	product := productOf(t, "evt2.fits.gz", lengthOf(cut))
	if _, err := celestialdata.Fetch(get, product, into); err == nil ||
		!nothingBut(t, into) {
		t.Errorf("kept a cut gzip file: %v", err)
	}
}

// umask lets open and written through as they are. It's defined: the
// group and others lose write, which neither mode gives them.
const umask = 0o022

// The folders Fetch makes may be read and listed by anyone, and its
// files read by anyone.
func TestFetchMakesWhatAnyoneMayRead(t *testing.T) {
	was := syscall.Umask(umask)
	defer syscall.Umask(was)
	into := filepath.Join(t.TempDir(), "sky")
	content := []byte("photons")
	get := served(t, content, lengthOf(content))
	home, err := celestialdata.Fetch(get, productOf(t, "a.txt", unsized), into)
	if err != nil {
		t.Fatal(err)
	}
	folder, folderErr := os.Stat(into)
	file, fileErr := os.Stat(home)
	if folderErr != nil || fileErr != nil ||
		folder.Mode().Perm() != open || file.Mode().Perm() != written {
		t.Errorf("%v %v, %v %v", folder, folderErr, file, fileErr)
	}
}

// The poles and the widest circle are on the sky; a hair past them is
// not.
func TestTheSkysEdgesAreItsOwn(t *testing.T) {
	south := math.Nextafter(southPole, math.Inf(negative))
	wider := math.Nextafter(widest, math.Inf(positive))
	if _, err := celestialdata.NewPosition(raStart, southPole); err != nil {
		t.Errorf("the south pole: %v", err)
	}
	if _, err := celestialdata.NewPosition(raEnd, equator); err == nil {
		t.Error("a full turn of right ascension, which is none again")
	}
	if _, err := celestialdata.NewPosition(raStart, south); err == nil {
		t.Error("past the south pole")
	}
	centre := at(t, raStart, equator)
	if _, err := celestialdata.NewNear(centre, widest); err != nil {
		t.Errorf("the widest circle: %v", err)
	}
	if _, err := celestialdata.NewNear(centre, pointlike); err == nil {
		t.Error("a circle of no radius")
	}
	if _, err := celestialdata.NewNear(centre, wider); err == nil {
		t.Error("wider than the sky")
	}
}

// keepsExactlyWhenLengthsAgree is the property of
// TestFetchKeepsAFileExactlyWhenTheLengthsAgree, for one draw.
func keepsExactlyWhenLengthsAgree(t *rapid.T) {
	content := rapid.SliceOf(rapid.Byte()).Draw(t, "content")
	length := lengthOf(content)
	promised := rapid.SampledFrom([]int64{unsaid, length,
		length + more}).Draw(t, "promised")
	size := rapid.SampledFrom([]int64{unsized, length,
		length + more}).Draw(t, "size")
	agree := (promised == unsaid || promised == length) &&
		(size == unsized || size == length)
	into, err := os.MkdirTemp("", "fetch")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(into)
	product, _ := tester(t).Product("1", "f.bin", where, size, "")
	home, err := celestialdata.Fetch(served(t, content, promised), product, into)
	if agree != (err == nil) {
		t.Fatalf("lengths agree %v, fetch says %v", agree, err)
	}
	got, _ := os.ReadFile(home)
	if agree && !bytes.Equal(got, content) {
		t.Fatalf("holds %d bytes of %d", len(got), len(content))
	}
}

// Property: after a fetch, the file is at home exactly when every length
// that was said is the length that came. Then it holds what came.
func TestFetchKeepsAFileExactlyWhenTheLengthsAgree(t *testing.T) {
	rapid.Check(t, keepsExactlyWhenLengthsAgree)
}

// onTheSky is the property of TestNewPositionTakesExactlyTheSky, for one
// draw.
func onTheSky(t *rapid.T) {
	ra := rapid.Float64().Draw(t, "ra")
	dec := rapid.Float64().Draw(t, "dec")
	place, err := celestialdata.NewPosition(ra, dec)
	sky := ra >= raStart && ra < raEnd && dec >= southPole &&
		dec <= northPole
	if sky != (err == nil) {
		t.Fatalf("ra %v, dec %v: on the sky %v, %v", ra, dec, sky, err)
	}
	if sky && (place.RA() != ra || place.Dec() != dec) {
		t.Fatalf("made %v, %v", place.RA(), place.Dec())
	}
}

// Property: a position is made exactly when it is on the sky, and it keeps
// what it was made of.
func TestNewPositionTakesExactlyTheSky(t *testing.T) {
	rapid.Check(t, onTheSky)
}

// aCircle is the property of TestNewNearTakesExactlyACircle, for one draw.
func aCircle(t *rapid.T) {
	radius := rapid.Float64().Draw(t, "radius")
	centre, _ := celestialdata.NewPosition(raStart, equator)
	circle, err := celestialdata.NewNear(centre, radius)
	fits := radius > pointlike && radius <= widest
	if fits != (err == nil) {
		t.Fatalf("radius %v: fits %v, %v", radius, fits, err)
	}
	if fits && (circle.Radius() != radius || circle.Centre() != *centre) {
		t.Fatalf("made %v round %v", circle.Radius(), circle.Centre())
	}
}

// Property: a circle is made exactly when its radius is one a circle on
// the sky can have.
func TestNewNearTakesExactlyACircle(t *testing.T) {
	rapid.Check(t, aCircle)
}

// Anti-tests: numbers that are not numbers are refused everywhere.
func TestNumbersThatAreNotNumbersAreRefused(t *testing.T) {
	centre, _ := celestialdata.NewPosition(raStart, equator)
	for _, bad := range []float64{math.NaN(), math.Inf(positive),
		math.Inf(negative)} {
		if _, err := celestialdata.NewPosition(bad, equator); err == nil {
			t.Errorf("ra %v", bad)
		}
		if _, err := celestialdata.NewPosition(raStart, bad); err == nil {
			t.Errorf("dec %v", bad)
		}
		if _, err := celestialdata.NewNear(centre, bad); err == nil {
			t.Errorf("radius %v", bad)
		}
	}
}

// Adapters' refusals say why and are found as refusals.
func TestRefuseSaysWhy(t *testing.T) {
	err := celestialdata.Refuse("%s needs %d", "nustar", once)
	if !errors.Is(err, celestialdata.Refused()) ||
		err.Error() != "refused: nustar needs 1" {
		t.Errorf("%v", err)
	}
}

// onlyOne is the place of a find's only product. It's defined.
const onlyOne = 0

// Finds of one product hand on Product's refusal, or the product.
func TestFoundIsOneProductOrWhyNot(t *testing.T) {
	made := tester(t)
	found, err := celestialdata.Found(made.Product("1", "photons.fits",
		where, unsized, "light"))
	if err != nil || len(found) != once || found[onlyOne].About() !=
		"light" {
		t.Errorf("%v, %v", found, err)
	}
	none, err := celestialdata.Found(made.Product("1", "a/b", where,
		unsized, "light"))
	if none != nil || !errors.Is(err, celestialdata.Refused()) {
		t.Errorf("%v, %v", none, err)
	}
}

// saying is a check that holds a file to saying exactly what it wants.
type saying struct{ want string }

// Check is nil for a file that says what the check wants.
func (check saying) Check(path string) error {
	got, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	if string(got) != check.want {
		return errors.New("not what was wanted")
	}
	return nil
}

// checkedFor is a check that holds a file to saying exactly want.
func checkedFor(want string) celestialdata.Checker { return saying{want} }

// A product's check is given the file once all its bytes are in, and a
// file that passes is kept under the product's name.
func TestFetchKeepsAFileItsCheckPasses(t *testing.T) {
	content := []byte("whole")
	product := productOf(t, "w.fits", unsized).Checked(
		checkedFor("whole"))
	into := t.TempDir()
	home, err := celestialdata.Fetch(served(t, content, unsaid), product,
		into)
	if err != nil || !nothingBut(t, into, "w.fits") {
		t.Errorf("%s, %v", home, err)
	}
}

// Anti-test: a file its product's check refuses is not kept, nor is its
// .part, and the check's reason is the fetch's.
func TestFetchLeavesNothingWhenItsCheckRefuses(t *testing.T) {
	product := productOf(t, "w.fits", unsized).Checked(
		checkedFor("whole"))
	into := t.TempDir()
	get := served(t, []byte("whol"), unsaid)
	_, err := celestialdata.Fetch(get, product, into)
	if err == nil || err.Error() != "not what was wanted" ||
		!nothingBut(t, into) {
		t.Errorf("%v", err)
	}
}

// A product that was never made, because Product refused it, stays none
// when it is checked, so its refusal can be handed on with it.
func TestCheckedHandsOnAProductThatWasNeverMade(t *testing.T) {
	made, err := tester(t).Product("1", "a/b", where, unsized, "")
	if checked := made.Checked(checkedFor("")); checked != nil ||
		!errors.Is(err, celestialdata.Refused()) {
		t.Errorf("%v, %v", checked, err)
	}
}

// Anti-test: a whole gzip file is read to its end and then still held to
// the product's check.
func TestFetchChecksAGzipFileAfterReadingIt(t *testing.T) {
	content := zipped(t, "events")
	product := productOf(t, "e.fits.gz", unsized).Checked(
		checkedFor("not the gzip file"))
	into := t.TempDir()
	_, err := celestialdata.Fetch(served(t, content, unsaid), product,
		into)
	if err == nil || err.Error() != "not what was wanted" ||
		!nothingBut(t, into) {
		t.Errorf("%v", err)
	}
}
