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

// circle is a circle on the sky round Sgr A*, or the whole sky.
func circle(t *testing.T, radius float64) *celestialdata.Near {
	t.Helper()
	centre, err := celestialdata.NewPosition(266.4168371, -29.0078106)
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

func TestTheArchiveIsGaiaAndAskedOnlyByName(t *testing.T) {
	asked := archive(t, "")
	if asked.Name() != "gaia" || !asked.ByNameOnly() {
		t.Fatalf("got %q, %v", asked.Name(), asked.ByNameOnly())
	}
}

func TestNewRefusesNoColumns(t *testing.T) {
	if _, err := gaia.New(" ", ""); !errors.Is(err,
		celestialdata.Refused()) {
		t.Fatalf("got %v", err)
	}
}

func TestACircleIsOneJobOfTheRowsWithinIt(t *testing.T) {
	found, err := archive(t, "radial_velocity IS NOT NULL").Find(
		circle(t, 0.1))
	if err != nil || len(found) != 1 {
		t.Fatalf("got %v, %v", found, err)
	}
	want := tap.URL(async, "SELECT source_id, radial_velocity FROM "+
		"gaiadr3.gaia_source WHERE CONTAINS(POINT('ICRS', ra, dec), "+
		"CIRCLE('ICRS', 266.4168371, -29.0078106, 0.1)) = 1 AND "+
		"radial_velocity IS NOT NULL", "fits")
	product := found[0]
	if product.URL() != want || product.Archive() != "gaia" ||
		product.Name() != "gaia_source_266.4168371_-29.0078106_r0.1.fits" ||
		product.ID() != "gaiadr3.gaia_source" ||
		!strings.Contains(product.About(), "within 0.1 degrees") {
		t.Fatalf("got %s %s %s %q", product.URL(), product.Name(),
			product.ID(), product.About())
	}
}

func TestTheWholeSkyAsksEveryRowWithNoCircle(t *testing.T) {
	for where, want := range map[string]string{
		"radial_velocity IS NOT NULL": "SELECT source_id, " +
			"radial_velocity FROM gaiadr3.gaia_source WHERE " +
			"radial_velocity IS NOT NULL",
		"": "SELECT source_id, radial_velocity FROM gaiadr3.gaia_source",
	} {
		found, err := archive(t, where).Find(circle(t, 180))
		if err != nil || found[0].URL() != tap.URL(async, want, "fits") {
			t.Errorf("%q: got %v, %v", where, found, err)
		}
	}
}

func TestSignInSendsTheFormToTheArchive(t *testing.T) {
	get := mocks.NewMockWeb(gomock.NewController(t))
	get.EXPECT().Post("https://gea.esac.esa.int/tap-server/login",
		url.Values{"username": {"ada"}, "password": {"pw"}}).Return("",
		nil)
	if err := gaia.SignIn(get, "ada", "pw"); err != nil {
		t.Fatal(err)
	}
}

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

func TestASavedSessionLoadsAsItWas(t *testing.T) {
	path := filepath.Join(t.TempDir(), "session")
	for range 2 { // the second is saved over the first
		if err := gaia.Save(session(), path); err != nil {
			t.Fatal(err)
		}
	}
	got, err := gaia.Load(path)
	if err != nil || len(got) != 1 || got[0].Value != "s1" {
		t.Fatalf("got %v, %v", got, err)
	}
}

func TestSaveRefusesWhatIsNotAPrivateSession(t *testing.T) {
	dir := t.TempDir()
	open := filepath.Join(dir, "open")
	if err := os.WriteFile(open, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	// json holds a time only from year 0 to 9999.
	far := []*http.Cookie{{Name: "JSESSIONID", Value: "s1",
		Expires: time.Date(10000, 1, 1, 0, 0, 0, 0, time.UTC)}}
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

func TestLoadRefusesWhatIsNotAPrivateSession(t *testing.T) {
	dir := t.TempDir()
	for name, file := range map[string]struct {
		text string
		mode os.FileMode
	}{"open": {`[{"Name":"JSESSIONID","Value":"s1"}]`, 0o644},
		"cookieless": {`[{"Name":"other","Value":"s1"}]`, 0o600},
		"garbled":    {`[{`, 0o600}} {
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
	if err := os.Mkdir(folder, 0o600); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{folder, filepath.Join(dir, "none")} {
		if _, err := gaia.Load(path); err == nil {
			t.Errorf("%s was loaded", path)
		}
	}
}
