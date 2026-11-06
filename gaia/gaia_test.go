package gaia_test

import (
	"errors"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/janearc/celestial-data"
	"github.com/janearc/celestial-data/gaia"
	"github.com/janearc/celestial-data/mocks"
	"github.com/janearc/celestial-data/tap"
	"go.uber.org/mock/gomock"
)

// async is the archive's endpoint for jobs, as its page names it.
const async = "https://gea.esac.esa.int/tap-server/tap/async"

// numbersFile is where the gaia package's test numbers are kept.
const numbersFile = "testdata/numbers.yaml"

// The tests' own numbers, each defined.
const (
	wholeSky   = 180   // defined: the whole sky's radius, celestialdata.NewNear's
	oneProduct = 1     // defined: one product found
	oneCookie  = 1     // defined: one cookie in a session
	twice      = 2     // defined: saved twice, the second over the first
	onlyOwner  = 0o600 // defined: a file its owner alone reads and writes
	openToAll  = 0o644 // defined: a file anyone reads
	january    = 1     // defined: the first month, and its first day
	midnight   = 0     // defined: the first hour, minute, second and nanosecond
)

// picked is a picked number from the test numbers, or the test stops.
func picked(t *testing.T, name string) float64 {
	t.Helper()
	raw, err := os.ReadFile(numbersFile)
	if err != nil {
		t.Fatal(err)
	}
	value, err := celestialdata.NewMeasured(numbersFile, raw).Get(name)
	if err != nil {
		t.Fatal(err)
	}
	return value
}

// circle is a circle on the sky round Sgr A*, or the whole sky.
func circle(t *testing.T, radius float64) *celestialdata.Near {
	t.Helper()
	centre, err := celestialdata.NewPosition(picked(t, "sgr-a-ascension"),
		picked(t, "sgr-a-declination"))
	if err != nil {
		t.Fatal(err)
	}
	near, err := celestialdata.NewNear(centre, radius)
	if err != nil {
		t.Fatal(err)
	}
	return near
}

// archive is gaia_source's ids and radial velocities, of the stars that
// have one.
func archive(t *testing.T, where string) *gaia.Archive {
	t.Helper()
	asked, err := gaia.New("source_id, radial_velocity", where)
	if err != nil {
		t.Fatal(err)
	}
	return asked
}

// the archive is named gaia and is asked only by name, never by every
// archive at once.
func TestTheArchiveIsGaiaAndAskedOnlyByName(t *testing.T) {
	asked := archive(t, "")
	if asked.Name() != "gaia" || !asked.ByNameOnly() {
		t.Fatalf("got %q, %v", asked.Name(), asked.ByNameOnly())
	}
}

// the anti-test: an archive asked for no columns is refused.
func TestNewRefusesNoColumns(t *testing.T) {
	if _, err := gaia.New(" ", ""); !errors.Is(err,
		celestialdata.Refused()) {
		t.Fatalf("got %v", err)
	}
}

// a circle is one job: the query of the rows within it, as the archive's
// TAP service takes it, named for the circle.
func TestACircleIsOneJobOfTheRowsWithinIt(t *testing.T) {
	small := circle(t, picked(t, "small-circle"))
	found, err := archive(t, "radial_velocity IS NOT NULL").Find(small)
	if err != nil || len(found) != oneProduct {
		t.Fatalf("got %v, %v", found, err)
	}
	want := tap.URL(async, "SELECT source_id, radial_velocity FROM "+
		"gaiadr3.gaia_source WHERE CONTAINS(POINT('ICRS', ra, dec), "+
		"CIRCLE('ICRS', 266.4168371, -29.0078106, 0.1)) = 1 AND "+
		"radial_velocity IS NOT NULL", "fits")
	for _, product := range found {
		if product.URL() != want || product.Archive() != "gaia" ||
			product.Name() != "gaia_source_266.4168371_-29.0078106_r0.1.fits" ||
			product.ID() != "gaiadr3.gaia_source" ||
			!strings.Contains(product.About(), "within 0.1 degrees") {
			t.Fatalf("got %s %s %s %q", product.URL(), product.Name(),
				product.ID(), product.About())
		}
	}
}

