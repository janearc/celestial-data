package earth

import (
	"fmt"
	"net/url"
	"path"
	"strconv"

	"github.com/janearc/celestial-data"
)

// asfSearch is the public ASF catalog. We offer public browse images.
const asfSearch = "https://api.daac.asf.alaska.edu/services/search/param"

// findASF lists public browse images for radar interferograms.
func findASF(base *celestialdata.Adapter,
	query Query) ([]*celestialdata.Product, error) {
	where := url.Values{
		"intersectsWith":  {polygon(*query.Bounds)},
		"maxResults":      {strconv.Itoa(query.Most)},
		"output":          {"geojson"},
		"platform":        {"UAVSAR"},
		"processingLevel": {"INTERFEROMETRY"},
	}
	var catalog struct {
		Features *[]struct {
			Properties struct {
				ID     string   `json:"fileID"`
				Scene  string   `json:"sceneName"`
				Browse []string `json:"browse"`
			} `json:"properties"`
		} `json:"features"`
	}
	search := asfSearch + "?" + where.Encode()
	if err := readJSON(base, search, &catalog); err != nil {
		return nil, err
	}
	if catalog.Features == nil {
		return nil, base.Fail("ASF response has no features array")
	}
	var found []*celestialdata.Product
	seen := make(map[string]bool)
	for _, feature := range *catalog.Features {
		meta := feature.Properties
		for _, link := range meta.Browse {
			address, err := httpsAddress(link)
			if err != nil || meta.ID == "" || seen[link] {
				continue
			}
			file := path.Base(address.Path)
			about := "UAVSAR interferogram " + meta.Scene
			product, err := base.Product(meta.ID, file, link,
				celestialdata.Unsized, about)
			if err != nil {
				return nil, base.Wrap(err)
			}
			seen[link] = true
			found = append(found, product)
			if len(found) == query.Most {
				return found, nil
			}
		}
	}
	return found, nil
}

// polygon closes the search box as a WKT polygon in longitude, latitude.
func polygon(bounds [4]float64) string {
	west, south, east, north := bounds[0], bounds[1], bounds[2], bounds[3]
	return fmt.Sprintf("POLYGON((%s,%s,%s,%s,%s))",
		numbers(" ", west, south), numbers(" ", east, south),
		numbers(" ", east, north), numbers(" ", west, north),
		numbers(" ", west, south))
}
