package celestialdata_test

import (
	"errors"
	"math"
	"os"
	"slices"
	"strings"
	"testing"

	"github.com/janearc/celestial-data"
	"github.com/janearc/celestial-data/web"
	"github.com/janearc/puzzlemouth/numbers"
	"pgregory.net/rapid"
)

// listed is the links Links reads from a recorded listing in testdata.
func listed(t *testing.T, name string) []string {
	t.Helper()
	page, err := os.ReadFile("testdata/" + name)
	if err != nil {
		t.Fatal(err)
	}
	return linksOf(t, page)
}

// linksOf is the links an adapter's Links reads from a page.
func linksOf(t *testing.T, page []byte) []string {
	t.Helper()
	get := served(t, page, lengthOf(page))
	links, err := listerOn(t, get).Links(where)
	if err != nil {
		t.Fatal(err)
	}
	return links
}

// listerOn is an adapter that lists folders through get.
func listerOn(t *testing.T, get web.Web) *celestialdata.Adapter {
	t.Helper()
	lister, err := celestialdata.NewAdapter("lister", get)
	if err != nil {
		t.Fatal(err)
	}
	return lister
}

// Chandra's archive lists a folder with quoted links. SARAO's page links
// its files with none.
func TestLinksAreReadHoweverTheyAreQuoted(t *testing.T) {
	chandra := listed(t, "listing-chandra.html")
	for _, want := range []string{"acisf03392N004_evt2.fits.gz",
		"pcadf03392_001N001_asol1.fits.gz", "responses/"} {
		if !slices.Contains(chandra, want) {
			t.Errorf("chandra: no %s in %q", want, chandra)
		}
	}
	sarao := listed(t, "listing-sarao.html")
	want := "https://archive-gw-1.kat.ac.za/public/repository/" +
		"10.48479/fyst-hj47/data/MeerKAT_Galactic_Centre_alpha.fits"
	if !slices.Contains(sarao, want) {
		t.Errorf("sarao: no %s in %q", want, sarao)
	}
}

func TestLinksReadSingleQuotesAndEntities(t *testing.T) {
	page := []byte(`<A HREF='first.fits'>a</A> <a href="x?a=1&amp;b=2">` +
		`b</a> <a href="">none</a> <a name="top"> <a href=x>one</a>`)
	got := linksOf(t, page)
	if !slices.Equal(got, []string{"first.fits", "x?a=1&b=2", "x"}) {
		t.Errorf("%q", got)
	}
}

// Anti-tests: a listing that does not come is an error. So is one cut
// short.
func TestAListingThatDoesNotComeIsAnError(t *testing.T) {
	refused := errors.New("not found")
	get := silent(t)
	get.EXPECT().Get(where).Return(nil, int64(unsaid), refused)
	_, err := listerOn(t, get).Links(where)
	if !errors.Is(err, refused) {
		t.Errorf("%v", err)
	}
	cut := silent(t)
	cut.EXPECT().Get(where).Return(&dropping{}, int64(unsaid), nil)
	if _, err := listerOn(t, cut).Links(where); err == nil {
		t.Error("a listing cut short read whole")
	}
}

// The square in testdata reaches half a degree each way on the sky. At
// latitude 60 that is a whole degree of longitude.
func TestFootprintIsReadFromTheCards(t *testing.T) {
	kept := measured(t)
	square, err := kept.Footprint(celestialdata.InICRS, "square")
	if err != nil {
		t.Fatal(err)
	}
	ra := picked(t, "square-crval1")
	dec := picked(t, "square-crval2")
	for offset, want := range map[float64]bool{
		unit - small: true,
		unit + small: false,
	} {
		near := circle(t, at(t, ra+offset, dec), small*small)
		if near.Reaches(square) != want {
			t.Errorf("%v degrees east: reached %v", offset, !want)
		}
	}
	flat, err := kept.Footprint(celestialdata.InGalactic, "square")
	if err != nil || flat == nil {
		t.Errorf("in the galactic frame: %v", err)
	}
}

// Anti-tests: an image with no cards is an error that says so. So is a
// numbers file that will not read.
func TestFootprintWithoutCardsIsAnError(t *testing.T) {
	_, err := measured(t).Footprint(celestialdata.InICRS, "nowhere")
	if err == nil || !strings.Contains(err.Error(), "nowhere-crval1") {
		t.Errorf("an image with no cards: %v", err)
	}
	raw := []byte("numbers: [")
	_, why := numbers.Read("broken.yaml", raw)
	broken := celestialdata.NewMeasured("broken.yaml", raw)
	if _, err := broken.Get("rounding"); err == nil ||
		err.Error() != why.Error() {
		t.Errorf("a file that does not read: %v, not %v", err, why)
	}
}

// spans is the property of TestSpanIsTheImagesWidth, for one draw.
func spans(t *rapid.T) {
	value := rapid.Float64Range(-widest, widest).Draw(t, "value")
	pixel := rapid.Float64Range(-raEnd, raEnd).Draw(t, "pixel")
	step := rapid.Float64Range(-unit, unit).Draw(t, "step")
	length := rapid.Float64Range(unit, raEnd).Draw(t, "length")
	centre, half := celestialdata.Span(value, pixel, step, length)
	wantHalf := math.Abs(length*step) / both
	wantCentre := value + ((length+unit)/both-pixel)*step
	if math.Abs(half-wantHalf) > rounding ||
		math.Abs(centre-wantCentre) > rounding {
		t.Fatalf("%v, %v; want %v, %v", centre, half, wantCentre,
			wantHalf)
	}
}

// both is the count of an axis's ends. It's defined.
const both = 2

// Property: an axis reaches half its length either side of its middle
// pixel, wherever the reference pixel is.
func TestSpanIsTheImagesWidth(t *testing.T) {
	rapid.Check(t, spans)
}

// The numbers file names a circle at the square's centre a hundredth of
// a degree across. Missing numbers are each an error that names them.
// So are places off the sky and radii below nothing.
func TestCircleIsReadFromItsNumbers(t *testing.T) {
	kept := measured(t)
	near, err := kept.Circle("square-crval1", "square-crval2",
		"square-cdelt2")
	if err != nil || near.Centre().RA() != picked(t, "square-crval1") ||
		near.Radius() != picked(t, "square-cdelt2") {
		t.Errorf("%v, %v", near, err)
	}
	for says, names := range map[string]struct{ ra, dec, radius string }{
		"nowhere":             {"nowhere", "square-crval2", "square-cdelt2"},
		"right ascension 400": {"off-sky", "square-crval2", "square-cdelt2"},
		"radius -0.01":        {"square-crval1", "square-crval2", "square-cdelt1"},
	} {
		_, err := kept.Circle(names.ra, names.dec, names.radius)
		if err == nil || !strings.Contains(err.Error(), says) {
			t.Errorf("%s: %v", says, err)
		}
	}
}
