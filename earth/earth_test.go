package earth

import (
	"errors"
	"io"
	"net/url"
	"strings"
	"testing"

	"github.com/janearc/celestial-data"
	"github.com/janearc/celestial-data/mocks"
	"go.uber.org/mock/gomock"
)

var (
	box   = [4]float64{-123, 37, -122, 38}
	point = [2]float64{37.8, -122.4}
)

func answer(get *mocks.MockWeb, address, reply string) {
	get.EXPECT().Get(address).Return(io.NopCloser(strings.NewReader(reply)),
		int64(len(reply)), nil)
}

func TestCatalog(t *testing.T) {
	seen := make(map[string]bool)
	adapted := 0
	for _, source := range Sources() {
		where, err := url.Parse(source.Reference)
		if err != nil || where.Host == "" || seen[source.Name] {
			t.Fatalf("invalid or duplicate source: %+v", source)
		}
		seen[source.Name] = true
		if source.Queries != "" {
			adapted++
		}
	}
	if len(seen) != 64 || adapted != 6 || seen["nasa"] {
		t.Fatalf("found %d entries and %d adapters, Black Marble=%t",
			len(seen), adapted, seen["nasa"])
	}
}

func TestFindRefusalsAskNothing(t *testing.T) {
	badBox := [4]float64{170, -10, -170, 10}
	badPoint := [2]float64{91, -122}
	tests := []struct {
		name, why string
		query     Query
	}{
		{"wrong", "unknown", Query{}},
		{"data_gov", "catalog reference only", Query{}},
		{"asf", "bounds", Query{}},
		{"opensky", "antimeridian", Query{Bounds: &badBox}},
		{"nws", "latitude,longitude", Query{Point: &badPoint}},
		{"nws", "latitude,longitude", Query{Point: &point, Bounds: &box}},
		{"nasa-gibs", "between", Query{Bounds: &box, Most: 51}},
		{"opensky", "limit", Query{Bounds: &box, Most: 2}},
		{"usgs-3dep", "text filter", Query{Bounds: &box, Text: "lidar"}},
	}
	for _, tt := range tests {
		t.Run(tt.name+"/"+tt.why, func(t *testing.T) {
			get := mocks.NewMockWeb(gomock.NewController(t))
			_, err := Find(tt.name, get, tt.query)
			if !errors.Is(err, celestialdata.Refused()) ||
				!strings.Contains(err.Error(), tt.why) {
				t.Fatalf("want %q refusal, got %v", tt.why, err)
			}
		})
	}
	if _, err := Find("asf", nil, Query{Bounds: &box}); err == nil {
		t.Fatal("wanted missing web refusal")
	}
}

func TestUSGSLidar(t *testing.T) {
	get := mocks.NewMockWeb(gomock.NewController(t))
	answer(get, tnmTestAddress,
		`{"items":[{"sourceId":"empty","downloadURL":""},
		{"sourceId":"tile1","title":"Lidar tile","sizeInBytes":567,
		"downloadURL":"https://prd-tnm.s3.amazonaws.com/tile.laz?k=v"},
		{"sourceId":"tile2",
		"downloadURL":"https://prd-tnm.s3.amazonaws.com/another.laz"}]}`)
	found, err := Find("usgs-3dep", get, Query{Bounds: &box, Most: 1})
	if err != nil || len(found) != 1 {
		t.Fatalf("found %d products: %v", len(found), err)
	}
	if p := found[0]; p.ID() != "tile1" || p.Name() != "tile.laz" ||
		p.About() != "Lidar tile" || p.Archive() != "usgs-3dep" {
		t.Fatalf("wrong product: %+v", p)
	}
}

const asfFixture = `{"type":"FeatureCollection","features":[
	{"properties":{"fileID":"no-preview","url":"https://example.org/raw"}},
	{"properties":{"fileID":"scene","sceneName":"Hayward",
	"browse":["http://example.org/old.jpeg",
	"https://datapool.asf.alaska.edu/BROWSE/UA/first.jpg"]}},
	{"properties":{"fileID":"another",
	"browse":["https://datapool.asf.alaska.edu/BROWSE/UA/first.jpg"]}}]}`

