// Package mast finds the files of the HST/NICMOS Paschen-alpha survey of
// the Galactic Centre. They are a high level science product at MAST: the
// mosaic in each of its filters and its catalogues. The survey covers one
// field, so the archive is asked only when the circle reaches it.
package mast

import (
	_ "embed"

	"github.com/janearc/celestial-data"
	"github.com/janearc/celestial-data/web"
)

// folder is where MAST keeps the survey's files.
const folder = "https://archive.stsci.edu/pub/hlsp/hpsgc/"

// numbersFile is the numbers file that holds the field's header cards.
const numbersFile = "numbers.yaml"

//go:embed numbers.yaml
var numbersRaw []byte

// Footprint is the field the survey covers. It is read from the Paschen-
// alpha mosaic's header cards, kept beside this package.
func Footprint() (*celestialdata.Patch, error) {
	measured := celestialdata.NewMeasured(numbersFile, numbersRaw)
	return measured.Footprint(celestialdata.InGalactic, "palpha")
}

// Survey is the Paschen-alpha survey's files at MAST.
type Survey struct {
	base *celestialdata.Adapter
}

// New is the survey asked through get, covering a field. Footprint gives
// the field. It refuses no web and no field.
func New(get web.Web, field *celestialdata.Patch) (*Survey, error) {
	base, err := celestialdata.NewField("hpsgc", get, field)
	if err != nil {
		return nil, err
	}
	return &Survey{base: base}, nil
}

// Name is the adapter's name as the command line takes it.
func (survey *Survey) Name() string { return survey.base.Name() }

// Find is every FITS file the survey holds, when the circle reaches its
// field. Otherwise it is nothing, and nothing is asked of the archive.
func (survey *Survey) Find(near *celestialdata.Near) (
	[]*celestialdata.Product, error) {
	if !survey.base.Reaches(near) {
		return nil, nil
	}
	about := "HST/NICMOS Paschen-alpha survey of the Galactic Centre"
	return survey.base.Files(folder, celestialdata.EndsIn(".fits"), "hpsgc",
		about)
}
