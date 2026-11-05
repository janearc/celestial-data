package replay_test

import (
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/janearc/celestial-data"
	"github.com/janearc/celestial-data/internal/replay"
)

// address is where the recorded reply is said to come from.
const address = "https://archive.example/answer"

func TestTheRecordedReplyIsTheAnswer(t *testing.T) {
	get := replay.Web(t, map[string]string{address: "answer.txt"})
	body, length, err := get.Get(address)
	if err != nil {
		t.Fatal(err)
	}
	defer body.Close()
	read, err := io.ReadAll(body)
	if err != nil || string(read) != "photons\n" ||
		length != int64(len(read)) {
		t.Errorf("%q, %d, %v", read, length, err)
	}
}

// stopped is a test that records being stopped, so a test of replay can
// watch it stop one.
type stopped struct {
	testing.TB
	said bool
}

// Fatalf records that the test was stopped, and why.
func (watched *stopped) Fatalf(string, ...any) { watched.said = true }

// Anti-test: a reply that was never recorded stops the test.
func TestAMissingRecordingStopsTheTest(t *testing.T) {
	watched := &stopped{TB: t}
	get := replay.Web(watched, map[string]string{address: "nowhere.txt"})
	if !watched.said || get != nil {
		t.Error("a missing recording went unsaid")
	}
}

// The asked circle is a tenth of a degree round Sgr A*.
func TestTheAskedCircleIsRoundSgrAStar(t *testing.T) {
	asked := replay.Asked(t)
	galactic := asked.Centre().Galactic()
	if asked.Radius() <= nothing || galactic.Longitude() < westOfCentre ||
		galactic.Latitude() > nothing {
		t.Errorf("%v at %v, %v", asked.Radius(), galactic.Longitude(),
			galactic.Latitude())
	}
}

// The bounds the asked circle is held within, in degrees. Sgr A* lies a
// little west of the galactic centre and a little south of the plane.
const (
	nothing      = 0   // defined: no radius, and the galactic plane
	westOfCentre = 359 // defined: a degree west of the galactic centre
)

// The live client asks a real server, and says what it is.
func TestTheLiveClientAsks(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(answerAgent))
	defer server.Close()
	body, _, err := replay.Live(t).Get(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	defer body.Close()
	said, err := io.ReadAll(body)
	if err != nil || string(said) != "celestial-data-live-check" {
		t.Errorf("%q, %v", said, err)
	}
}

// answerAgent answers with the user agent it was sent.
func answerAgent(writer http.ResponseWriter, request *http.Request) {
	io.WriteString(writer, request.UserAgent())
}

// Products of the size they say fetch whole, gzip or not. So do those of none.
func TestAFetchedProductComesWhole(t *testing.T) {
	made, err := celestialdata.Named("test")
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"a.txt", "a.fits.gz"} {
		product, err := made.Product("1", name, address, unsaid, "")
		if err != nil {
			t.Fatal(err)
		}
		if err := replay.Fetched(t, product); err != nil {
			t.Errorf("%s: %v", name, err)
		}
	}
	wrong, err := made.Product("1", "a.txt", address, more, "")
	if err != nil {
		t.Fatal(err)
	}
	if err := replay.Fetched(t, wrong); err == nil {
		t.Error("a product of the wrong size came whole")
	}
}

// The sizes a test product says. Each is defined.
const (
	unsaid = 0  // defined by celestialdata.NewProduct: no size said
	more   = 99 // defined: more than the test file holds
)