// the whole sky asks for every row, with no circle in the query, with
// or without a condition.
func TestTheWholeSkyAsksEveryRowWithNoCircle(t *testing.T) {
	for where, want := range map[string]string{
		"radial_velocity IS NOT NULL": "SELECT source_id, " +
			"radial_velocity FROM gaiadr3.gaia_source WHERE " +
			"radial_velocity IS NOT NULL",
		"": "SELECT source_id, radial_velocity FROM gaiadr3.gaia_source",
	} {
		found, err := archive(t, where).Find(circle(t, wholeSky))
		if err != nil || len(found) != oneProduct {
			t.Fatalf("%q: got %v, %v", where, found, err)
		}
		for _, product := range found {
			if product.URL() != tap.URL(async, want, "fits") {
				t.Errorf("%q: got %s", where, product.URL())
			}
		}
	}
}

// signing in sends the archive's form with the username and password.
func TestSignInSendsTheFormToTheArchive(t *testing.T) {
	get := mocks.NewMockWeb(gomock.NewController(t))
	get.EXPECT().Post("https://gea.esac.esa.int/tap-server/login",
		url.Values{"username": {"ada"}, "password": {"pw"}}).Return("",
		nil)
	if err := gaia.SignIn(get, "ada", "pw"); err != nil {
		t.Fatal(err)
	}
}

// the anti-test: a refused sign-in says who was refused and never the
// password.
func TestSignInRefusedSaysWhoAndNeverThePassword(t *testing.T) {
	get := mocks.NewMockWeb(gomock.NewController(t))
	get.EXPECT().Post(gomock.Any(), gomock.Any()).Return("",
		errors.New("POST: 401 Unauthorized"))
	err := gaia.SignIn(get, "ada", "hunter2")
	if err == nil || !strings.Contains(err.Error(), "ada") ||
		strings.Contains(err.Error(), "hunter2") {
		t.Fatalf("got %v", err)
	}
}

// session is the cookies the archive sets a signed-in user.
func session() []*http.Cookie {
	return []*http.Cookie{{Name: "JSESSIONID", Value: "s1"}}
}

// a saved session loads as it was, and saving over it keeps the last.
func TestASavedSessionLoadsAsItWas(t *testing.T) {
	path := filepath.Join(t.TempDir(), "session")
	for range twice {
		if err := gaia.Save(session(), path); err != nil {
			t.Fatal(err)
		}
	}
	got, err := gaia.Load(path)
	if err != nil || len(got) != oneCookie {
		t.Fatalf("got %v, %v", got, err)
	}
	for _, cookie := range got {
		if cookie.Value != "s1" {
			t.Fatalf("got %v", cookie)
		}
	}
}

// the anti-test: Save refuses a file anyone else can read, cookies json
// cannot write, and cookies with no session.
func TestSaveRefusesWhatIsNotAPrivateSession(t *testing.T) {
	dir := t.TempDir()
	open := filepath.Join(dir, "open")
	if err := os.WriteFile(open, nil, openToAll); err != nil {
		t.Fatal(err)
	}
	// json holds a time only from year 0 to 9999.
	pastJSON := int(picked(t, "past-json"))
	far := []*http.Cookie{{Name: "JSESSIONID", Value: "s1",
		Expires: time.Date(pastJSON, january, january, midnight, midnight,
			midnight, midnight, time.UTC)}}
	for name, each := range map[string]struct {
		cookies []*http.Cookie
		path    string
	}{"open": {session(), open}, "far": {far, filepath.Join(dir, "f")},
		"none": {nil, filepath.Join(dir, "n")}} {
		if err := gaia.Save(each.cookies, each.path); err == nil {
			t.Errorf("%s was saved", name)
		}
	}
}

// the anti-test: Load refuses a file anyone else can read, one with no
// session in it, one that is not json, a folder and a missing file.
func TestLoadRefusesWhatIsNotAPrivateSession(t *testing.T) {
	dir := t.TempDir()
	for name, file := range map[string]struct {
		text string
		mode os.FileMode
	}{"open": {`[{"Name":"JSESSIONID","Value":"s1"}]`, openToAll},
		"cookieless": {`[{"Name":"other","Value":"s1"}]`, onlyOwner},
		"garbled":    {`[{`, onlyOwner}} {
		path := filepath.Join(dir, name)
		if err := os.WriteFile(path, []byte(file.text),
			file.mode); err != nil {
			t.Fatal(err)
		}
		if _, err := gaia.Load(path); err == nil {
			t.Errorf("%s was loaded", name)
		}
	}
	folder := filepath.Join(dir, "folder")
	if err := os.Mkdir(folder, onlyOwner); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{folder, filepath.Join(dir, "none")} {
		if _, err := gaia.Load(path); err == nil {
			t.Errorf("%s was loaded", path)
		}
	}
}
