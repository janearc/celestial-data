package netcdf

import (
	"math"
	"sort"
)

// fullTurn is the longitude's period in degrees. It's defined.
const fullTurn = 360.0

// corners is the number of values a bilinear mix takes. It's defined.
const corners = 4

// ValueAt is the field at a longitude and latitude in degrees, by
// bilinear interpolation, and whether there is a value. Longitude wraps,
// so the seam is a cell like any. Latitude clamps at the first and last
// row, which is the pole for a grid that reaches it.
//
// No data in any corner the place has a share of is no data; a corner it
// has no share of, because it is on a grid line, is not asked.
//
// A place that is not a number has no value.
func (slice *Slice) ValueAt(lon, lat float64) (float64, bool) {
	if math.IsNaN(lon) || math.IsNaN(lat) || math.IsInf(lon, 0) ||
		math.IsInf(lat, 0) {
		return 0, false
	}
	i, j, across := lonCell(slice.lons, lon)
	k, l, down := latCell(slice.lats, lat)
	width := len(slice.lons)
	values := [corners]float64{slice.values[k*width+i],
		slice.values[k*width+j], slice.values[l*width+i],
		slice.values[l*width+j]}
	weights := [corners]float64{(1 - across) * (1 - down),
		across * (1 - down), (1 - across) * down, across * down}
	return mixed(values, weights)
}

// mixed is the sum of values by their weights. A value that is not a
// number with a weight above none is no data.
func mixed(values, weights [corners]float64) (float64, bool) {
	total := 0.0
	for at, each := range values {
		if weights[at] == 0 {
			continue
		}
		if math.IsNaN(each) {
			return 0, false
		}
		total += each * weights[at]
	}
	return total, true
}

// lonCell is the two longitude columns a longitude lies between and its
// share of the way from the first to the second. The last column's
// neighbour is the first, a turn on.
//
// The turn is taken with math.Mod, which is exact, so that a longitude
// of any size lands in [0, 360) past the first column. Dividing and
// flooring loses the fraction above 2^53 and can leave a negative
// remainder.
//
// A small negative remainder that rounds up to a whole turn is the first
// column.
func lonCell(lons []float64, lon float64) (int, int, float64) {
	first := lons[0]
	along := math.Mod(lon-first, fullTurn)
	if along < 0 {
		along += fullTurn
	}
	if along >= fullTurn {
		along = 0
	}
	place := first + along
	i := sort.Search(len(lons), func(k int) bool {
		return lons[k] > place
	}) - 1
	j := (i + 1) % len(lons)
	span := lons[j] - lons[i]
	if j <= i {
		span += fullTurn
	}
	return i, j, (place - lons[i]) / span
}

// latCell is the two latitude rows a latitude lies between and its share
// of the way from the first to the second. Beyond the ends it is the end
// row.
func latCell(lats []float64, lat float64) (int, int, float64) {
	last := len(lats) - 1
	if lat <= lats[0] {
		return 0, 0, 0
	}
	if lat >= lats[last] {
		return last, last, 0
	}
	k := sort.Search(len(lats), func(i int) bool {
		return lats[i] > lat
	}) - 1
	return k, k + 1, (lat - lats[k]) / (lats[k+1] - lats[k])
}
