package starhorse

// Star is one row of the StarHorse file: the posterior StarHorse found
// for one star, from its APOGEE spectrum, its photometry and Gaia's
// parallax. Units are the data page's.
type Star struct {
	id                         string
	gaia                       int64
	inGaia                     bool
	mass, distance, extinction Spread
	age                        Spread
	ageLow, ageHigh            float64
	teff, logg, metals         float64
	output, ageFlag            string
	ageWarning                 string
}

// Spread is a value's 16th, 50th and 84th percentiles.
type Spread struct{ low, median, high float64 }

// Low is the 16th percentile.
func (spread Spread) Low() float64 { return spread.low }

// Median is the 50th percentile.
func (spread Spread) Median() float64 { return spread.median }

// High is the 84th percentile.
func (spread Spread) High() float64 { return spread.high }

// ID is the star's id in APOGEE, its APOGEE_ID.
func (star Star) ID() string { return star.id }

// GaiaID is the star's Gaia DR3 source, DR3_source_id, and whether it
// has one.
func (star Star) GaiaID() (int64, bool) { return star.gaia, star.inGaia }

// Mass is the star's mass in suns, mass16, mass50 and mass84.
func (star Star) Mass() Spread { return star.mass }

// Distance is the star's distance in kpc, dist16, dist50 and dist84.
func (star Star) Distance() Spread { return star.distance }

// Extinction is the star's extinction in magnitudes at 542 nm, AV16,
// AV50 and AV84.
func (star Star) Extinction() Spread { return star.extinction }

// Age is the star's age in Gyr, age16, age50 and age84: not a number
// where AgeFlag is NOAGE.
func (star Star) Age() Spread { return star.age }

// AgeWide is the age's 5th and 95th percentiles in Gyr, age05 and
// age95.
func (star Star) AgeWide() (float64, float64) {
	return star.ageLow, star.ageHigh
}

// Temperature is the star's effective temperature in kelvin, teff50.
func (star Star) Temperature() float64 { return star.teff }

// Gravity is the star's log g in dex, logg50.
func (star Star) Gravity() float64 { return star.logg }

// Metals is the star's metallicity in dex, met50.
func (star Star) Metals() float64 { return star.metals }

// OutputFlags is StarHorse's flags on the quality of its results,
// StarHorse_OUTPUTFLAGS, as the file writes them.
func (star Star) OutputFlags() string { return star.output }

// AgeFlag is the evolutionary stage the age was fit for, MSTO or SGB,
// or NOAGE where none was, StarHorse_AGEFLAG.
func (star Star) AgeFlag() string { return star.ageFlag }

// AgeWarning is StarHorse's warning on the age, StarHorse_AGE_INOUT.
func (star Star) AgeWarning() string { return star.ageWarning }
