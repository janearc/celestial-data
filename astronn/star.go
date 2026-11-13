package astronn

// Star is one row of the astroNN file: what the networks read from one
// of allStar's spectra. Units are the data model's.
type Star struct {
	id, telescope         string
	gaia                  int64
	inGaia                bool
	distance, weighted    uncertain
	raw, linear, lowess   float64
	ageError, ageModel    float64
	eccentric, highest    float64
	pericentre, apocentre float64
	angular, energy       float64
}

// uncertain is a value and its uncertainty.
type uncertain struct{ value, uncertainty float64 }

// ID is the star's id in APOGEE, its APOGEE_ID, as allStar has it.
func (star Star) ID() string { return star.id }

// Telescope is the telescope the row's spectra came from.
func (star Star) Telescope() string { return star.telescope }

// GaiaID is the star's Gaia source, its source_id, and whether Gaia
// matched one.
func (star Star) GaiaID() (int64, bool) { return star.gaia, star.inGaia }

// Age is the star's age in Gyr as the catalogue recommends it,
// age_lowess_correct: the networks' age with their bias taken out by a
// LOWESS fit (Mackereth et al. 2019).
func (star Star) Age() float64 { return star.lowess }

// AgeUncorrected is the networks' age in Gyr before any correction, age.
func (star Star) AgeUncorrected() float64 { return star.raw }

// AgeLinear is the age in Gyr with a linear correction of its bias,
// age_linear_correct.
func (star Star) AgeLinear() float64 { return star.linear }

// AgeError is the age's whole uncertainty in Gyr, age_total_error.
func (star Star) AgeError() float64 { return star.ageError }

// AgeModelError is the part of the age's uncertainty in Gyr that is the
// networks' own, age_model_error.
func (star Star) AgeModelError() float64 { return star.ageModel }

// Distance is the star's distance in parsecs from its spectrum alone,
// dist, and its uncertainty.
func (star Star) Distance() (float64, float64) {
	return star.distance.value, star.distance.uncertainty
}

// WeightedDistance is the distance in parsecs the catalogue recommends,
// weighted_dist: the spectrum's distance weighed with Gaia's parallax,
// and its uncertainty.
func (star Star) WeightedDistance() (float64, float64) {
	return star.weighted.value, star.weighted.uncertainty
}

// Eccentricity is the eccentricity of the star's orbit about the
// Galaxy, e.
func (star Star) Eccentricity() float64 { return star.eccentric }

// Pericentre is the orbit's nearest distance from the Galaxy's centre in
// kpc, rperi.
func (star Star) Pericentre() float64 { return star.pericentre }

// Apocentre is the orbit's farthest distance from the centre in kpc,
// rap.
func (star Star) Apocentre() float64 { return star.apocentre }

// HighestAbovePlane is how far the orbit climbs from the Galaxy's plane
// in kpc, zmax.
func (star Star) HighestAbovePlane() float64 { return star.highest }

// AngularMomentum is the orbit's angular momentum about the Galaxy's
// axis in kpc km/s, Lz.
func (star Star) AngularMomentum() float64 { return star.angular }

// Energy is the orbit's energy in (km/s)^2, Energy.
func (star Star) Energy() float64 { return star.energy }
