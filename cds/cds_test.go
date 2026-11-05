package cds_test

import (
	"errors"
	"net/url"
	"testing"

	"github.com/janearc/celestial-data"
	"github.com/janearc/celestial-data/cds"
	"github.com/janearc/celestial-data/internal/replay"
)

// The survey and size the tests ask for.
const (
	survey = "CDS/P/DSS2/red"
	pixels = 512 // defined: a size the tests ask for, a power of two
	none   = 0   // defined: no pixels at all
	only   = 0   // defined: the place of a find's only product
)

// mirror is a host other than Strasbourg, to show the host is the one
// asked for.
const mirror = "alaskybis.cds.unistra.fr"

// cutoutAt is the one product a host's survey finds for the asked circle.
func cutoutAt(t *testing.T, host string) *celestialdata.Product {
	t.Helper()
	cutouts, err := cds.New(host, survey, pixels)
	if err != nil {
		t.Fatal(err)
	}
	products, err := cutouts.Find(replay.Asked(t))
	if err != nil || len(products) != len("1") {
		t.Fatalf("%v, %v", products, err)
	}
	return products[only]
}

// The cutout asks the service for the survey on the circle's square in
// the tangent projection. It is named for where it is.
func TestFindAsksForTheCirclesSquare(t *testing.T) {
	product := cutoutAt(t, cds.Strasbourg)
	address, err := url.Parse(product.URL())
	if err != nil {
		t.Fatal(err)
	}
	asked := address.Query()
	for key, want := range map[string]string{"hips": survey,
		"ra": "266.4168371", "dec": "-29.0078106", "fov": "0.2",
		"width": "512", "height": "512", "projection": "TAN",
		"coordsys": "icrs", "format": "fits"} {
		if asked.Get(key) != want {
			t.Errorf("%s is %q", key, asked.Get(key))
		}
	}
	if address.Scheme != "https" || address.Host != cds.Strasbourg ||
		address.Path != "/hips-image-services/hips2fits" ||
		product.Name() != "CDS_P_DSS2_red_266.4168371_-29.0078106_r0.1.fits" ||
		product.ID() != survey || product.Archive() != "hips" ||
		product.About() !=
			"CDS/P/DSS2/red within 0.1 degrees, 512 pixels a side" {
		t.Errorf("%s %s %q", product.URL(), product.Name(), product.About())
	}
}

// Mirrors are asked in Strasbourg's place for the same cutout.
func TestAMirrorIsAskedInStrasbourgsPlace(t *testing.T) {
	product := cutoutAt(t, mirror)
	address, err := url.Parse(product.URL())
	if err != nil || address.Host != mirror {
		t.Errorf("%s, %v", product.URL(), err)
	}
	home := cutoutAt(t, cds.Strasbourg)
	if product.Name() != home.Name() {
		t.Errorf("%s and %s", product.Name(), home.Name())
	}
}

// Anti-tests: New refuses no host and a host that is more than a name.
// It refuses no survey and no pixels.
func TestNewRefusesWhatCannotBeDrawn(t *testing.T) {
	for why, each := range map[string]struct {
		host, hips string
		pixels     int
	}{
		"no host":       {"", survey, pixels},
		"an address":    {"https://" + cds.Strasbourg, survey, pixels},
		"a host's path": {cds.Strasbourg + "/hips", survey, pixels},
		"no survey":     {cds.Strasbourg, "", pixels},
		"no pixels":     {cds.Strasbourg, survey, none},
	} {
		_, err := cds.New(each.host, each.hips, each.pixels)
		if !errors.Is(err, celestialdata.Refused()) {
			t.Errorf("%s: %v", why, err)
		}
	}
}

func TestTheAdapterIsCalledHips(t *testing.T) {
	cutouts, err := cds.New(cds.Strasbourg, survey, pixels)
	if err != nil || cutouts.Name() != "hips" {
		t.Errorf("%v", err)
	}
}

// The cutout comes whole: it is of the size the adapter gave it. Cutouts
// may be one pixel a side, the least.
func TestTheCutoutComesWholeAndOnePixelIsEnough(t *testing.T) {
	if err := replay.Fetched(t, cutoutAt(t, cds.Strasbourg)); err != nil {
		t.Errorf("%v", err)
	}
	if _, err := cds.New(cds.Strasbourg, survey, least); err != nil {
		t.Errorf("one pixel: %v", err)
	}
}

// least is the fewest pixels a side a cutout may be. It's defined.
const least = 1
