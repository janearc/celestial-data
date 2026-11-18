package earth

import (
	"fmt"
	"math"
	"net/url"

	"github.com/janearc/celestial-data"
)

const (
	terrainZoom   = 14
	offshoreZoom  = 10
	mercatorLimit = 85.0511287798066
	terrainBucket = "https://s3.amazonaws.com/elevation-tiles-prod/terrarium"
	etopo2022     = "https://oceanwatch.pifsc.noaa.gov/erddap/griddap/ETOPO_2022_v1_15s.csv"
	etopoBoxLimit = 1
)

// findTerrainTiles names the Mapzen elevation tiles that cover a box.
func findTerrainTiles(base *celestialdata.Adapter,
	query Query) ([]*celestialdata.Product, error) {
	return terrainProducts(base, query, terrainZoom)
}

// findOffshoreTiles names lower-zoom tiles with regional offshore relief.
func findOffshoreTiles(base *celestialdata.Adapter,
	query Query) ([]*celestialdata.Product, error) {
	return terrainProducts(base, query, offshoreZoom)
}

// terrainProducts selects one zoom whose complete tile set fits the limit.
func terrainProducts(base *celestialdata.Adapter, query Query,
	zoom int) ([]*celestialdata.Product, error) {
	b := *query.Bounds
	if b[1] < -mercatorLimit || b[3] > mercatorLimit {
		return nil, base.Refuse("box exceeds Web Mercator coverage")
	}
	var west, north, east, south, count int
	for {
		west, north = terrainTile(b[0], b[3], zoom)
		east, south = terrainTile(math.Nextafter(b[2], b[0]),
			math.Nextafter(b[1], b[3]), zoom)
		count = (east - west + 1) * (south - north + 1)
		if count <= query.Most {
			break
		}
		zoom--
	}
	products := make([]*celestialdata.Product, 0, count)
	for y := north; y <= south; y++ {
		for x := west; x <= east; x++ {
			id := fmt.Sprintf("%d/%d/%d", zoom, x, y)
			name := fmt.Sprintf("%d-%d-%d.png", zoom, x, y)
			product, err := base.Product(id, name,
				terrainBucket+"/"+id+".png", celestialdata.Unsized,
				"Mapzen Terrarium heights, EPSG:3857, source resolution varies")
			if err != nil {
				return nil, base.Wrap(err)
			}
			products = append(products, product)
		}
	}
	return products, nil
}

// terrainTile maps a geographic coordinate to a tile index.
func terrainTile(longitude, latitude float64, zoom int) (int, int) {
	n := 1 << zoom
	x := (longitude + 180) / 360
	y := (1 - math.Asinh(math.Tan(latitude*math.Pi/180))/math.Pi) / 2
	return min(n-1, max(0, int(math.Floor(x*float64(n))))),
		min(n-1, max(0, int(math.Floor(y*float64(n)))))
}

// findETOPO names a bounded native NOAA ETOPO 2022 CSV subset.
func findETOPO(base *celestialdata.Adapter,
	query Query) ([]*celestialdata.Product, error) {
	b := *query.Bounds
	if b[2]-b[0] > etopoBoxLimit || b[3]-b[1] > etopoBoxLimit {
		return nil, base.Refuse("each box side must be at most one degree")
	}
	slice := fmt.Sprintf("z[(%g):1:(%g)][(%g):1:(%g)]", b[1], b[3], b[0], b[2])
	product, err := base.Product("ETOPO_2022_v1_15s", "etopo-2022.csv",
		etopo2022+"?"+url.QueryEscape(slice), celestialdata.Unsized,
		"NOAA ETOPO 2022 15-arc-second elevation in metres")
	if err != nil {
		return nil, base.Wrap(err)
	}
	return []*celestialdata.Product{product}, nil
}
