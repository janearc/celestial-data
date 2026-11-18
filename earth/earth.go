// Package earth finds files and snapshots by terrestrial coordinates.
package earth

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"math"
	"net/url"
	"strconv"
	"strings"

	"github.com/janearc/celestial-data"
	"github.com/janearc/celestial-data/web"
)

// Query locates products on Earth. Bounds is west, south, east, north.
// Point is latitude, longitude. Text filters GIBS layers by name or title.
// Most caps catalog results and defaults to 10.
type Query struct {
	Bounds *[4]float64
	Point  *[2]float64
	Text   string
	Most   int
}

// Source describes one catalog entry. Queries contains the inputs taken
// by its adapter, or is empty when the catalog only has a reference.
type Source struct {
	Name, Title, Reference, Queries string
}

// references is the Kingfisher source inventory, with the weather
// providers listed separately and stand-alone Black Marble in bluemarble.
//
//go:embed sources.tsv
var references string

// Sources lists the catalog without asking the network.
func Sources() []Source {
	var sources []Source
	for row := range strings.Lines(references) {
		fields := strings.Split(strings.TrimSuffix(row, "\n"), "\t")
		source := Source{Name: fields[0], Title: fields[1],
			Reference: fields[2],
			Queries:   providers[fields[0]].queries}
		sources = append(sources, source)
	}
	return sources
}

// finder names the products an adapter holds at an Earth location.
type finder func(*celestialdata.Adapter, Query) (
	[]*celestialdata.Product, error)

// providers holds the query requirements and entry point for each
// implemented source. Entries without an adapter live in sources.tsv.
var providers = map[string]struct {
	queries string
	find    finder
}{
	"asf":        {"bounds, most", findASF},
	"nasa-gibs":  {"bounds, most, text", findGIBS},
	"nws":        {"point", findNWS},
	"open-meteo": {"point", findOpenMeteo},
	"opensky":    {"bounds", findOpenSky},
	"usgs-3dep":  {"bounds, most", findUSGS},
}

// Find lists the products from one named source. It takes terrestrial
// coordinates in degrees and refuses boxes that cross the antimeridian.
func Find(name string, get web.Web,
	query Query) ([]*celestialdata.Product, error) {
	provider, ok := providers[name]
	if !ok {
		for _, source := range Sources() {
			if source.Name == name {
				return nil, celestialdata.Refuse(
					"%s has a catalog reference only (%s)",
					name, source.Reference)
			}
		}
		return nil, celestialdata.Refuse(
			"unknown Earth source %q", name)
	}
	base, err := celestialdata.NewAdapter(name, get)
	if err != nil {
		return nil, err
	}
	if err := validate(base, provider.queries, query); err != nil {
		return nil, err
	}
	if query.Most == 0 {
		query.Most = defaultMost
	}
	return provider.find(base, query)
}

const (
	defaultMost = 10
	maxMost     = 50
)

// validate refuses invalid coordinates and options a source cannot use.
func validate(base *celestialdata.Adapter, inputs string, query Query) error {
	if inputs == "point" {
		if query.Point == nil || query.Bounds != nil ||
			!validPoint(*query.Point) {
			return base.Refuse(
				"needs a valid latitude,longitude point")
		}
	} else if query.Bounds == nil || query.Point != nil ||
		!validBounds(*query.Bounds) {
		return base.Refuse("needs valid west,south,east,north " +
			"bounds that do not cross the antimeridian")
	}
	if query.Most < 0 || query.Most > maxMost {
		return base.Refuse("most must be between 1 and %d", maxMost)
	}
	if query.Most != 0 && !strings.Contains(inputs, "most") {
		return base.Refuse("does not take a result limit")
	}
	if query.Text != "" && !strings.Contains(inputs, "text") {
		return base.Refuse("does not take a text filter")
	}
	return nil
}

// validPoint checks latitude and longitude in decimal degrees.
func validPoint(point [2]float64) bool {
	return within(point[0], 90) && within(point[1], 180)
}

// validBounds checks both corners and requires a nonempty box.
func validBounds(bounds [4]float64) bool {
	west, south, east, north := bounds[0], bounds[1], bounds[2], bounds[3]
	return validPoint([2]float64{south, west}) &&
		validPoint([2]float64{north, east}) &&
		west < east && south < north
}

// within checks a signed angular coordinate against its allowed extent.
func within(value, edge float64) bool {
	return !math.IsNaN(value) && -edge <= value && value <= edge
}

// numbers gives coordinates as decimal degrees, without rounding.
func numbers(separator string, values ...float64) string {
	parts := make([]string, len(values))
	for i, value := range values {
		parts[i] = strconv.FormatFloat(value, 'g', -1, 64)
	}
	return strings.Join(parts, separator)
}

// readJSON decodes a provider response and closes its body.
func readJSON(base *celestialdata.Adapter, address string, out any) error {
	body, _, err := base.Web().Get(address)
	if err != nil {
		return base.Wrap(err)
	}
	defer body.Close()
	if err := json.NewDecoder(body).Decode(out); err != nil {
		return base.Fail("read %s: %w", address, err)
	}
	return nil
}

// httpsAddress accepts a provider's absolute HTTPS file link.
func httpsAddress(address string) (*url.URL, error) {
	where, err := url.Parse(address)
	if err != nil {
		return nil, err
	}
	if where.Scheme != "https" || where.Hostname() == "" ||
		where.User != nil {
		return nil, fmt.Errorf("expected an HTTPS address")
	}
	return where, nil
}
