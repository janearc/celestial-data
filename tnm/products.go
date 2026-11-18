package tnm

import (
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"path"
	"regexp"
	"sort"
	"strings"

	celestialdata "github.com/janearc/celestial-data"
)

// productTitle distinguishes dated 1/3 arc-second products from other records
// returned by the service, including older records titled "13 arc-second".
var productTitle = regexp.MustCompile(
	"^USGS 1/3 Arc Second (n[0-9]+w[0-9]+) ([0-9]{8})$")

type item struct {
	Title    string
	SourceID string `json:"sourceId"`
	Download string `json:"downloadURL"`
	Metadata string `json:"vendorMetaUrl"`
	Bytes    int64  `json:"sizeInBytes"`
}

// Products selects the latest dated release of each tile in a retained index.
// The accompanying metadata is retained too, so datum and units can be read
// before the terrain is staged. No network query occurs during selection.
func Products(index string) ([]*celestialdata.Product, error) {
	raw, err := os.ReadFile(index)
	if err != nil {
		return nil, err
	}
	var catalogue struct {
		Total  int
		Items  []item
		Errors []json.RawMessage
	}
	if err := json.Unmarshal(raw, &catalogue); err != nil {
		return nil, err
	}
	if len(catalogue.Errors) != 0 ||
		catalogue.Total != len(catalogue.Items) {
		return nil, fmt.Errorf("tnm: failed or paginated catalogue")
	}
	chosen := map[string]item{}
	dates := map[string]string{}
	for _, row := range catalogue.Items {
		match := productTitle.FindStringSubmatch(row.Title)
		if match == nil {
			continue
		}
		if match[2] > dates[match[1]] {
			chosen[match[1]], dates[match[1]] = row, match[2]
		}
	}
	if len(chosen) == 0 {
		return nil, fmt.Errorf("tnm: no dated elevation tiles")
	}
	keys := make([]string, 0, len(chosen))
	for key := range chosen {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	var products []*celestialdata.Product
	for _, key := range keys {
		row := chosen[key]
		if row.Bytes <= 0 {
			return nil, fmt.Errorf("tnm: missing tile byte count")
		}
		terrain, err := safeFile(row.SourceID, row.Download,
			row.Bytes, true)
		if err != nil {
			return nil, err
		}
		metadata, err := safeFile(row.SourceID, row.Metadata, 0, false)
		if err != nil {
			return nil, err
		}
		archive, err := celestialdata.NewFixed("usgs-tnm", terrain,
			metadata)
		if err != nil {
			return nil, err
		}
		found, err := archive.Find(nil)
		if err != nil {
			return nil, err
		}
		products = append(products, found...)
	}
	return products, nil
}

// safeFile confines this catalogue adapter to the archive's published hosts.
func safeFile(id, address string, size int64,
	raster bool) (celestialdata.File, error) {
	u, err := url.Parse(address)
	if err != nil {
		return celestialdata.File{}, err
	}
	host, suffix := "thor-f5.er.usgs.gov", ".xml"
	if raster {
		host, suffix = "prd-tnm.s3.amazonaws.com", ".tif"
	}
	if u.Scheme != "https" || u.Host != host || u.User != nil ||
		!strings.HasSuffix(u.Path, suffix) || u.RawQuery != "" {
		return celestialdata.File{}, fmt.Errorf(
			"tnm: unexpected product URL %q", address)
	}
	return celestialdata.NewFile(id, path.Base(u.Path), address, size,
		"USGS National Map; retain metadata with terrain"), nil
}

// ServedProduct pins a transfer to the server's observed representation size.
// The original catalogue remains retained; a changed catalogue size must be
// recorded rather than silently accepted. The subsequent GET must match too.
func ServedProduct(product *celestialdata.Product,
	size int64) (*celestialdata.Product, error) {
	if product == nil || size <= 0 {
		return nil, fmt.Errorf("tnm: no observed transfer size")
	}
	archive, err := celestialdata.NewFixed(product.Archive(),
		celestialdata.NewFile(product.ID(), product.Name(),
			product.URL(), size, product.About()))
	if err != nil {
		return nil, err
	}
	found, err := archive.Find(nil)
	if err != nil {
		return nil, err
	}
	return found[0], nil
}
