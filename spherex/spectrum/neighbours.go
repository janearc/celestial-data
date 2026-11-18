package spectrum

import (
	"math"
	"sort"

	"github.com/janearc/celestial/psf"
)

// The steps a neighbour is found by. Each is defined.
const (
	around = 1 // a pixel's eight about it lie one step off
	middle = 2 // the median is the middle of the sorted values
)

// neighbours are the cutout's other stars, as its own peaks find them:
// a used pixel higher than the eight about it and peak-sigma over the
// cutout's median in its own sigma, placed at the centroid of its three
// by three. A peak within own-peak of the star is the star's.
func (measure *Measure) neighbours(tile psf.Tile, x, y float64,
	shape *psf.Shape) []psf.Star {
	median := medianOf(tile.Pixels)
	var found []psf.Star
	for row := around; row < tile.Height-around; row++ {
		for column := around; column < tile.Width-around; column++ {
			if !measure.isPeak(tile, column, row, median) {
				continue
			}
			cx, cy := centroid(tile, column, row, median)
			if math.Hypot(cx-x, cy-y) < measure.ownPeak {
				continue
			}
			found = append(found,
				psf.Star{X: cx, Y: cy, Shape: shape})
		}
	}
	return found
}

// isPeak is whether a pixel is used, peak-sigma over the median and
// higher than the eight about it.
func (measure *Measure) isPeak(tile psf.Tile, column, row int,
	median float64) bool {
	at := row*tile.Width + column
	value, variance := tile.Pixels[at], tile.Variance[at]
	if tile.Ignored[at] || !(variance > 0) || math.IsNaN(value) {
		return false
	}
	if value-median < measure.peakSigma*math.Sqrt(variance) {
		return false
	}
	for dy := -around; dy <= around; dy++ {
		for dx := -around; dx <= around; dx++ {
			other := (row+dy)*tile.Width + column + dx
			if other == at || tile.Ignored[other] {
				continue
			}
			if tile.Pixels[other] >= value {
				return false
			}
		}
	}
	return true
}

// centroid is the light-weighted centre of a pixel's three by three
// over the median. A pixel left out, not a number or under the median
// weighs nothing.
func centroid(tile psf.Tile, column, row int, median float64) (float64,
	float64) {
	var sum, across, up float64
	for dy := -around; dy <= around; dy++ {
		for dx := -around; dx <= around; dx++ {
			at := (row+dy)*tile.Width + column + dx
			weight := tile.Pixels[at] - median
			if tile.Ignored[at] || !(weight > 0) {
				continue
			}
			sum += weight
			across += weight * float64(column+dx)
			up += weight * float64(row+dy)
		}
	}
	return across / sum, up / sum
}

// medianOf is the middle of the numbers, those not a number left out.
func medianOf(values []float64) float64 {
	var kept []float64
	for _, value := range values {
		if !math.IsNaN(value) {
			kept = append(kept, value)
		}
	}
	if len(kept) == 0 {
		return math.NaN()
	}
	sort.Float64s(kept)
	return kept[len(kept)/middle]
}

// quality is the fit's quality over the star's circle: the mean of each
// used pixel's miss over its sigma, near one for a good fit, as the
// Help page's fit_ql is. It is not a number when no pixel is used.
func (measure *Measure) quality(tile psf.Tile, fitted psf.Fitted) float64 {
	star := tile.Stars[0]
	var sum float64
	var count int
	for at, value := range tile.Pixels {
		column, row := at%tile.Width, at/tile.Width
		dx, dy := float64(column)-star.X, float64(row)-star.Y
		sigma := math.Sqrt(tile.Variance[at])
		if math.Hypot(dx, dy) > measure.radius || tile.Ignored[at] ||
			!(sigma > 0) || math.IsNaN(value) {
			continue
		}
		model := fitted.Background
		for index, each := range tile.Stars {
			model += fitted.Fluxes[index] * each.Shape.Share(
				float64(column)-each.X, float64(row)-each.Y)
		}
		sum += math.Abs(value-model) / sigma
		count++
	}
	if count == 0 {
		return math.NaN()
	}
	return sum / float64(count)
}
