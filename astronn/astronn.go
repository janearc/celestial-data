// Package astronn is the adapter for astroNN's catalogue of APOGEE DR17,
// from the Sloan Digital Sky Survey: a row for each of allStar's rows, in
// allStar's order, with what Leung and Bovy's networks read from each
// spectrum, among them an age, a distance, and an orbit about the Galaxy.
//
// The archive is the one file. A Catalogue reads it, on disk or any
// io.ReaderAt, and a Star is one of its rows. The file writes no units:
// each unit here is the one SDSS's data model gives, read 2026-11-06.
//
// The file names some columns as the data model does not: age, dist and
// the orbit's are lower-cased in it (AGE in the model, age in the file).
// The reader asks for each as the file names it (sources.yaml).
package astronn

import (
	_ "embed"

	"github.com/janearc/celestial-data"
)

// adapterName is the archive's name, as the command line takes it.
const adapterName = "astronn"

// The file, as the Science Archive Server keeps it.
const (
	// catalogueName is the DR17 catalogue's file.
	catalogueName = "apogee_astroNN-DR17.fits"
	// address is where the server serves it.
	address = "https://data.sdss.org/sas/dr17/env/APOGEE_ASTRO_NN/" +
		catalogueName
	// catalogueID is the id of the file's product.
	catalogueID = "dr17 astroNN"
	// catalogueSize is the name of the file's length in numbers.yaml.
	catalogueSize = "astronn-size"
	// catalogueAbout is what the file holds.
	catalogueAbout = "astroNN's APOGEE DR17 catalogue: ages, distances " +
		"and orbits"
)

// numbersFile is the numbers file that holds the file's length.
const numbersFile = "numbers.yaml"

//go:embed numbers.yaml
var numbersRaw []byte

// New is the archive of astroNN's DR17 catalogue, asked by name only. It
// holds the one file, whatever the circle.
func New() (*celestialdata.Fixed, error) {
	size := int64(measured().Value(catalogueSize))
	catalogue := celestialdata.NewFile(catalogueID, catalogueName, address,
		size, catalogueAbout)
	return celestialdata.NewFixed(adapterName, catalogue)
}

// measured is the numbers file, read.
func measured() *celestialdata.Measured {
	return celestialdata.NewMeasured(numbersFile, numbersRaw)
}
