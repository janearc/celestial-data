package earth

import (
	"net/url"
	"path"
	"strconv"

	"github.com/janearc/celestial-data"
)

// tnmProducts is the National Map's downloadable products search.
const tnmProducts = "https://tnmaccess.nationalmap.gov/api/v1/products"

// findUSGS lists downloadable point clouds from the National Map.
func findUSGS(base *celestialdata.Adapter,
	query Query) ([]*celestialdata.Product, error) {
	// TNM drops the hits without links after applying its own limit.
	// Read one larger page and cap the downloadable products locally.
	where := url.Values{
		"bbox":         {numbers(",", query.Bounds[:]...)},
		"datasets":     {"Lidar Point Cloud (LPC)"},
		"max":          {strconv.Itoa(maxMost)},
		"outputFormat": {"JSON"},
	}
	var catalog struct {
		Errors []string `json:"errors"`
		Items  *[]struct {
			ID    string `json:"sourceId"`
			Title string `json:"title"`
			URL   string `json:"downloadURL"`
			Bytes int64  `json:"sizeInBytes"`
		} `json:"items"`
	}
	address := tnmProducts + "?" + where.Encode()
	if err := readJSON(base, address, &catalog); err != nil {
		return nil, err
	}
	if len(catalog.Errors) > 0 {
		return nil, base.Fail("National Map: %s", catalog.Errors[0])
	}
	if catalog.Items == nil {
		return nil, base.Fail("National Map response has no items")
	}
	var found []*celestialdata.Product
	for _, item := range *catalog.Items {
		address, err := httpsAddress(item.URL)
		if err != nil || item.ID == "" {
			continue
		}
		file := path.Base(address.Path)
		product, err := base.Product(item.ID, file, item.URL,
			item.Bytes, item.Title)
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
