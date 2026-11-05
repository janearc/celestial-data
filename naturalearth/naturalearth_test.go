package naturalearth_test

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"strconv"
	"strings"
	"testing"

	"github.com/janearc/celestial-data"
	"github.com/janearc/celestial-data/internal/replay"
	"github.com/janearc/celestial-data/mocks"
	"github.com/janearc/celestial-data/naturalearth"
	"go.uber.org/mock/gomock"
)

// excerpt is three real features of the land: an islet in the Florida
// Keys, a pair of polygons off the Russian coast, and Afro-Eurasia, whose
// outline and hole are thinned. testdata/README.md says how it was cut.
const excerpt = "testdata/land-excerpt.geojson"

// The pieces of land in the excerpt.
const (
	pieces  = 4 // defined: one islet, two polygons, Afro-Eurasia
	rings   = 2 // defined: Afro-Eurasia's outline and the Caspian Sea
	last    = 3 // defined: the place of Afro-Eurasia among the pieces
	hole    = 1 // defined: the place of the Caspian among its rings
	outline = 0 // defined: the place of the outline among its rings
	// The thinned rings' points, as testdata/README.md counts them.
	outlinePoints = 207 // defined: every 50th of 10297, and the last
	holePoints    = 99  // defined: every 4th of 391, and the last
)

// Places chosen by reading the excerpt's coordinates, in degrees.
const (
	// in the islet, a sliver between (-80.6651, 24.8984), (-80.6257,
	// 24.9411) and (-80.6146, 24.9379)
	keyLon = -80.636 // defined: inside the islet
	keyLat = 24.92   // defined: inside the islet
	// within the islet's box and west of its edge
	cornerLon = -80.66 // defined: in the box, off the islet
	cornerLat = 24.93  // defined: in the box, off the islet
	// in the larger of the two polygons off the coast
	coastLon = 52.9 // defined: inside the polygon
	coastLat = 71.2 // defined: inside the polygon
	// in the pair's box and south of the polygon's southern shore
	bayLon = 52.3  // defined: in the box, off the polygon
	bayLat = 70.99 // defined: in the box, off the polygon
	// Moscow, deep in Eurasia
	moscowLon = 37.62 // defined: inland
	moscowLat = 55.75 // defined: inland
	// the middle of the Caspian Sea, in the hole of Eurasia
	caspianLon = 51.0 // defined: in the hole
	caspianLat = 42.0 // defined: in the hole
	// the Gulf of Guinea, in Afro-Eurasia's box and off its shore
	gulfLon = 0.0 // defined: at sea
	gulfLat = 0.0 // defined: at sea
	// the middle of the Pacific, in no piece's box
	pacificLon = -150.0 // defined: at sea
	pacificLat = 0.0    // defined: at sea
)

// mapOf is the land the excerpt holds.
func mapOf(t *testing.T) *naturalearth.Map {
	t.Helper()
	file, err := os.Open(excerpt)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	land, err := naturalearth.Read(file)
	if err != nil {
		t.Fatal(err)
	}
	return land
}

// The pieces are the file's polygons and the multipolygon's, each with
// its outline first.
func TestReadKeepsEveryPieceAndItsHoles(t *testing.T) {
	parts := mapOf(t).Polygons()
	if len(parts) != pieces {
		t.Fatalf("%d pieces", len(parts))
	}
	var eurasia naturalearth.Polygon = parts[last]
	if len(eurasia) != rings || len(eurasia[outline]) != outlinePoints ||
		len(eurasia[hole]) != holePoints {
		t.Errorf("%d rings of %d and %d points", len(eurasia),
			len(eurasia[outline]), len(eurasia[hole]))
	}
}

// A place is held when it is on land: in a piece, and not in its hole.
func TestHoldsLandButNotItsHolesOrTheSea(t *testing.T) {
	land := mapOf(t)
	for _, each := range []struct {
		why      string
		lon, lat float64
		want     bool
	}{
		{"in the islet", keyLon, keyLat, true},
		{"in the islet's box, off it", cornerLon, cornerLat, false},
		{"in a polygon of a multipolygon", coastLon, coastLat, true},
		{"in the pair's box, off them", bayLon, bayLat, false},
		{"inland in Eurasia", moscowLon, moscowLat, true},
		{"in the Caspian, Eurasia's hole", caspianLon, caspianLat, false},
		{"in the sea, in Eurasia's box", gulfLon, gulfLat, false},
		{"in the sea, in no box", pacificLon, pacificLat, false},
	} {
		if got := land.Holds(each.lon, each.lat); got != each.want {
			t.Errorf("%s: Holds(%v, %v) = %v", each.why, each.lon,
				each.lat, got)
		}
	}
}

