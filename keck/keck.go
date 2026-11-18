// Package keck finds the Keck Observatory Archive's HIRES spectra near a
// place on the sky: each public science frame whose pointing falls in
// the circle, and the spectra KOA extracted from it with MAKEE, one a
// CCD, where it has them. HIRES's echelle reaches lithium's 6708 A line.
//
// The frames are asked of KOA's TAP service, which answers anonymous
// questions with public frames only, and the extracted spectra of its
// level-1 list. Both, and where the files are fetched, were read on
// 2026-11-06 from KOA's pages and its answers.
//
// KOA asks that work using its data acknowledge it, and the principal
// investigators of the frames, as koa.ipac.caltech.edu/acknowledge.html
// says. The archive is asked only by name.
package keck

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"path"
	"strings"

	"github.com/janearc/celestial-data"
	"github.com/janearc/celestial-data/tap"
	"github.com/janearc/celestial-data/web"
)

// adapterName is the archive's name, as the command line takes it.
const adapterName = "keck"

// KOA's places: its TAP service, where a frame is fetched by its file
// handle, and where its level-1 files are listed and fetched.
const (
	koa     = "https://koa.ipac.caltech.edu"
	sync    = koa + "/TAP/sync"
	raw     = koa + "/cgi-bin/getKOA/nph-getKOA"
	listing = koa + "/cgi-bin/KoaAPI/nph-getL1list"
	reduced = koa + "/cgi-bin/KoaAPI/nph-dnloadL1data"
)

// What KOA is asked of HIRES.
const (
	instrument = "HIRES"
	table      = "koa_hires"
	columns    = "koaid, filehand, targname, " +
		"object, utdatetime, exptime, " +
		"deckname, progpi"
	// science keeps the frames of a target, not its calibrations.
	science = "koaimtyp = 'object'"
	// makee is where a frame's extracted spectra are in its level-1
	// list: makee/ccdN/fits, one a CCD, each named _N_Flux.
	makee     = "makee/"
	makeeFits = "/fits"
	flux      = "_Flux.fits.gz"
)

// least is the fewest frames a search may ask for. It's defined: a
// search that asks for none finds nothing.
const least = 1

// Archive is KOA's HIRES frames and spectra.
type Archive struct {
	base *celestialdata.Adapter
	most int
}

// New is KOA's HIRES frames, asked through get for at most most frames
// a search. It refuses no web and a most below one.
func New(get web.Web, most int) (*Archive, error) {
	base, err := celestialdata.NewAdapter(adapterName, get)
	if err != nil {
		return nil, err
	}
	if most < least {
		return nil, base.Refuse("needs at least one frame a "+
			"search, not %d",
			most)
	}
	return &Archive{base: base, most: most}, nil
}

// Name is the adapter's name as the command line takes it.
func (archive *Archive) Name() string { return archive.base.Name() }

// ByNameOnly says the archive is asked only when it is named.
func (*Archive) ByNameOnly() bool { return true }

// Find is each public HIRES science frame whose pointing falls in the
// circle, at most as many as the archive was made to ask for, in the
// order KOA gives them: its raw file, then its extracted spectra.
func (archive *Archive) Find(near *celestialdata.Near) (
	[]*celestialdata.Product, error) {
	adql := tap.Search(archive.most, columns, table, near, science, "")
	answer, err := tap.Query(archive.base.Web(), sync, adql)
	if err != nil {
		return nil, archive.base.Wrap(err)
	}
	var products []*celestialdata.Product
	for at := range answer.Len() {
		seen, err := sighted(answer, at)
		if err != nil {
			return nil, archive.base.Wrap(err)
		}
		found, err := archive.frame(seen)
		if err != nil {
			return nil, err
		}
		products = append(products, found...)
	}
	return products, nil
}

