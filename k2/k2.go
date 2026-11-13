// Package k2 finds what Kepler's second mission, K2, holds of a place on
// the sky, as MAST keeps it: the light curves and target pixel files of
// the stars it watched there, and the full-frame images of each campaign
// whose field holds the place, each fetched as a FITS file.
//
// A full-frame image is the whole focal plane in one file, some 400 MB,
// whichever star is asked about; K2 took one or two a campaign. MAST
// keeps each in four forms, calibrated, raw, raw with WCS and its
// uncertainty, and each is found. fetch --match picks one.
//
// The products are asked of MAST's API through internal/caom: its cone
// search and then each observation's products. The archive is asked only
// by name.
package k2

import (
	"fmt"

	"github.com/janearc/celestial-data"
	"github.com/janearc/celestial-data/internal/caom"
	"github.com/janearc/celestial-data/web"
)

// adapterName is the archive's name, as the command line takes it.
const adapterName = "k2"

// What is kept of MAST's answers, as its answers name them: K2's own
// collection, its time series and its images, and their science files.
const (
	mission    = "K2"
	timeSeries = "timeseries"
	images     = "image"
	science    = "SCIENCE"
)

// Archive is MAST's K2 products.
type Archive struct {
	base  *celestialdata.Adapter
	asker *caom.Asker
}

// New is K2's products at MAST, asked through get. It refuses no web.
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

// kept is whether an observation is K2's own time series or image.
func kept(seen caom.Observation) bool {
	return seen.Collection == mission &&
		(seen.Kind == timeSeries || seen.Kind == images)
}

// Find is the science files of each of K2's observations whose
// footprint meets the circle, in the order MAST gives them.
func (archive *Archive) Find(near *celestialdata.Near) (
	[]*celestialdata.Product, error) {
	seen, err := archive.asker.Cone(near)
	if err != nil {
		return nil, err
	}
	var products []*celestialdata.Product
	for _, each := range seen {
		if !kept(each) {
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
	about := fmt.Sprintf("a K2 %s of %s, campaign %d, %.0f s, from %s",
		seen.Kind, seen.Target, seen.Sequence, seen.Exposure,
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
