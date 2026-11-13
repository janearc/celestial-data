package horizons_test

import (
	"strings"
	"testing"

	"github.com/janearc/celestial-data"
	"github.com/janearc/celestial-data/horizons"
	"github.com/janearc/celestial-data/internal/replay"
)

// What the archive holds, as Horizons' API page says a question is
// written: the whole address of each, by hand, so a change to how the
// package builds one shows here.
const (
	sunAddress = "https://ssd.jpl.nasa.gov/api/horizons.api?format=text" +
		"&MAKE_EPHEM=YES&OBJ_DATA=NO&EPHEM_TYPE=VECTORS&COMMAND=%2710%27" +
		"&CENTER=%27500@-170%27&START_TIME=%272027-05-22%2020:53%27" +
		"&STOP_TIME=%272027-05-22%2020:54%27&STEP_SIZE=%271%20min%27" +
		"&REF_PLANE=FRAME&REF_SYSTEM=ICRF&VEC_TABLE=2&OUT_UNITS=KM-S" +
		"&CSV_FORMAT=YES"
	earthAddress = "https://ssd.jpl.nasa.gov/api/horizons.api?format=text" +
		"&MAKE_EPHEM=YES&OBJ_DATA=NO&EPHEM_TYPE=VECTORS&COMMAND=%27399%27" +
		"&CENTER=%27500@-170%27&START_TIME=%272027-05-22%2020:53%27" +
		"&STOP_TIME=%272027-05-22%2020:54%27&STEP_SIZE=%271%20min%27" +
		"&REF_PLANE=FRAME&REF_SYSTEM=ICRF&VEC_TABLE=2&OUT_UNITS=KM-S" +
		"&CSV_FORMAT=YES"
	moonAddress = "https://ssd.jpl.nasa.gov/api/horizons.api?format=text" +
		"&MAKE_EPHEM=YES&OBJ_DATA=NO&EPHEM_TYPE=VECTORS&COMMAND=%27301%27" +
		"&CENTER=%27500@-170%27&START_TIME=%272027-05-22%2020:53%27" +
		"&STOP_TIME=%272027-05-22%2020:54%27&STEP_SIZE=%271%20min%27" +
		"&REF_PLANE=FRAME&REF_SYSTEM=ICRF&VEC_TABLE=2&OUT_UNITS=KM-S" +
		"&CSV_FORMAT=YES"
	bodies = 3 // defined: the sun, the earth and the moon
)

// held is one file the archive is to hold.
type held struct{ id, name, address, about string }

// wanted is the files the archive is to hold, in its order.
var wanted = []held{
	{"sun from webb", "sun-from-webb-2027-05-22.txt", sunAddress,
		"the sun as seen from Webb at 2027-05-22 20:53, from JPL Horizons"},
	{"earth from webb", "earth-from-webb-2027-05-22.txt", earthAddress,
		"the earth as seen from Webb at 2027-05-22 20:53, from JPL Horizons"},
	{"moon from webb", "moon-from-webb-2027-05-22.txt", moonAddress,
		"the moon as seen from Webb at 2027-05-22 20:53, from JPL Horizons"},
}

// found is what the archive New gives holds.
func found(t *testing.T) []*celestialdata.Product {
	t.Helper()
	archive, err := horizons.New()
	if err != nil {
		t.Fatal(err)
	}
	products, err := archive.Find(replay.Asked(t))
	if err != nil {
		t.Fatal(err)
	}
	return products
}

// the test: the archive is named for the command line, and is asked only
// by name.
func TestTheArchiveIsAskedOnlyByName(t *testing.T) {
	archive, err := horizons.New()
	if err != nil {
		t.Fatal(err)
	}
	var asked celestialdata.ByName = archive
	if asked.Name() != "horizons" || !asked.ByNameOnly() {
		t.Errorf("%q, by name only %v", asked.Name(), asked.ByNameOnly())
	}
}

// the test: the archive holds the sun, the earth and the moon as seen
// from Webb, each by the whole address Horizons is asked at.
func TestTheArchiveHoldsTheThreeAsWebbSeesThem(t *testing.T) {
	products := found(t)
	if len(products) != bodies {
		t.Fatalf("%d products", len(products))
	}
	for at, want := range wanted {
		got := products[at]
		if got.Archive() != "horizons" || got.ID() != want.id ||
			got.Name() != want.name || got.URL() != want.address ||
			got.About() != want.about {
			t.Errorf("%s %s %s %s %q", got.Archive(), got.ID(), got.Name(),
				got.URL(), got.About())
		}
	}
}

// the anti-test: no two questions are one question. Each asks after its
// own body and is kept under its own name, and they differ in nothing
// else: the same middle, the same moment, the same frame.
func TestNoTwoQuestionsAreOne(t *testing.T) {
	products := found(t)
	for at, one := range products {
		for _, other := range products[at+1:] {
			if one.URL() == other.URL() || one.Name() == other.Name() ||
				one.ID() == other.ID() {
				t.Errorf("%s and %s are asked or kept as one", one.ID(),
					other.ID())
			}
		}
		_, after, cut := strings.Cut(one.URL(), "&CENTER=")
		_, same, _ := strings.Cut(sunAddress, "&CENTER=")
		if !cut || after != same {
			t.Errorf("%s is asked another way after its body: %s", one.ID(),
				after)
		}
	}
}

// the anti-test: nothing of the asker goes in a question. Horizons takes
// an address to write to and the archive sends none, and no question
// asks Horizons for more than a table of vectors.
func TestAQuestionSaysNothingOfWhoAsks(t *testing.T) {
	for _, each := range found(t) {
		for _, never := range []string{"EMAIL_ADDR", "OBSERVER", "ELEMENTS",
			"SPK", " ", "'"} {
			if strings.Contains(each.URL(), never) {
				t.Errorf("%s: the question holds %q", each.ID(), never)
			}
		}
		// the one at-sign is the middle's, 500@-170: no address to
		// write to is anywhere in the question
		if strings.Count(each.URL(), "@") != 1 {
			t.Errorf("%s: the question holds an at-sign that is not the "+
				"middle's", each.ID())
		}
	}
}

// the test: each answer is fetched whole through the web it is given,
// with no length expected ahead, since Horizons says none. No question
// goes to the network: replay's mock gives a few bytes.
func TestEachAnswerIsFetchedWithNoLengthAhead(t *testing.T) {
	for _, each := range found(t) {
		if err := replay.Fetched(t, each); err != nil {
			t.Errorf("%s was not fetched: %v", each.ID(), err)
		}
	}
}

// the anti-test: the archive is the same whatever circle is asked, and
// asks nothing to say what it holds: it is found twice the same.
func TestTheArchiveIsTheSameAskedTwice(t *testing.T) {
	once, again := found(t), found(t)
	for at := range once {
		if once[at].URL() != again[at].URL() ||
			once[at].Name() != again[at].Name() {
			t.Errorf("%s changed between two askings", once[at].ID())
		}
	}
}