func asfAddress(most string) string {
	return asfSearch + "?intersectsWith=POLYGON%28%28-123+37%2C-122+37%2C" +
		"-122+38%2C-123+38%2C-123+37%29%29&maxResults=" + most +
		"&output=geojson&platform=UAVSAR&processingLevel=INTERFEROMETRY"
}

func TestASFFetchesPublicBrowseProducts(t *testing.T) {
	for _, tt := range []struct {
		most int
		arg  string
	}{{0, "10"}, {1, "1"}} {
		get := mocks.NewMockWeb(gomock.NewController(t))
		answer(get, asfAddress(tt.arg), asfFixture)
		found, err := Find("asf", get, Query{Bounds: &box, Most: tt.most})
		if err != nil || len(found) != 1 || found[0].Name() != "first.jpg" {
			t.Fatalf("want one public browse image, got %v (%v)", found, err)
		}
	}
}

func TestGIBSFindsBlackMarbleBeyondLimitAndUsesItsVintage(t *testing.T) {
	get := mocks.NewMockWeb(gomock.NewController(t))
	answer(get, gibsCatalog, `<Capabilities><Contents>
		<Layer><Identifier>AAA</Identifier><Title>Unrelated</Title></Layer>
		<Layer><Identifier>VIIRS_Black_Marble</Identifier>
		  <Title>Black Marble (Annual)</Title>
		  <Dimension><Identifier>Time</Identifier>
		    <Default>2016-01-01</Default>
		    <Value>2012-01-01/2012-01-01/P1Y</Value>
		    <Value>2016-01-01/2016-01-01/P1Y</Value>
		  </Dimension>
		</Layer>
	</Contents></Capabilities>`)
	query := Query{Bounds: &box, Text: " BLACK MARBLE ", Most: 1}
	found, err := Find("nasa-gibs", get, query)
	if err != nil || len(found) != 1 {
		t.Fatalf("found %d products: %v", len(found), err)
	}
	p := found[0]
	want := snapshots + "?BBOX=37%2C-123%2C38%2C-122&CRS=EPSG%3A4326" +
		"&FORMAT=image%2Fjpeg&HEIGHT=2048&LAYERS=VIIRS_Black_Marble" +
		"&REQUEST=GetSnapshot&TIME=2016-01-01&WIDTH=2048"
	file := "VIIRS_Black_Marble_2016-01-01_-123_37_-122_38.jpg"
	if p.URL() != want || p.Name() != file {
		t.Fatalf("wrong snapshot: %s %s", p.Name(), p.URL())
	}
}

func TestGIBSSortsAndAcceptsUndatedLayers(t *testing.T) {
	get := mocks.NewMockWeb(gomock.NewController(t))
	answer(get, gibsCatalog, `<Capabilities><Contents>
		<Layer><Identifier>BBB</Identifier></Layer>
		<Layer><Title>No identifier</Title></Layer>
		<Layer><Identifier>AAA</Identifier>
		  <Dimension><Identifier>Time</Identifier><Default>bad</Default></Dimension>
		</Layer>
	</Contents></Capabilities>`)
	found, err := Find("nasa-gibs", get, Query{Bounds: &box})
	if err != nil || len(found) != 2 {
		t.Fatalf("wanted two products: %v (%v)", found, err)
	}
	if found[0].ID() != "AAA" || strings.Contains(found[0].URL(), "TIME=") ||
		found[1].ID() != "BBB" {
		t.Fatalf("wanted undated layers in order: %v", found)
	}
}

