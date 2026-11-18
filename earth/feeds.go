package earth

import (
	"net/url"
	"strings"

	"github.com/janearc/celestial-data"
)

const (
	openSkyStates = "https://opensky-network.org/api/states/all"
	openMeteo     = "https://api.open-meteo.com/v1/forecast"
	weatherHost   = "api.weather.gov"
	weatherAPI    = "https://" + weatherHost
)

// findOpenSky offers one state-vector snapshot for the given box.
func findOpenSky(base *celestialdata.Adapter,
	query Query) ([]*celestialdata.Product, error) {
	west, south, east, north := query.Bounds[0], query.Bounds[1],
		query.Bounds[2], query.Bounds[3]
	where := url.Values{
		"lamin": {numbers("", south)}, "lamax": {numbers("", north)},
		"lomin": {numbers("", west)}, "lomax": {numbers("", east)},
	}
	file := "states_" + numbers("_", query.Bounds[:]...) + ".json"
	return celestialdata.Found(base.Product("states", file,
		openSkyStates+"?"+where.Encode(), celestialdata.Unsized,
		"OpenSky current state vectors in the bounding box"))
}

// findOpenMeteo offers one current-conditions snapshot at a point.
func findOpenMeteo(base *celestialdata.Adapter,
	query Query) ([]*celestialdata.Product, error) {
	const current = "temperature_2m,wind_speed_10m,wind_direction_10m," +
		"precipitation,cloud_cover,pressure_msl,relative_humidity_2m"
	where := url.Values{
		"latitude":  {numbers("", query.Point[0])},
		"longitude": {numbers("", query.Point[1])},
		"timezone":  {"UTC"},
		"current":   {current},
	}
	file := "current_" + numbers("_", query.Point[:]...) + ".json"
	return celestialdata.Found(base.Product("current", file,
		openMeteo+"?"+where.Encode(), celestialdata.Unsized,
		"Open-Meteo current conditions and units"))
}

// findNWS offers the nearest listed station's latest observation.
func findNWS(base *celestialdata.Adapter,
	query Query) ([]*celestialdata.Product, error) {
	var point struct {
		Properties struct{ ObservationStations string }
	}
	address := weatherAPI + "/points/" + numbers(",", query.Point[:]...)
	if err := readJSON(base, address, &point); err != nil {
		return nil, err
	}
	stations, err := httpsAddress(point.Properties.ObservationStations)
	if err != nil || stations.Host != weatherHost {
		return nil, base.Fail("the point has no NWS station listing")
	}
	id, err := nearestStation(base, stations.String())
	if err != nil {
		return nil, err
	}
	address = weatherAPI + "/stations/" + id + "/observations/latest"
	about := "NWS latest observation at " + id
	return celestialdata.Found(base.Product(id, id+"_latest.json",
		address, celestialdata.Unsized, about))
}

// nearestStation takes the first usable id from the ordered station list.
func nearestStation(base *celestialdata.Adapter,
	address string) (string, error) {
	var stations struct {
		Features []struct {
			Properties struct{ StationIdentifier string }
		}
	}
	if err := readJSON(base, address, &stations); err != nil {
		return "", err
	}
	for _, feature := range stations.Features {
		if id := feature.Properties.StationIdentifier; stationID(id) {
			return id, nil
		}
	}
	return "", base.Refuse("no observation station near that point")
}

// stationID recognizes the uppercase alphanumeric station codes.
func stationID(id string) bool {
	return id != "" && !strings.ContainsFunc(id, func(r rune) bool {
		return !(r >= '0' && r <= '9' || r >= 'A' && r <= 'Z')
	})
}
