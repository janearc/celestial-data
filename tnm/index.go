// Package tnm retains elevation product catalogues from the USGS National Map.
// Catalogue bytes are fetched and kept before their products are selected.
package tnm

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"net/url"
	"os"

	celestialdata "github.com/janearc/celestial-data"
	"github.com/janearc/puzzlemouth/yamlfile"
)

//go:embed hawaii.yaml
var hawaiiQuery []byte

// HawaiiIndex requests 1/3 arc-second elevation products intersecting an
// authored bounding rectangle around Hawaii island. These are search bounds,
// not surveyed coastline coordinates; archive metadata establishes coverage.
func HawaiiIndex() (*celestialdata.Product, error) {
	var picked map[string]string
	if err := yamlfile.Read("hawaii.yaml", hawaiiQuery,
		&picked); err != nil {
		return nil, err
	}
	query := url.Values{}
	for key, value := range picked {
		query.Set(key, value)
	}

	address := "https://tnmaccess.nationalmap.gov/api/v1/products?" +
		query.Encode()
	archive, err := celestialdata.NewFixed("usgs-tnm",
		celestialdata.NewFile("hawaii-elevation",
			"hawaii-elevation-index.json", address, 0,
			"USGS National Map elevation catalogue; authored "+
				"Hawaii search bounds"))
	if err != nil {
		return nil, err
	}
	products, err := archive.Find(nil)
	if err != nil {
		return nil, err
	}
	return products[0], nil
}

// indexCheck requires a complete JSON object, not an HTML service error.
type indexCheck struct{}

// CheckIndex validates a retained catalogue without discarding archive replies.
func CheckIndex(path string) error { return (indexCheck{}).Check(path) }

// Check validates a retained index as a complete JSON object with items.
func (indexCheck) Check(path string) error {
	raw, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	var value map[string]json.RawMessage
	if err := json.Unmarshal(raw, &value); err != nil {
		return err
	}
	if _, ok := value["items"]; !ok {
		return fmt.Errorf("tnm: catalogue has no items field")
	}
	return nil
}
