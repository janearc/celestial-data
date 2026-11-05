package chroma_test

import (
	"errors"
	"io"
	"strings"
	"testing"
	"testing/iotest"

	"github.com/janearc/celestial-data"
	"github.com/janearc/celestial-data/chroma"
	"github.com/janearc/celestial-data/internal/replay"
	"github.com/janearc/celestial-data/mocks"
	"go.uber.org/mock/gomock"
)

// The replies to a listing of the folder are kept in testdata. recorded.json
// is GitHub's own, fetched on 2026-11-02 with curl from the address below.
// contents.json is written by hand in the same shape, to hold what a real
// listing may: a folder named like a colourway, a file with no download
// address, a name that leads out of the folder.
const sources = "https://api.github.com/repos/janearc/chroma/contents/" +
	"colourways/sources?ref=main"

// raw is where GitHub serves the colourways from.
const raw = "https://raw.githubusercontent.com/janearc/chroma/main/" +
	"colourways/sources/"

// The counts the tests work with.
const (
	colourways = 2  // defined: the .css files contents.json holds
	recorded   = 16 // defined: the .css files GitHub listed on 2026-11-02
	first      = 0  // defined: the first product
	second     = 1  // defined: the second product
)

// colourwaysOf is the archive asked through get.
func colourwaysOf(t *testing.T, get *mocks.MockWeb) *chroma.Colourways {
	t.Helper()
	colours, err := chroma.New(get)
	if err != nil {
		t.Fatal(err)
	}
	return colours
}

// answering is a web that answers the listing once with a recorded file.
func answering(t *testing.T, recorded string) *mocks.MockWeb {
	t.Helper()
	return replay.Web(t, map[string]string{sources: recorded})
}

// Each .css file the listing holds is found, in the order listed, with
// the address it is downloaded from. Nothing else in the folder is.
func TestFindNamesEachColourway(t *testing.T) {
	colours := colourwaysOf(t, answering(t, "contents.json"))
	products, err := colours.Find(replay.Asked(t))
	if err != nil || len(products) != colourways {
		t.Fatalf("%d products, %v", len(products), err)
	}
	for at, name := range map[int]string{first: "aqua.css",
		second: "twilight.css"} {
		product := products[at]
		if product.Name() != name || product.URL() != raw+name ||
			product.ID() != "janearc/chroma" ||
			product.Archive() != "chroma" ||
			!strings.Contains(product.About(), "colourway") {
			t.Errorf("%s %s %s %q", product.Archive(), product.ID(),
				product.URL(), product.About())
		}
	}
}

// GitHub's own listing gives jane's sixteen colourways, and not the
// folder's SOURCES.md or its samples folder.
func TestGitHubsListingGivesEveryColourway(t *testing.T) {
	colours := colourwaysOf(t, answering(t, "recorded.json"))
	products, err := colours.Find(replay.Asked(t))
	if err != nil || len(products) != recorded {
		t.Fatalf("%d products, %v", len(products), err)
	}
	for _, product := range products {
		name := product.Name()
		if !strings.HasSuffix(name, ".css") || product.URL() != raw+name {
			t.Errorf("%s at %s", name, product.URL())
		}
	}
}

// The files do not depend on the circle: it is asked only by name, and
// asked the same for any.
func TestTheColourwaysAreAskedOnlyByName(t *testing.T) {
	get := mocks.NewMockWeb(gomock.NewController(t))
	colours := colourwaysOf(t, get)
	named, ok := celestialdata.Archive(colours).(celestialdata.ByName)
	if !ok || !named.ByNameOnly() {
		t.Error("the colourways are asked with every archive")
	}
	if colours.Name() != "chroma" {
		t.Errorf("the archive is called %q", colours.Name())
	}
}

// Anti-tests: an archive with no web is refused.
func TestNoWebIsRefused(t *testing.T) {
	_, err := chroma.New(nil)
	if err == nil || !strings.Contains(err.Error(), "chroma needs a web") {
		t.Errorf("no web gave %v", err)
	}
}

// Anti-tests: a listing that is not a list of files, one with no
// colourway in it, and a listing that cannot be had are errors that name
// the archive.
func TestAListingThatIsNotColourwaysIsAnError(t *testing.T) {
	for says, recorded := range map[string]string{
		"chroma: the listing at " + sources + " is not a list": "limit.json",
		"chroma: no .css file is listed at " + sources:         "none.json",
	} {
		colours := colourwaysOf(t, answering(t, recorded))
		products, err := colours.Find(replay.Asked(t))
		if err == nil || !strings.Contains(err.Error(), says) ||
			products != nil {
			t.Errorf("%s gave %v, %v", recorded, products, err)
		}
	}
}

// Anti-tests: a listing GitHub will not give, and one that breaks off,
// are errors that name the archive.
func TestAListingThatDoesNotComeIsAnError(t *testing.T) {
	down := errors.New("down")
	broken := io.NopCloser(iotest.ErrReader(down))
	for _, each := range []struct {
		body io.ReadCloser
		err  error
	}{{nil, down}, {broken, nil}} {
		get := mocks.NewMockWeb(gomock.NewController(t))
		length := int64(len(raw))
		get.EXPECT().Get(sources).Return(each.body, length, each.err)
		_, err := colourwaysOf(t, get).Find(replay.Asked(t))
		if !errors.Is(err, down) || !strings.HasPrefix(err.Error(),
			"chroma: ") {
			t.Errorf("the listing gave %v", err)
		}
	}
}
