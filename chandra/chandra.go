// Package chandra finds Chandra's primary data products near a place on
// the sky. It asks HEASARC's chanmaster table which archived observations
// looked there, longest first. Then it reads each observation's primary
// folder at the Chandra Data Archive and names the FITS files in it.
package chandra

import (
	"errors"
	"fmt"
	"strconv"

	"github.com/janearc/celestial-data"
	"github.com/janearc/celestial-data/tap"
	"github.com/janearc/celestial-data/web"
)

// The places Chandra's records and files are kept.
const (
	sync = "https://heasarc.gsfc.nasa.gov/xamin/vo/tap/sync"
	tree = "https://cxc.cfa.harvard.edu/cdaftp/byobsid"
)

// The numbers a search works with.
const (
	// least is the fewest observations a search may ask for. It's
	// defined: a search that asks for none finds nothing.
	least = 1
	// lastDigit takes an id's last digit, which the archive files the
	// observation under. It's defined: the last digit is the id modulo ten.
	lastDigit = 10
	// noID is the least an observation's id may be. It's defined: ids
	// count up from one.
	noID = 0
)

// Archive is Chandra's observations, as the archives keep them.
type Archive struct {
	base *celestialdata.Adapter
	most int
}

// New is Chandra's archive, asked through get for at most most
// observations a search. It refuses no web and a most below one.
func New(get web.Web, most int) (*Archive, error) {
	base, err := celestialdata.NewAdapter("chandra", get)
	if err != nil {
		return nil, err
	}
	if most < least {
		return nil, base.Refuse("needs at least one observation a "+
			"search, not %d", most)
	}
	return &Archive{base: base, most: most}, nil
}

// Name is the adapter's name as the command line takes it.
func (archive *Archive) Name() string { return archive.base.Name() }

// Find is the primary products of the longest archived observations whose
// pointing falls in the circle.
func (archive *Archive) Find(near *celestialdata.Near) (
	[]*celestialdata.Product, error) {
	adql := tap.Search(archive.most, "obsid, name, exposure, ra, dec, "+
		"detector, grating, status", "chanmaster", near,
		"status = 'archived'", "exposure DESC")
	table, err := tap.Query(archive.base.Web(), sync, adql)
	if err != nil {
		return nil, archive.base.Wrap(err)
	}
	var products []*celestialdata.Product
	for at := range table.Len() {
		obsid, obsidErr := table.Text(at, "obsid")
		target, targetErr := table.Text(at, "name")
		detector, detectorErr := table.Text(at, "detector")
		grating, gratingErr := table.Text(at, "grating")
		exposure, exposureErr := table.Number(at, "exposure")
		err := errors.Join(obsidErr, targetErr, detectorErr, gratingErr,
			exposureErr)
		if err != nil {
			return nil, archive.base.Wrap(err)
		}
		about := fmt.Sprintf("%s, %s with %s, %.0f s", target, detector,
			grating, exposure)
		found, err := archive.primary(obsid, about)
		if err != nil {
			return nil, err
		}
		products = append(products, found...)
	}
	return products, nil
}

// primary is the FITS files in one observation's primary folder.
func (archive *Archive) primary(obsid, about string) (
	[]*celestialdata.Product, error) {
	id, err := strconv.Atoi(obsid)
	if err != nil || id <= noID {
		return nil, archive.base.Fail("observation %q is not an id",
			obsid)
	}
	folder := fmt.Sprintf("%s/%d/%d/primary/", tree, id%lastDigit, id)
	return archive.base.Files(folder, celestialdata.EndsIn(".fits.gz"),
		obsid, about)
}