// Two pieces of land drawn by hand, each two degrees across.
const (
	square = `{"features": [{"geometry": {"type": "Polygon",
		"coordinates": [[[0, 0], [0, 2], [2, 2], [2, 0], [0, 0]]]}}]}`
	diamond = `{"features": [{"geometry": {"type": "Polygon",
		"coordinates": [[[1, 0], [2, 1], [1, 2], [0, 1], [1, 0]]]}}]}`
)

// The square's edges, and the places tried on and about them, in degrees.
const (
	squareWest  = 0 // defined: the west edge of square
	squareSouth = 0 // defined: the south edge of square
	squareEast  = 2 // defined: the east edge of square
	squareNorth = 2 // defined: the north edge of square
	squareMid   = 1 // defined: half way along each edge of square
	off         = 1 // defined: how far past an edge a place is tried
)

// A place on a piece's west or south edge is on it, and one on its east
// or north edge is not, so that pieces side by side leave no place
// held twice. A place beyond an edge is off it.
func TestHoldsTakesTheWestAndSouthEdgesNotTheEastAndNorth(t *testing.T) {
	land, err := naturalearth.Read(strings.NewReader(square))
	if err != nil {
		t.Fatal(err)
	}
	for _, each := range []struct {
		why      string
		lon, lat float64
		want     bool
	}{
		{"inside", squareMid, squareMid, true},
		{"on the west edge", squareWest, squareMid, true},
		{"on the south edge", squareMid, squareSouth, true},
		{"on the east edge", squareEast, squareMid, false},
		{"on the north edge", squareMid, squareNorth, false},
		{"west of it", squareWest - off, squareMid, false},
		{"south of it", squareMid, squareSouth - off, false},
		{"east of it", squareEast + off, squareMid, false},
		{"north of it", squareMid, squareNorth + off, false},
	} {
		if got := land.Holds(each.lon, each.lat); got != each.want {
			t.Errorf("%s: Holds(%v, %v) = %v", each.why, each.lon,
				each.lat, got)
		}
	}
}

// The latitude of the diamond's west and east corners, and the longitudes
// tried along it, in degrees.
const (
	level     = 1    // defined: the latitude of both corners
	insideWat = 0.5  // defined: inside, between the west corner and the middle
	insideEat = 1.5  // defined: inside, between the middle and the east corner
	beyondW   = -0.5 // defined: level with the corners, west of them
	beyondE   = 2.5  // defined: level with the corners, east of them
)

// A place level with a corner of a piece is held when it is inside,
// and not when it is outside.
func TestHoldsPlacesLevelWithACorner(t *testing.T) {
	land, err := naturalearth.Read(strings.NewReader(diamond))
	if err != nil {
		t.Fatal(err)
	}
	for _, each := range []struct {
		why      string
		lon, lat float64
		want     bool
	}{
		{"inside, west of the middle", insideWat, level, true},
		{"inside, east of the middle", insideEat, level, true},
		{"level with both corners, west of them", beyondW, level, false},
		{"level with both corners, east of them", beyondE, level, false},
	} {
		if got := land.Holds(each.lon, each.lat); got != each.want {
			t.Errorf("%s: Holds(%v, %v) = %v", each.why, each.lon,
				each.lat, got)
		}
	}
}

// The wedge of land, and the places tried on it. Along its edge from
// (-30, 5) to (2.2, 1) the crossing at latitude 1 is worked out as a hair
// east of 2.2.
const (
	wedge = `{"features": [{"geometry": {"type": "Polygon",
		"coordinates": [[[-30, 5], [2.2, 1], [-30, 1], [-30, 5]]]}}]}`
	wedgeEast  = 2.2 // defined: the longitude of the wedge's east corner
	wedgeSouth = 1   // defined: the latitude of the wedge's east corner
	wedgeInLon = -20 // defined: inside the wedge
	wedgeInLat = 2   // defined: inside the wedge
	positive   = 1   // defined: math.Inf's sign for positive infinity
)

