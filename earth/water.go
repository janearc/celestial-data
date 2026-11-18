package earth

import (
	"net/url"
	"strings"

	"github.com/janearc/celestial-data"
)

// usgsLatest is the USGS latest observation for each continuous time series.
// Each returned feature retains its station, coordinate, parameter, unit,
// observation time, qualifier and approval status.
const usgsLatest = "https://api.waterdata.usgs.gov/ogcapi/v1/" +
	"collections/latest-continuous/items"

type waterParameter struct {
	name, code, about string
}

var waterParameters = []waterParameter{
	{"turbidity", "63680", "Turbidity, formazin nephelometric units"},
	{"salinity", "90860", "Practical salinity at 25 degrees Celsius"},
	{"salinity-ppt", "00480", "Salinity, parts per thousand"},
	{"sediment", "80154", "Suspended sediment concentration, mg/L"},
	{"estimated-sediment", "80300",
		"Suspended sediment concentration estimated at a point, mg/L"},
}

// findWaterQuality offers bounded USGS snapshots, one per parameter.
// The service pages large responses; its GeoJSON links include the next page.
func findWaterQuality(base *celestialdata.Adapter,
	query Query) ([]*celestialdata.Product, error) {
	wanted := strings.ToLower(strings.TrimSpace(query.Text))
	parameters := waterParameters
	if wanted != "" {
		parameters = nil
		for _, parameter := range waterParameters {
			if wanted == parameter.name || wanted == parameter.code {
				parameters = append(parameters, parameter)
				break
			}
		}
		if len(parameters) == 0 && waterCode(wanted) {
			parameters = []waterParameter{{wanted, wanted,
				"USGS water parameter " + wanted}}
		}
		if len(parameters) == 0 {
			return nil, base.Refuse("text must name a water parameter " +
				"or give its five-digit USGS parameter code")
		}
	}
	var products []*celestialdata.Product
	for _, parameter := range parameters {
		values := url.Values{
			"bbox":           {numbers(",", query.Bounds[:]...)},
			"f":              {"json"},
			"limit":          {"1000"},
			"parameter_code": {parameter.code},
		}
		name := parameter.code + "_" + numbers("_", query.Bounds[:]...) + ".json"
		product, err := base.Product(parameter.code, name,
			usgsLatest+"?"+values.Encode(), celestialdata.Unsized,
			parameter.about+"; latest by time series, check dates and pagination")
		if err != nil {
			return nil, base.Wrap(err)
		}
		products = append(products, product)
		if len(products) == query.Most {
			break
		}
	}
	return products, nil
}

func waterCode(code string) bool {
	if len(code) != 5 {
		return false
	}
	for _, digit := range code {
		if digit < '0' || digit > '9' {
			return false
		}
	}
	return true
}