func TestLiveProductAddresses(t *testing.T) {
	nwsStations := weatherAPI + "/gridpoints/MTR/85,105/stations"
	tests := []struct {
		name, want string
		query      Query
	}{
		{"opensky", openSkyStates + "?lamax=38&lamin=37&lomax=-122&lomin=-123",
			Query{Bounds: &box}},
		{"open-meteo", openMeteo + "?current=temperature_2m%2Cwind_speed_10m" +
			"%2Cwind_direction_10m%2Cprecipitation%2Ccloud_cover%2Cpressure_msl" +
			"%2Crelative_humidity_2m&latitude=37.8&longitude=-122.4&timezone=UTC",
			Query{Point: &point}},
		{"nws", weatherAPI + "/stations/KSFO/observations/latest", Query{Point: &point}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			get := mocks.NewMockWeb(gomock.NewController(t))
			if tt.name == "nws" {
				answer(get, weatherAPI+"/points/37.8,-122.4",
					`{"properties":{"observationStations":"`+nwsStations+`"}}`)
				answer(get, nwsStations, `{"features":[{"properties":{}},
					{"properties":{"stationIdentifier":"KSFO"}}]}`)
			}
			found, err := Find(tt.name, get, tt.query)
			if err != nil || len(found) != 1 || found[0].URL() != tt.want {
				t.Fatalf("want %s, got %v (%v)", tt.want, found, err)
			}
		})
	}
}

// tnmTestAddress is the full National Map page requested for box.
const tnmTestAddress = tnmProducts + "?bbox=-123%2C37%2C-122%2C38" +
	"&datasets=Lidar+Point+Cloud+%28LPC%29&max=50&outputFormat=JSON"

func TestFailedCatalogGets(t *testing.T) {
	broken := errors.New("offline")
	tests := []struct {
		name, address string
		query         Query
	}{
		{"usgs-3dep", tnmTestAddress,
			Query{Bounds: &box}},
		{"asf", asfAddress("10"), Query{Bounds: &box}},
		{"nasa-gibs", gibsCatalog, Query{Bounds: &box}},
		{"nws", weatherAPI + "/points/37.8,-122.4", Query{Point: &point}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			get := mocks.NewMockWeb(gomock.NewController(t))
			get.EXPECT().Get(tt.address).Return(nil, int64(0), broken)
			if _, err := Find(tt.name, get, tt.query); !errors.Is(err, broken) {
				t.Fatalf("want provider error, got %v", err)
			}
		})
	}
}

func TestBrokenCatalogBodies(t *testing.T) {
	tests := []struct{ name, address, reply, why string }{
		{"usgs-3dep", tnmTestAddress,
			`{}`, "no items"},
		{"usgs-3dep", tnmTestAddress, `{"errors":["catalog error"]}`, "catalog error"},
		{"usgs-3dep", tnmTestAddress, `{"items":[{"sourceId":"bad",
			"sizeInBytes":-1,"downloadURL":"https://example.org/a.laz"}]}`, "bytes"},
		{"asf", asfAddress("10"), `{}`, "no features"},
		{"asf", asfAddress("10"), "bad JSON", "read"},
		{"nasa-gibs", gibsCatalog, `<Capabilities/>`, "no layers"},
		{"nasa-gibs", gibsCatalog, "<bad", "read"},
	}
	for _, tt := range tests {
		t.Run(tt.name+"/"+tt.why, func(t *testing.T) {
			get := mocks.NewMockWeb(gomock.NewController(t))
			answer(get, tt.address, tt.reply)
			_, err := Find(tt.name, get, Query{Bounds: &box})
			if err == nil || !strings.Contains(err.Error(), tt.why) {
				t.Fatalf("want %s, got %v", tt.why, err)
			}
		})
	}
}

func TestNWSMissingOrUnusableStations(t *testing.T) {
	for _, link := range []string{"https://example.org/stations", weatherAPI + "/stations"} {
		get := mocks.NewMockWeb(gomock.NewController(t))
		answer(get, weatherAPI+"/points/37.8,-122.4",
			`{"properties":{"observationStations":"`+link+`"}}`)
		if strings.HasPrefix(link, weatherAPI) {
			answer(get, link, `{"features":[{"properties":{"stationIdentifier":"../bad"}}]}`)
		}
		products, err := Find("nws", get, Query{Point: &point})
		if err == nil || len(products) != 0 {
			t.Fatalf("wanted station failure, got %v (%v)", products, err)
		}
	}
}