// A place just east of a piece is off it, though the ray test alone says
// otherwise: it is west of the crossing as worked out. The piece's box
// keeps the place off.
func TestHoldsKeepsAPlaceBeyondTheBoxOffThePiece(t *testing.T) {
	land, err := naturalearth.Read(strings.NewReader(wedge))
	if err != nil {
		t.Fatal(err)
	}
	past := math.Nextafter(wedgeEast, math.Inf(positive))
	if land.Holds(past, wedgeSouth) {
		t.Errorf("a place at %v, east of the piece, is held", past)
	}
	if land.Holds(wedgeEast, wedgeSouth) {
		t.Error("the east corner, on the east edge, is held")
	}
	if !land.Holds(wedgeInLon, wedgeInLat) {
		t.Error("a place inside the piece is not held")
	}
}

// The numbers of a position with a height, and the parts it keeps.
const (
	heightLon = 10  // defined: the longitude of the first position
	heightLat = 20  // defined: the latitude of the first position
	positions = 2   // defined: longitude and latitude
	longitude = 0   // defined: where a position keeps its longitude
	latitude  = 1   // defined: where a position keeps its latitude
	heightNo  = 300 // defined: the height, which is not kept
	stride    = 2   // defined: how far the other positions lie
)

// A position is longitude and latitude and no more: a height that GeoJSON
// allows after them is not kept.
func TestAPositionKeepsOnlyLongitudeAndLatitude(t *testing.T) {
	text := fmt.Sprintf(`{"features": [{"geometry": {"type": "Polygon",
		"coordinates": [[[%d, %d, %d], [%d, %d, %d], [%d, %d, %d]]]}}]}`,
		heightLon, heightLat, heightNo,
		heightLon, heightLat+stride, heightNo,
		heightLon+stride, heightLat+stride, heightNo)
	land, err := naturalearth.Read(strings.NewReader(text))
	if err != nil {
		t.Fatal(err)
	}
	corner := land.Polygons()[first][outline][first]
	if len(corner) != positions || corner[longitude] != heightLon ||
		corner[latitude] != heightLat {
		t.Errorf("a position of %d parts: %v", len(corner), corner)
	}
}

// The land of no features, and a piece with an empty ring, hold nothing.
func TestHoldsNothingWhereThereIsNoLand(t *testing.T) {
	for _, shape := range []string{
		`{"features": []}`,
		`{"features": [{"geometry": {"type": "Polygon",
			"coordinates": [[]]}}]}`,
	} {
		land, err := naturalearth.Read(strings.NewReader(shape))
		if err != nil || land.Holds(keyLon, keyLat) {
			t.Errorf("%s: %v", shape, err)
		}
	}
}

// Anti-tests: Read refuses a shape it does not read, and a polygon with
// no outline, in the package's words.
func TestReadRefusesWhatIsNotLand(t *testing.T) {
	for _, each := range []struct {
		why, file, saying string
	}{
		{"a point", `{"features": [{"geometry": {"type": "Point",
			"coordinates": [0, 0]}}]}`,
			`naturalearth holds no land of the shape "Point"`},
		{"no geometry", `{"features": [{"geometry": null}]}`,
			`naturalearth holds no land of the shape ""`},
		{"a polygon of no rings", `{"features": [{"geometry": {
			"type": "Polygon", "coordinates": []}}]}`,
			"naturalearth holds a polygon with no outline"},
		{"a multipolygon with one", `{"features": [{"geometry": {
			"type": "MultiPolygon", "coordinates": [[]]}}]}`,
			"naturalearth holds a polygon with no outline"},
	} {
		land, err := naturalearth.Read(strings.NewReader(each.file))
		if land != nil || !errors.Is(err, celestialdata.Refused()) ||
			!strings.Contains(err.Error(), each.saying) {
			t.Errorf("%s: %v, %v", each.why, land, err)
		}
	}
}

