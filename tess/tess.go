// Package tess finds TESS's light curves of a place on the sky, as MAST
// keeps them: those the mission made and those of its high level science
// products, QLP's among them, each fetched as a FITS file.
//
// The light curves are asked of MAST's API through internal/caom: its
// cone search and then each observation's products.
//
// The archive is asked only by name. Its full-frame images are not
// found: each is a whole CCD, and a cutout is another adapter's work.
package tess

import (
	"fmt"

	"github.com/janearc/celestial-data"
	"github.com/janearc/celestial-data/internal/caom"
	"github.com/janearc/celestial-data/web"
)

// adapterName is the archive's name, as the command line takes it.
const adapterName = "tess"

// What is kept of MAST's answers, as its answers name them.
const (
	mission    = "TESS"
	timeSeries = "timeseries"
	science    = "SCIENCE"
)

// Archive is MAST's TESS light curves.
type Archive struct {
	base  *celestialdata.Adapter
	asker *caom.Asker
}

// New is TESS's light curves at MAST, asked through get. It refuses no
// web.
func New(get web.Web) (*Archive, error) {
	base, err := celestialdata.NewAdapter(adapterName, get)
	if err != nil {
		return nil, err
	}
	return &Archive{base: base, asker: caom.New(base)}, nil
}

// Name is the adapter's name as the command line takes it.
func (archive *Archive) Name() string { return archive.base.Name() }

// ByNameOnly says the archive is asked only when it is named.
func (*Archive) ByNameOnly() bool { return true }

// Find is each TESS light curve of the circle, in the order MAST gives
// its observations: the mission's and its high level products'.
func (archive *Archive) Find(near *celestialdata.Near) (
	[]*celestialdata.Product, error) {
	seen, err := archive.asker.Cone(near)
	if err != nil {
		return nil, err
	}
	var products []*celestialdata.Product
	for _, each := range seen {
		if each.Project != mission || each.Kind != timeSeries {
			continue
		}
		found, err := archive.products(each)
		if err != nil {
			return nil, err
		}
		products = append(products, found...)
	}
	return products, nil
}

// products is an observation's science files, each a product.
func (archive *Archive) products(seen caom.Observation) (
	[]*celestialdata.Product, error) {
	files, err := archive.asker.Files(seen.ObsID)
	if err != nil {
		return nil, err
	}
	id := fmt.Sprint(seen.ObsID)
	about := fmt.Sprintf("a TESS light curve of %s, sector %d, %.0f s "+
		"cadence, from %s", seen.Target, seen.Sequence, seen.Exposure,
		seen.Provenance)
	var products []*celestialdata.Product
	for _, each := range files {
		if each.Kind != science {
			continue
		}
		product, err := archive.base.Product(id, each.Name,
			caom.Download(each.URI), each.Size, about)
		if err != nil {
			return nil, archive.base.Wrap(err)
		}
		products = append(products, product)
	}
	return products, nil
}
