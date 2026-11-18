package tnm

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestTerrainProductsSelectLatestReleaseAndRetainMetadata(t *testing.T) {
	raw := `{"total":2,"items":[
{"title":"USGS 1/3 Arc Second n20w156 20230522","sourceId":"old",
"sizeInBytes":123,"downloadURL":"https://prd-tnm.s3.amazonaws.com/old.tif",
"vendorMetaUrl":"https://thor-f5.er.usgs.gov/old.xml"},
{"title":"USGS 1/3 Arc Second n20w156 20250611","sourceId":"new",
"sizeInBytes":456,"downloadURL":"https://prd-tnm.s3.amazonaws.com/new.tif",
"vendorMetaUrl":"https://thor-f5.er.usgs.gov/new.xml"}]}`
	p := filepath.Join(t.TempDir(), "catalogue.json")
	if err := os.WriteFile(p, []byte(raw), 0600); err != nil {
		t.Fatal(err)
	}
	got, err := Products(p)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0].ID() != "new" || got[0].Name() != "new.tif" ||
		got[1].Name() != "new.xml" {
		t.Fatal("selection lost latest terrain or metadata", got)
	}
	if err := os.WriteFile(p, []byte(strings.ReplaceAll(raw, "prd-tnm.s3.amazonaws.com", "example.com")), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := Products(p); err == nil {
		t.Fatal("unexpected product host accepted")
	}
	if err := os.WriteFile(p, []byte(strings.Replace(raw, `"total":2`, `"total":3`, 1)), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := Products(p); err == nil {
		t.Fatal("partial catalogue accepted")
	}
}
