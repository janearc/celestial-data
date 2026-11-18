package main

import (
	"bytes"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/janearc/celestial-data/tnm"
)

func TestTerrainIndexDryRunNeedsDestination(t *testing.T) {
	for _, row := range []struct {
		args []string
		ok   bool
	}{
		{[]string{"terrain-index", "--into", t.TempDir(), "--dry-run"}, true},
		{[]string{"terrain-index", "--dry-run"}, false},
	} {
		var out, errs bytes.Buffer
		code := (command{}).run(row.args, bytes.NewReader(nil), &out, &errs)
		if (code == 0) != row.ok {
			t.Fatalf("code %d: %s / %s", code, out.String(), errs.String())
		}
	}
}

// Both terrain verbs keep a host's stop, and the override asks exactly
// once more. The mock permits no requests beyond those promised here.
func TestTerrainRequestsKeepHostStopsAndHonorTheOverride(t *testing.T) {
	for _, verb := range []string{"terrain-index", "terrain-fetch"} {
		for _, insist := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/override=%t", verb, insist), func(t *testing.T) {
				tool := askingHosts(t)
				args := []string{verb, "--into", t.TempDir()}
				tries := 1 // the request itself, without an override
				if insist {
					args = append(args, "--i-mean-it")
					tries++
				}
				stop := refusal(t, http.StatusTooManyRequests)
				if verb == "terrain-index" {
					product, err := tnm.HawaiiIndex()
					if err != nil {
						t.Fatal(err)
					}
					tool.get.EXPECT().Get(product.URL()).Return(nil,
						int64(0), stop).Times(tries)
				} else {
					index := filepath.Join(t.TempDir(), "catalogue.json")
					if err := os.WriteFile(index, []byte(exampleCatalogue), 0600); err != nil {
						t.Fatal(err)
					}
					args = append(args, "--index", index)
					tool.get.EXPECT().Head("https://prd-tnm.s3.amazonaws.com/new.tif").
						Return(int64(0), stop).Times(tries)
				}
				_, errs, code := ran(tool.command, args...)
				want := "to ask anyway, add --i-mean-it"
				if insist {
					want = "the override was tried"
				}
				if code != failed || !strings.Contains(errs, want) {
					t.Fatalf("exit %d, errors %q", code, errs)
				}
			})
		}
	}
}

// Dry runs accept the override but still ask nothing and make no output
// folder. They need neither a live server nor a writable host record.
func TestTerrainDryRunsWithTheOverrideAskNothing(t *testing.T) {
	for _, verb := range []string{"terrain-index", "terrain-fetch"} {
		t.Run(verb, func(t *testing.T) {
			tool := askingHosts(t)
			into := filepath.Join(t.TempDir(), "absent")
			args := []string{verb, "--into", into, "--dry-run", "--i-mean-it"}
			if verb == "terrain-fetch" {
				index := filepath.Join(t.TempDir(), "catalogue.json")
				if err := os.WriteFile(index, []byte(exampleCatalogue), 0600); err != nil {
					t.Fatal(err)
				}
				args = append(args, "--index", index)
			}
			_, errs, code := ran(tool.command, args...)
			if code != exitOK {
				t.Fatalf("exit %d, errors %q", code, errs)
			}
			if _, err := os.Stat(into); !os.IsNotExist(err) {
				t.Fatalf("dry run made its output folder: %v", err)
			}
		})
	}
}

func Example_terrainIndex() {
	(command{}).run([]string{"terrain-index", "--into", "terrain", "--dry-run"},
		bytes.NewReader(nil), os.Stdout, os.Stderr)
	// Output:
	// USGS Hawaii elevation index: no download
}

// exampleCatalogue is a retained National Map catalogue of one tile in two
// releases, small enough to read here. The newer release is the one a
// fetch brings, with its metadata.
const exampleCatalogue = `{"total":2,"items":[
{"title":"USGS 1/3 Arc Second n20w156 20230522","sourceId":"old",
"sizeInBytes":123,"downloadURL":"https://prd-tnm.s3.amazonaws.com/old.tif",
"vendorMetaUrl":"https://thor-f5.er.usgs.gov/old.xml"},
{"title":"USGS 1/3 Arc Second n20w156 20250611","sourceId":"new",
"sizeInBytes":456,"downloadURL":"https://prd-tnm.s3.amazonaws.com/new.tif",
"vendorMetaUrl":"https://thor-f5.er.usgs.gov/new.xml"}]}`

// A dry terrain fetch names each file it would bring and where from. It
// reads the retained catalogue only, so the example asks nothing of the
// network and writes nothing into the folder --into names.
func Example_terrainFetch() {
	folder, err := os.MkdirTemp("", "terrain-fetch-example")
	if err != nil {
		fmt.Println(err)
		return
	}
	defer os.RemoveAll(folder)
	index := filepath.Join(folder, "catalogue.json")
	if err := os.WriteFile(index, []byte(exampleCatalogue), 0600); err != nil {
		fmt.Println(err)
		return
	}
	(command{}).run([]string{"terrain-fetch", "--index", index, "--into",
		"terrain", "--dry-run"}, bytes.NewReader(nil), os.Stdout, os.Stderr)
	// Output:
	// new.tif https://prd-tnm.s3.amazonaws.com/new.tif
	// new.xml https://thor-f5.er.usgs.gov/new.xml
}
