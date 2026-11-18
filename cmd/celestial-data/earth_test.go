package main

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/janearc/celestial-data"
	"github.com/janearc/celestial-data/mocks"
	"go.uber.org/mock/gomock"
)

// openskyExample is the bounded state-vector snapshot the example asks.
const openskyExample = "https://opensky-network.org/api/states/all?" +
	"lamax=38&lamin=37&lomax=-122&lomin=-123"

func Example_earthSources() {
	out := said("earth-sources")
	fmt.Println(strings.Contains(out, "USGS 3DEP lidar point clouds"))
	fmt.Println(strings.Contains(out, "NOAA InPort (catalog reference)"))
	// Output:
	// true
	// true
}

func Example_earthFind() {
	fmt.Print(said("earth-find", "opensky", "--bounds", "-123,37,-122,38"))
	// Output:
	// opensky states states_-123_37_-122_38.json
	//     OpenSky current state vectors in the bounding box
	//     https://opensky-network.org/api/states/all?lamax=38&lamin=37&lomax=-122&lomin=-123
}

func Example_earthFetch() {
	fmt.Print(said("earth-fetch", "opensky", "--bounds", "-123,37,-122,38",
		"--into", "earth-data", "--dry-run"))
	// Output:
	// would fetch https://opensky-network.org/api/states/all?lamax=38&lamin=37&lomax=-122&lomin=-123
	//     into earth-data/opensky/states_-123_37_-122_38.json
}

func TestEarthFetchKeepsTheProviderResponse(t *testing.T) {
	get := mocks.NewMockWeb(gomock.NewController(t))
	reply := `{"time":1,"states":[]}`
	get.EXPECT().Get(openskyExample).Return(
		io.NopCloser(strings.NewReader(reply)), int64(len(reply)), nil)
	folder := t.TempDir()
	got := saidThrough(get, "earth-fetch", "opensky", "--bounds",
		"-123,37,-122,38", "--into", folder)
	file := filepath.Join(folder, "opensky", "states_-123_37_-122_38.json")
	saved, err := os.ReadFile(file)
	if err != nil || string(saved) != reply || !strings.Contains(got, file) {
		t.Fatalf("saved %q (%v), CLI said %q", saved, err, got)
	}
}

func TestEarthCommandRefusesBadInput(t *testing.T) {
	testCases := []struct {
		args []string
		why  string
	}{
		{[]string{"earth-fetch", "opensky"}, "needs --into"},
		{[]string{"earth-fetch", "opensky", "--into", "unused"}, "bounds"},
		{[]string{"earth-find", "nws", "--point", "1,2", "--most", "2"},
			"does not take a result limit"},
		{[]string{"earth-find"}, "name one source"},
		{[]string{"earth-find", "asf", "--bounds", "1,2"}, "expected 4"},
		{[]string{"earth-find", "nws", "--point", "west,3"},
			"invalid coordinate"},
		{[]string{"earth-find", "usgs-3dep", "--most", "0"},
			"needs a positive integer"},
		{[]string{"earth-find", "overture"}, "catalog reference only"},
	}
	for _, tc := range testCases {
		t.Run(tc.why, func(t *testing.T) {
			get := mocks.NewMockWeb(gomock.NewController(t))
			var out, errs bytes.Buffer
			tool := command{get: get}
			if code := tool.run(tc.args, strings.NewReader(""), &out,
				&errs); code == exitOK || !strings.Contains(errs.String(), tc.why) {
				t.Fatalf("want %q refusal, exit %d, said %q (%q)",
					tc.why, code, errs.String(), out.String())
			}
		})
	}
}

func TestEarthWebUsesRegularClient(t *testing.T) {
	tool := command{picked: celestialdata.NewMeasured(numbersFile, numbersRaw)}
	if get, err := tool.earthWeb(); err != nil || get == nil {
		t.Fatalf("want a web client, got %v (%v)", get, err)
	}

	tool.picked = celestialdata.NewMeasured("empty.yaml", []byte("{}"))
	if _, err := tool.earthWeb(); err == nil {
		t.Fatal("wanted refusal of missing wait")
	}
}
