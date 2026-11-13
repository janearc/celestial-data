// Package starhorse is the adapter for StarHorse's APOGEE DR17 and Gaia
// EDR3 results (Queiroz et al. 2023), from AIP in Potsdam: for each star
// it fit, percentiles of its mass, distance, extinction and, where it
// gives one, its age, with flags on each.
//
// StarHorse gives ages only near the main-sequence turn-off and on the
// subgiant branch: its age flag is NOAGE for a star elsewhere, a red
// giant among them, read 2026-11-06 from its data page (sources.yaml).
//
// The archive is the one file, its rows its own, keyed by APOGEE_ID and
// Gaia's source, not allStar's order. The file writes no units: each
// unit here is its data page's.
package starhorse

import (
	_ "embed"

	"github.com/janearc/celestial-data"
)

// adapterName is the archive's name, as the command line takes it.
const adapterName = "starhorse"

// The file, as AIP keeps it.
const (
	// resultsName is the v2 results file, whose ages use Gaussian priors.
	resultsName = "APOGEE_DR17_EDR3_StarHorse_v2.fits"
	// address is where AIP's object store serves it.
	address = "https://s3.data.aip.de:9000/shaqueiroz2023/" + resultsName
	// resultsID is the id of the file's product.
	resultsID = "dr17 starhorse v2"
	// resultsSize is the name of the file's length in numbers.yaml.
	resultsSize = "starhorse-size"
	// resultsAbout is what the file holds.
	resultsAbout = "StarHorse's APOGEE DR17 results: masses, distances, " +
		"extinctions, ages"
)

// numbersFile is the numbers file that holds the file's length.
const numbersFile = "numbers.yaml"

//go:embed numbers.yaml
var numbersRaw []byte

// New is the archive of StarHorse's APOGEE DR17 results, asked by name
// only. It holds the one file, whatever the circle.
func New() (*celestialdata.Fixed, error) {
	size := int64(measured().Value(resultsSize))
	results := celestialdata.NewFile(resultsID, resultsName, address, size,
		resultsAbout)
	return celestialdata.NewFixed(adapterName, results)
}

// measured is the numbers file, read.
func measured() *celestialdata.Measured {
	return celestialdata.NewMeasured(numbersFile, numbersRaw)
}
