// Package nustar finds NuSTAR's cleaned event files near a place on the
// sky. It asks HEASARC's numaster table which observations looked there,
// longest first. Each observation holds two event files, one for each of
// the telescope's two focal plane modules.
package nustar

import (
	"errors"
	"fmt"

	"github.com/janearc/celestial-data"
	"github.com/janearc/celestial-data/tap"
	"github.com/janearc/celestial-data/web"
)

// The places NuSTAR's records and files are kept.
const (
	sync = "https://heasarc.gsfc.nasa.gov/xamin/vo/tap/sync"
	tree = "https://heasarc.gsfc.nasa.gov/FTP/nustar/data/obs"
)

// The archive files an observation under the second and third digits of
// its id, then under the first. These are the places of those digits.
const (
	digits      = 11 // defined: NuSTAR's observation ids are eleven digits
	firstDigit  = 0  // defined: the id's first digit
	secondDigit = 1  // defined: the id's second digit
	fourthDigit = 3  // defined: the digit after the third
)

// least is the fewest observations a search may ask for. It's defined: a
// search that asks for none finds nothing.
const least = 1

// modules are NuSTAR's two focal plane modules. Each has its own event
// file.
var modules = []string{"A", "B"}

// Archive is NuSTAR's observations, as the archive keeps them.
type Archive struct {
	base *celestialdata.Adapter
	most int
}

// New is NuSTAR's archive, asked through get for at most most
// observations a search. It refuses no web and a most below one.
func New(get web.Web, most int) (*Archive, error) {
	base, err := celestialdata.NewAdapter("nustar", get)
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

// Find is the cleaned event files of the longest observations whose
// pointing falls in the circle. Two files come for each observation.
func (archive *Archive) Find(near *celestialdata.Near) (
	[]*celestialdata.Product, error) {
	adql := tap.Search(archive.most, "obsid, name, exposure_a, ra, dec",
		"numaster", near, "exposure_a > 0", "exposure_a DESC")
	table, err := tap.Query(archive.base.Web(), sync, adql)
	if err != nil {
		return nil, archive.base.Wrap(err)
	}
	var products []*celestialdata.Product
	for at := range table.Len() {
		obsid, obsidErr := table.Text(at, "obsid")
		target, targetErr := table.Text(at, "name")
		exposure, exposureErr := table.Number(at, "exposure_a")
		err := errors.Join(obsidErr, targetErr, exposureErr)
		if err != nil {
			return nil, archive.base.Wrap(err)
		}
		found, err := archive.observation(obsid, target, exposure)
		if err != nil {
			return nil, err
		}
		products = append(products, found...)
	}
	return products, nil
}

// observation is the event files one observation holds.
func (archive *Archive) observation(obsid, target string,
	exposure float64) ([]*celestialdata.Product, error) {
	if len(obsid) != digits {
		return nil, archive.base.Fail("observation %q is not %d digits",
			obsid, digits)
	}
	series := obsid[firstDigit:secondDigit]
	folder := fmt.Sprintf("%s/%s/%s/%s/event_cl", tree,
		obsid[secondDigit:fourthDigit], series, obsid)
	var products []*celestialdata.Product
	for _, module := range modules {
		name := "nu" + obsid + module + "01_cl.evt.gz"
		about := fmt.Sprintf("%s, module %s, %.0f s", target, module,
			exposure)
		product, err := archive.base.Product(obsid, name,
			folder+"/"+name, celestialdata.Unsized, about)
		if err != nil {
			return nil, err
		}
		products = append(products, product)
	}
	return products, nil
}
