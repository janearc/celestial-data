package bsc5

import "github.com/janearc/celestial/sky"

// Star is one row of the catalogue: a star as the fifth edition gives
// it, the fields the reader reads.
type Star struct {
	hr               int
	name             string
	hd               int
	place            *sky.Equatorial
	visual, bv       float64
	hasVisual, hasBV bool
	spectral         string
}

// HR is the star's Harvard Revised number, its row in the catalogue,
// from 1 to 9110.
func (star Star) HR() int { return star.hr }

// Name is the star's Bayer or Flamsteed name as the catalogue writes
// it, such as "9Alp CMa" for Sirius, or none.
func (star Star) Name() string { return star.name }

// HD is the star's number in the Henry Draper Catalogue, and whether
// the catalogue gives one.
func (star Star) HD() (int, bool) { return star.hd, star.hd >= leastNumber }

// Place is where the star is on the sky, J2000, or nil for the few rows
// with none: novae and objects found not to be stars.
func (star Star) Place() *sky.Equatorial { return star.place }

// Visual is the star's visual magnitude, V, and whether the catalogue
// gives one.
func (star Star) Visual() (float64, bool) { return star.visual, star.hasVisual }

// BV is the star's B-V colour in the UBV system, in magnitudes, and
// whether the catalogue gives one.
func (star Star) BV() (float64, bool) { return star.bv, star.hasBV }

// Spectral is the star's spectral type as the catalogue writes it, such
// as "A1Vm", or none.
func (star Star) Spectral() string { return star.spectral }