// frame is a frame's raw file and its extracted spectra.
func (archive *Archive) frame(seen *sighting) (
	[]*celestialdata.Product, error) {
	address := raw + "?" + url.Values{"filehand": {seen.filehand}}.Encode()
	about := seen.about()
	product, err := archive.base.Product(seen.koaid, seen.koaid, address,
		celestialdata.Unsized, about)
	if err != nil {
		return nil, archive.base.Wrap(err)
	}
	spectra, err := archive.spectra(seen, about)
	if err != nil {
		return nil, err
	}
	return append([]*celestialdata.Product{product}, spectra...), nil
}

// level1 is a frame's level-1 list as KOA answers it.
type level1 struct {
	Result struct {
		Prefix string `json:"lev1subdir_prefix"`
		Data   []struct {
			Subdir string   `json:"subdir"`
			Files  []string `json:"lev1files"`
		} `json:"data"`
	} `json:"result"`
}

// spectra is a frame's extracted spectra, as its level-1 list names
// them: none when KOA reduced none.
func (archive *Archive) spectra(seen *sighting, about string) (
	[]*celestialdata.Product, error) {
	asked := listing + "?" + url.Values{"instrument": {instrument},
		"koaid": {seen.koaid}, "filehand": {seen.filehand}}.Encode()
	body, _, err := archive.base.Web().Get(asked)
	if err != nil {
		return nil, archive.base.Wrap(err)
	}
	defer body.Close()
	var listed level1
	if err := json.NewDecoder(body).Decode(&listed); err != nil {
		return nil, archive.base.Fail("the level-1 list of %s: %w",
			seen.koaid, err)
	}
	var products []*celestialdata.Product
	for _, folder := range listed.Result.Data {
		if !strings.HasPrefix(folder.Subdir, makee) ||
			!strings.HasSuffix(folder.Subdir, makeeFits) {
			continue
		}
		for _, file := range folder.Files {
			if !strings.HasSuffix(file, flux) {
				continue
			}
			kept := path.Join(listed.Result.Prefix, folder.Subdir,
				file)
			address := reduced + "?" + url.Values{
				"instrument": {instrument},
				"koaid":      {seen.koaid},
				"filehand":   {kept}}.Encode()
			product, err := archive.base.Product(seen.koaid, file,
				address, celestialdata.Unsized,
				about+", extracted by MAKEE")
			if err != nil {
				return nil, archive.base.Wrap(err)
			}
			products = append(products, product)
		}
	}
	return products, nil
}

// sighting is a frame as KOA's table gives it.
type sighting struct {
	koaid, filehand, target, when, deck, investigator string
	exposure                                          float64
}

// sighted is a row of the answer. A frame names its target in targname
// or in object, whichever KOA filled.
func sighted(answer rows, at int) (*sighting, error) {
	koaid, koaidErr := answer.Text(at, "koaid")
	filehand, filehandErr := answer.Text(at, "filehand")
	target, targetErr := answer.Text(at, "targname")
	object, objectErr := answer.Text(at, "object")
	when, whenErr := answer.Text(at, "utdatetime")
	deck, deckErr := answer.Text(at, "deckname")
	investigator, investigatorErr := answer.Text(at, "progpi")
	exposure, exposureErr := answer.Number(at, "exptime")
	err := errors.Join(koaidErr, filehandErr, targetErr, objectErr, whenErr,
		deckErr, investigatorErr, exposureErr)
	if err != nil {
		return nil, err
	}
	if target == "" {
		target = object
	}
	return &sighting{koaid: koaid, filehand: filehand, target: target,
		when: when, deck: deck, investigator: investigator,
		exposure: exposure}, nil
}

// about is a few words on the frame.
func (seen sighting) about() string {
	return fmt.Sprintf("a HIRES frame of %s, %s, %.0f s through %s, PI %s",
		seen.target, seen.when, seen.exposure, seen.deck,
		seen.investigator)
}

// rows is what sighted reads of an answered table.
type rows interface {
	Text(at int, name string) (string, error)
	Number(at int, name string) (float64, error)
}
