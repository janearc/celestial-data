package earth

import (
	"encoding/xml"
	"net/url"
	"slices"
	"strings"
	"time"

	"github.com/janearc/celestial-data"
)

const (
	gibsCatalog = "https://gibs.earthdata.nasa.gov/wmts/epsg4326/best/" +
		"wmts.cgi?SERVICE=WMTS&REQUEST=GetCapabilities"
	snapshots      = "https://wvs.earthdata.nasa.gov/api/v1/snapshot"
	snapshotPixels = "2048"
)

// layer is a WMTS imagery layer and its dimensions.
type layer struct {
	ID         string `xml:"Identifier"`
	Title      string `xml:"Title"`
	Dimensions []struct {
		Name    string `xml:"Identifier"`
		Default string `xml:"Default"`
	} `xml:"Dimension"`
}

// findGIBS reads the geographic imagery catalog NASA publishes for clients.
func findGIBS(base *celestialdata.Adapter,
	query Query) ([]*celestialdata.Product, error) {
	body, _, err := base.Web().Get(gibsCatalog)
	if err != nil {
		return nil, base.Wrap(err)
	}
	defer body.Close()
	var catalog struct {
		XMLName xml.Name `xml:"Capabilities"`
		Layers  []layer  `xml:"Contents>Layer"`
	}
	if err := xml.NewDecoder(body).Decode(&catalog); err != nil {
		return nil, base.Fail("read GIBS capabilities: %w", err)
	}
	if len(catalog.Layers) == 0 {
		return nil, base.Fail("GIBS capabilities have no layers")
	}
	return gibsProducts(base, query, catalog.Layers)
}

// gibsProducts filters layers by id or title before limiting the result.
func gibsProducts(base *celestialdata.Adapter, query Query,
	layers []layer) ([]*celestialdata.Product, error) {
	filter := strings.ToLower(strings.TrimSpace(query.Text))
	slices.SortFunc(layers, func(a, b layer) int {
		return strings.Compare(a.ID, b.ID)
	})
	var found []*celestialdata.Product
	for _, meta := range layers {
		if !meta.matches(filter) {
			continue
		}
		product, err := snapshot(base, meta, *query.Bounds)
		if err != nil {
			return nil, base.Wrap(err)
		}
		found = append(found, product)
		if len(found) == query.Most {
			break
		}
	}
	return found, nil
}

// matches checks whether a layer's name or title contains the filter.
func (meta layer) matches(filter string) bool {
	idMatch := strings.Contains(strings.ToLower(meta.ID), filter)
	titleMatch := strings.Contains(strings.ToLower(meta.Title), filter)
	return meta.ID != "" && (idMatch || titleMatch)
}

// vintage takes the layer's declared default date. For annual Black Marble
// this is a publication date, not yesterday or the end of a date range.
func (meta layer) vintage() string {
	for _, dimension := range meta.Dimensions {
		if strings.EqualFold(dimension.Name, "time") {
			day, _, _ := strings.Cut(dimension.Default, "T")
			_, err := time.Parse(time.DateOnly, day)
			if err == nil {
				return day
			}
		}
	}
	return ""
}

// snapshot makes a Worldview image product for one layer and box.
func snapshot(base *celestialdata.Adapter, meta layer,
	bounds [4]float64) (*celestialdata.Product, error) {
	west, south, east, north := bounds[0], bounds[1], bounds[2], bounds[3]
	// WVS uses latitude first for EPSG:4326.
	where := url.Values{
		"BBOX":    {numbers(",", south, west, north, east)},
		"CRS":     {"EPSG:4326"},
		"FORMAT":  {"image/jpeg"},
		"HEIGHT":  {snapshotPixels},
		"LAYERS":  {meta.ID},
		"REQUEST": {"GetSnapshot"},
		"WIDTH":   {snapshotPixels},
	}
	name := meta.ID
	if day := meta.vintage(); day != "" {
		where.Set("TIME", day)
		name += "_" + day
	}
	name += "_" + numbers("_", bounds[:]...) + ".jpg"
	return base.Product(meta.ID, name, snapshots+"?"+where.Encode(),
		celestialdata.Unsized, meta.Title)
}
