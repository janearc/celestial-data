// Package naturalearth fetches and reads Natural Earth's land: the
// outlines of the land at a scale of 1:50 million, as GeoJSON.
//
// Natural Earth is in the public domain. Its makers ask for no credit
// and are glad of one: "Made with Natural Earth."
package naturalearth

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"io"
	"math"

	"github.com/janearc/celestial-data"
)

// adapterName is the archive's name, as the command line takes it.
const adapterName = "naturalearth"

// The land's file, as the project keeps it.
const (
	// landName is the file of the land at 1:50m.
	landName = "ne_50m_land.geojson"
	// address is where the project's repository serves it.
	address = "https://raw.githubusercontent.com/nvkelso/" +
		"natural-earth-vector/master/geojson/" + landName
	// landID is the id of the land's product.
	landID = "50m land"
	// landSize is the name of the file's length in numbers.yaml.
	landSize = "land-size"
)

// numbersFile is the numbers file that holds the land file's length.
const numbersFile = "numbers.yaml"

//go:embed numbers.yaml
var numbersRaw []byte

// New is the archive of the land, asked by name only. It holds the one
// file of the land's outlines, whatever the circle.
func New() (*celestialdata.Fixed, error) {
	size := int64(measured().Value(landSize))
	land := celestialdata.NewFile(landID, landName, address, size,
		"Natural Earth land at 1:50m, public domain")
	return celestialdata.NewFixed(adapterName, land)
}

// measured is the numbers file, read.
func measured() *celestialdata.Measured {
	return celestialdata.NewMeasured(numbersFile, numbersRaw)
}

// refuse is a refusal that begins with the archive's name.
func refuse(format string, args ...any) error {
	return celestialdata.Refuse(adapterName+" "+format, args...)
}

// The places in what GeoJSON writes. They are defined by GeoJSON: a
// position is longitude then latitude, and a polygon's first ring is its
// outline.
const (
	// pair is the parts of a position. It's defined.
	pair = 2
	// lonAt is the place of the longitude in a position. It's defined.
	lonAt = 0
	// latAt is the place of the latitude in a position. It's defined.
	latAt = 1
	// outlineAt is the place of the outline among a polygon's rings. It's
	// defined.
	outlineAt = 0
)

// Polygon is one piece of land: its outline first, then any holes in it.
// Each ring is longitude and latitude pairs in degrees.
//
//ninja:vow practice a library: the viewer in slayer draws the pieces
type Polygon [][][pair]float64

// Map is the land, as polygons, each with the box its outline lies in.
//
//ninja:vow practice a library: the viewer in slayer draws the land
type Map struct {
	polygons []Polygon
	boxes    []box
}

// box is the longitudes and latitudes a ring lies within.
type box struct{ west, east, south, north float64 }

// The signs math.Inf takes.
const (
	upward   = 1  // defined: positive infinity
	downward = -1 // defined: negative infinity
)

// boxOf is the box a ring lies within.
func boxOf(ring [][pair]float64) box {
	bounds := box{west: math.Inf(upward), east: math.Inf(downward),
		south: math.Inf(upward), north: math.Inf(downward)}
	for _, at := range ring {
		bounds.west = math.Min(bounds.west, at[lonAt])
		bounds.east = math.Max(bounds.east, at[lonAt])
		bounds.south = math.Min(bounds.south, at[latAt])
		bounds.north = math.Max(bounds.north, at[latAt])
	}
	return bounds
}

// outside is whether a place lies outside a box. A piece holds its west
// and south edges and not its east and north; on the east edge the ray
// test's crossing can round a hair east of the corner, so the box keeps
// the edge off.
func (bounds box) outside(lon, lat float64) bool {
	return lon < bounds.west || lon >= bounds.east ||
		lat < bounds.south || lat > bounds.north
}

// geometry is a GeoJSON feature's shape, as read.
type geometry struct {
	Type        string          `json:"type"`
	Coordinates json.RawMessage `json:"coordinates"`
}

// collection is a GeoJSON feature collection, as read.
type collection struct {
	Features []struct {
		Geometry geometry `json:"geometry"`
	} `json:"features"`
}

// Read is the land in a Natural Earth GeoJSON file. It reads polygons and
// multipolygons, and refuses any other shape and a polygon with no
// outline.
//
//ninja:vow practice a library: the viewer in slayer calls it
func Read(file io.Reader) (*Map, error) {
	var read collection
	if err := json.NewDecoder(file).Decode(&read); err != nil {
		return nil, fmt.Errorf("%s: %w", adapterName, err)
	}
	land := &Map{}
	for _, feature := range read.Features {
		pieces, err := piecesOf(feature.Geometry)
		if err != nil {
			return nil, err
		}
		land.polygons = append(land.polygons, pieces...)
	}
	for _, piece := range land.polygons {
		if len(piece) == noRings {
			return nil, refuse("holds a polygon with no outline")
		}
		land.boxes = append(land.boxes, boxOf(piece[outlineAt]))
	}
	return land, nil
}

// noRings is a polygon with no outline. It's defined.
const noRings = 0

// piecesOf is the polygons of a shape: one for a polygon, and its
// polygons for a multipolygon. It refuses any other shape.
func piecesOf(shape geometry) ([]Polygon, error) {
	switch shape.Type {
	case "Polygon":
		var one Polygon
		if err := json.Unmarshal(shape.Coordinates, &one); err != nil {
			return nil, fmt.Errorf("%s: %w", adapterName, err)
		}
		return []Polygon{one}, nil
	case "MultiPolygon":
		var many []Polygon
		if err := json.Unmarshal(shape.Coordinates, &many); err != nil {
			return nil, fmt.Errorf("%s: %w", adapterName, err)
		}
		return many, nil
	}
	return nil, refuse("holds no land of the shape %q", shape.Type)
}

// Polygons are the land's pieces.
//
//ninja:vow practice a library: the viewer in slayer draws the pieces
func (land *Map) Polygons() []Polygon { return land.polygons }

// Holds is whether a place, by longitude and latitude in degrees, is on
// land: inside a piece's outline an odd number of times, counting its
// holes. Rings are taken as drawn on a plain longitude and latitude grid.
//
//ninja:vow practice a library: the viewer in slayer asks of each place
func (land *Map) Holds(lon, lat float64) bool {
	for i, piece := range land.polygons {
		if land.boxes[i].outside(lon, lat) {
			continue
		}
		inside := false
		for _, ring := range piece {
			if crosses(ring, lon, lat) {
				inside = !inside
			}
		}
		if inside {
			return true
		}
	}
	return false
}

// nextPoint is how far on a ring's next point is. It's defined.
const nextPoint = 1

// crosses is whether a ray east from a place crosses a ring an odd number
// of times.
func crosses(ring [][pair]float64, lon, lat float64) bool {
	odd := false
	for i := range ring {
		from, to := ring[i], ring[(i+nextPoint)%len(ring)]
		if (from[latAt] > lat) == (to[latAt] > lat) {
			continue
		}
		rise := (lat - from[latAt]) / (to[latAt] - from[latAt])
		at := from[lonAt] + rise*(to[lonAt]-from[lonAt])
		if lon < at {
			odd = !odd
		}
	}
	return odd
}