// Anti-tests: a file that is not JSON, and coordinates that are not
// numbers, are errors that name the package and wrap what json said.
func TestReadSaysWhyAFileWillNotRead(t *testing.T) {
	var syntax *json.SyntaxError
	var typed *json.UnmarshalTypeError
	for _, each := range []struct {
		why, file string
		wrap      any
	}{
		{"not json", `land`, &syntax},
		{"empty", ``, nil},
		{"polygon coordinates", `{"features": [{"geometry": {"type":
			"Polygon", "coordinates": "here"}}]}`, &typed},
		{"multipolygon coordinates", `{"features": [{"geometry": {"type":
			"MultiPolygon", "coordinates": "there"}}]}`, &typed},
	} {
		land, err := naturalearth.Read(strings.NewReader(each.file))
		if land != nil || err == nil ||
			!strings.HasPrefix(err.Error(), "naturalearth: ") {
			t.Errorf("%s: %v, %v", each.why, land, err)
			continue
		}
		if each.wrap == nil {
			if !errors.Is(err, io.EOF) {
				t.Errorf("%s: %v", each.why, err)
			}
		} else if !errors.As(err, each.wrap) {
			t.Errorf("%s: %v", each.why, err)
		}
	}
}

// landOf is the one product New holds.
func landOf(t *testing.T) *celestialdata.Product {
	t.Helper()
	archive, err := naturalearth.New()
	if err != nil {
		t.Fatal(err)
	}
	found, err := archive.Find(replay.Asked(t))
	if err != nil || len(found) != lone {
		t.Fatalf("%d products, %v", len(found), err)
	}
	return found[first]
}

// The land is one product: Natural Earth's 1:50m land file, with the
// length the numbers file gives. A fetch that brings another length is
// refused, so the length is tested by what Fetch says of it.
func TestLandIsTheFileAtNaturalEarth(t *testing.T) {
	land := landOf(t)
	if land.Archive() != adapter || land.ID() != "50m land" ||
		land.Name() != "ne_50m_land.geojson" ||
		!strings.HasPrefix(land.URL(), "https://raw.githubusercontent.com/"+
			"nvkelso/natural-earth-vector/") ||
		!strings.HasSuffix(land.URL(), "/"+land.Name()) ||
		!strings.Contains(land.About(), "public domain") {
		t.Errorf("%s %s %s %s %q", land.Archive(), land.ID(), land.Name(),
			land.URL(), land.About())
	}
	const body = "short"
	get := mocks.NewMockWeb(gomock.NewController(t))
	reply := io.NopCloser(strings.NewReader(body))
	promised := int64(len(body))
	get.EXPECT().Get(land.URL()).Return(reply, promised, nil)
	_, err := celestialdata.Fetch(get, land, t.TempDir())
	want := strconv.Itoa(len(body)) + " bytes came, the archive said " +
		strconv.Itoa(landBytes)
	if err == nil || !strings.Contains(err.Error(), want) {
		t.Errorf("fetch of a short body: %v", err)
	}
}

// New is a fixed archive of the land, asked only by name, and it finds
// the same for every circle.
func TestNewHoldsTheLandForEveryCircle(t *testing.T) {
	archive, err := naturalearth.New()
	if err != nil {
		t.Fatal(err)
	}
	if archive.Name() != adapter || !archive.ByNameOnly() {
		t.Errorf("%q, by name only %v", archive.Name(), archive.ByNameOnly())
	}
	var _ celestialdata.ByName = archive
	land := landOf(t)
	pole, err := celestialdata.NewPosition(east, north)
	if err != nil {
		t.Fatal(err)
	}
	far, err := celestialdata.NewNear(pole, wide)
	if err != nil {
		t.Fatal(err)
	}
	for _, circle := range []*celestialdata.Near{far, replay.Asked(t)} {
		found, err := archive.Find(circle)
		if err != nil || len(found) != lone || *found[first] != *land {
			t.Errorf("%v, %v", found, err)
		}
	}
}

// What a test asks of New.
const (
	adapter   = "naturalearth" // defined: the archive's name
	landBytes = 1636166        // defined: sources.yaml, fetched 2026-11-02
	first     = 0              // defined: the first product
	lone      = 1              // defined: one product
	north     = 60             // defined: a far declination
	east      = 10             // defined: a far right ascension
	wide      = 0.1            // defined: a radius, in degrees
)

// Every number the code asks the numbers file for is in it: Value panics
// on one it lacks, so New running through is the check.
func TestEveryNumberTheCodeAsksForIsInTheFile(t *testing.T) {
	if _, err := naturalearth.New(); err != nil {
		t.Errorf("New: %v", err)
	}
}
