// Package bluemarble fetches NASA's Blue Marble Next Generation, the
// whole earth's land and sea in true colour, a composite of a month of
// MODIS images, laid on a plain grid of longitude and latitude. It
// fetches Black Marble too, the earth's lights at night.
//
// NASA's images are free to use, but nothing of ours carries one: no
// Blue Marble file is committed to a repository, released, or shared as
// a picture made from it. This package only fetches it to the machine it
// runs on. Where a project keeps what it fetched, its .game omits it.
//
// The archive holds the two images wherever it is asked, and is asked
// only by name. Their lengths are kept in numbers.yaml.
package bluemarble

import (
	_ "embed"

	"github.com/janearc/celestial-data"
)

// archive is the archive's name as the command line takes it.
const archive = "bluemarble"

// numbersFile is the numbers file that holds the two images' lengths.
const numbersFile = "numbers.yaml"

//go:embed numbers.yaml
var numbersRaw []byte

// The July 2004 image at 5400 by 2700 pixels, as NASA's Visible Earth
// keeps it. Its pixels are a tenth of a degree a side less a third,
// longitude from -180 at the left, latitude from 90 at the top.
const (
	// julyID is what the image is of.
	julyID = "2004-07"
	// julyName is the image's file.
	julyName = "world.200407.3x5400x2700.jpg"
	// julyAddress is the download linked by NASA's current base-map page.
	julyAddress = "https://assets.science.nasa.gov/content/dam/science/" +
		"esd/eo/images/bmng/bmng-base/july/" + julyName
	// julySize is the name of its length in numbers.yaml.
	julySize = "july-size"
	// julyAbout says what it is.
	julyAbout = "NASA Blue Marble Next Generation, July 2004; " +
		"fetched locally, never shipped"
)

// Black Marble 2016, the earth at night from VIIRS: a year's cloud-free
// nights composited, a tenth of a degree a pixel, on the same grid as
// July. It is NASA's, and fetched to this machine only, as July is.
const (
	// nightID is what the image is of.
	nightID = "2016-night"
	// nightName is the image's file.
	nightName = "BlackMarble_2016_01deg.jpg"
	// nightAddress is the download linked by NASA's current night-map page.
	nightAddress = "https://assets.science.nasa.gov/content/dam/science/" +
		"esd/eo/images/imagerecords/144000/144898/" + nightName
	// nightSize is the name of its length in numbers.yaml.
	nightSize = "night-size"
	// nightAbout says what it is.
	nightAbout = "NASA Black Marble 2016, the earth at night; " +
		"fetched locally, never shipped"
)

// New is the archive of the July 2004 image and Black Marble 2016, with
// the lengths kept beside this package.
func New() (*celestialdata.Fixed, error) {
	lengths := celestialdata.NewMeasured(numbersFile, numbersRaw)
	july := int64(lengths.Value(julySize))
	night := int64(lengths.Value(nightSize))
	julyFile := celestialdata.NewFile(julyID, julyName, julyAddress, july,
		julyAbout)
	nightFile := celestialdata.NewFile(nightID, nightName, nightAddress,
		night, nightAbout)
	return celestialdata.NewFixed(archive, julyFile, nightFile)
}
