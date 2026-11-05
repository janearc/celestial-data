// Package sarao finds the files of SARAO's MeerKAT mosaic of the Galactic
// Centre: its total intensity at 1.28 GHz, its spectral index and that
// index's error.
//
// The DOI's landing page links them, wherever SARAO keeps it. The
// mosaic covers one field, so the page is asked only when the circle
// reaches it.
//
// The files are under CC BY-NC 4.0. The page asks that work using them
// cite arXiv:2201.10541.
package sarao

import (
	_ "embed"
	"strings"

	"github.com/janearc/celestial-data"
	"github.com/janearc/celestial-data/web"
)

// The places SARAO keeps the mosaic.
const (
	// doi is the mosaic's DOI, which names the files' set.
	doi = "10.48479/fyst-hj47"
	// page is the DOI, which leads to its landing page.
	page = "https://doi.org/" + doi
	// data is how a link into the files' folder begins its path, on
	// whichever host the files are kept.
	data = "/" + doi + "/data/"
)

// numbersFile is the numbers file that holds the field's header cards.
const numbersFile = "numbers.yaml"

//go:embed numbers.yaml
var numbersRaw []byte

// Footprint is the field the mosaic covers. It is read from the total
// intensity mosaic's header cards, kept beside this package.
func Footprint() (*celestialdata.Patch, error) {
	measured := celestialdata.NewMeasured(numbersFile, numbersRaw)
	return measured.Footprint(celestialdata.InICRS, "stokesi")
}

// Mosaic is the MeerKAT mosaic's files at SARAO.
type Mosaic struct {
	base *celestialdata.Adapter
}

// New is the mosaic asked through get, covering a field. Footprint gives
// the field. It refuses no web and no field.
func New(get web.Web, field *celestialdata.Patch) (*Mosaic, error) {
	base, err := celestialdata.NewField("meerkat", get, field)
	if err != nil {
		return nil, err
	}
	return &Mosaic{base: base}, nil
}

// Name is the adapter's name as the command line takes it.
func (mosaic *Mosaic) Name() string { return mosaic.base.Name() }

// Find is every FITS file the page links in the data folder, when the
// circle reaches the field. Otherwise it is nothing, and nothing is asked
// of the archive.
func (mosaic *Mosaic) Find(near *celestialdata.Near) (
	[]*celestialdata.Product, error) {
	if !mosaic.base.Reaches(near) {
		return nil, nil
	}
	links, err := mosaic.base.Links(page)
	if err != nil {
		return nil, err
	}
	about := "MeerKAT Galactic Centre mosaic, CC BY-NC 4.0, cite " +
		"arXiv:2201.10541"
	var products []*celestialdata.Product
	for _, link := range links {
		_, name, inData := strings.Cut(link, data)
		if !inData || !strings.HasSuffix(name, ".fits") {
			continue
		}
		product, err := mosaic.base.Product(doi, name, link,
			celestialdata.Unsized, about)
		if err != nil {
			// links into a folder below the data folder are not
			// its files.
			continue
		}
		products = append(products, product)
	}
	return products, nil
}
