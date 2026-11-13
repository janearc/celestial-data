package k2_test

import (
	"errors"
	"os"
	"strings"
	"testing"

	"go.uber.org/mock/gomock"

	"github.com/janearc/celestial-data"
	"github.com/janearc/celestial-data/internal/replay"
	"github.com/janearc/celestial-data/k2"
	"github.com/janearc/celestial-data/mocks"
)

// The recorded search: firang's star, Gaia DR3 4081136684118965760, in a
// circle of 0.003 degrees, asked of MAST through this adapter on
// 2026-11-06. Each defined by what MAST answered.
const (
	starRA       = 287.50086  // defined: the star's right ascension
	starDec      = -22.23481  // defined: its declination
	starWide     = 0.003      // defined: the circle's radius, degrees
	adapter      = "k2"       // defined: the archive's name
	scienceFiles = 8          // defined: campaign 7's two images, four forms each
	imageID      = "23796168" // defined: the images' CAOM obsid
	first        = 0          // defined: the first product's place
	firstName    = "ktwo2015331045232-c07_ffi-cal.fits"
	firstAbout   = "a K2 image of k2-c07, campaign 7, 1800 s, from K2"
	firstURL     = "https://mast.stsci.edu/api/v0.1/Download/file?uri=" +
		"mast%3AK2%2Furl%2Fmissions%2Fk2%2Fffi%2F" + firstName
	noLength = 0 // defined: a failed answer's length
	none     = 0 // defined: no products
)

// star is the recorded circle, or the test stops.
func star(test *testing.T) *celestialdata.Near {
	test.Helper()
	centre, err := celestialdata.NewPosition(starRA, starDec)
	if err != nil {
		test.Fatal(err)
	}
	near, err := celestialdata.NewNear(centre, starWide)
	if err != nil {
		test.Fatal(err)
	}
	return near
}

// answering is the archive asked through a web that answers the cone
// and the products with the files given, or the test stops.
func answering(test *testing.T, cone, products string) *k2.Archive {
	test.Helper()
	replies := map[string]string{recorded(test, "asked-cone.txt"): cone}
	if products != "" {
		replies[recorded(test, "asked-products.txt")] = products
	}
	archive, err := k2.New(replay.Web(test, replies))
	if err != nil {
		test.Fatal(err)
	}
	return archive
}

// the test: the archive is named, asked by name only, and refuses no
// web.
func TestTheArchiveIsAskedByName(test *testing.T) {
	get := mocks.NewMockWeb(gomock.NewController(test))
	archive, err := k2.New(get)
	if err != nil || archive.Name() != adapter || !archive.ByNameOnly() {
		test.Errorf("%v, %v", archive, err)
	}
	if nowhere, err := k2.New(nil); nowhere != nil || err == nil {
		test.Errorf("no web: %v", err)
	}
}

// the test: of the eleven observations MAST gave over the star, K2's
// campaign 7 images are the only K2 one, and its eight science files
// are found and fetched by their data URIs; its previews and the
// other missions' observations are not.
func TestASearchFindsCampaignSevensImages(test *testing.T) {
	products, err := answering(test, "cone.json", "products.json").Find(
		star(test))
	if err != nil || len(products) != scienceFiles {
		test.Fatalf("%d products, %v", len(products), err)
	}
	image := products[first]
	if image.Archive() != adapter || image.ID() != imageID ||
		image.Name() != firstName || image.URL() != firstURL ||
		image.About() != firstAbout {
		test.Errorf("%s %s %s %s %q", image.Archive(), image.ID(),
			image.Name(), image.URL(), image.About())
	}
	for _, each := range products {
		if !strings.HasSuffix(each.Name(), ".fits") {
			test.Errorf("%s is not a science file", each.Name())
		}
	}
}

// refused is a recorded search that is refused, and its refusal's words.
type refused struct{ cone, products, said string }

// the anti-test: a search that cannot be read is refused in the
// archive's name: a cone MAST says failed, products MAST says failed,
// and a product whose name is not a plain file's.
func TestASearchNotReadIsRefused(test *testing.T) {
	for _, each := range []refused{
		{"error.json", "", "Mast.Caom.Cone says ERROR"},
		{"cone.json", "error.json", "Mast.Caom.Products says ERROR"},
		{"cone.json", "products-not-plain.json", "plain"}} {
		found, err := answering(test, each.cone, each.products).Find(
			star(test))
		if found != nil || err == nil || !named(err) ||
			!strings.Contains(err.Error(), each.said) {
			test.Errorf("%s, %s: %v, %v", each.cone, each.products,
				found, err)
		}
	}
}

// the anti-test: a search the web cannot answer is refused in the
// archive's name, wrapping why.
func TestASearchTheWebCannotAnswerIsRefused(test *testing.T) {
	unreachable := errors.New("no route to MAST")
	get := mocks.NewMockWeb(gomock.NewController(test))
	get.EXPECT().Get(recorded(test, "asked-cone.txt")).Return(nil,
		int64(noLength), unreachable)
	archive, err := k2.New(get)
	if err != nil {
		test.Fatal(err)
	}
	found, err := archive.Find(star(test))
	if found != nil || !errors.Is(err, unreachable) || !named(err) {
		test.Errorf("%v, %v", found, err)
	}
}

// named is whether an error names the archive, as a failure does
// ("k2: ...") or a refusal ("refused: k2 ...").
func named(err error) bool {
	said := err.Error()
	return strings.HasPrefix(said, adapter+":") ||
		strings.HasPrefix(said, "refused: "+adapter+" ")
}

// recorded is the address a recorded search asked, or the test stops.
func recorded(test *testing.T, file string) string {
	test.Helper()
	asked, err := os.ReadFile("testdata/" + file)
	if err != nil {
		test.Fatal(err)
	}
	return strings.TrimSpace(string(asked))
}

// kind is a crafted cone of one K2 observation of a kind, and how many
// products it is found with.
type kind struct {
	cone  string
	found int
}

// the test: of K2's own observations, a light curve is kept and a kind
// that is neither a light curve nor an image is not, whose products are
// then never asked for. Each cone is written by hand, one row of the
// recorded image's with its kind changed.
func TestOnlyLightCurvesAndImagesAreKept(test *testing.T) {
	for _, each := range []kind{{"cone-k2-timeseries.json", scienceFiles},
		{"cone-k2-spectrum.json", none}} {
		products := ""
		if each.found != none {
			products = "products.json"
		}
		found, err := answering(test, each.cone, products).Find(star(test))
		if err != nil || len(found) != each.found {
			test.Errorf("%s: %d products, %v", each.cone, len(found), err)
		}
	}
}
