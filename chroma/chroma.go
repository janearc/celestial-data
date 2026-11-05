// Package chroma finds the colourways in jane's chroma repository on
// GitHub: the .css files of its colourways/sources folder, on its main
// branch.
//
// GitHub's contents API lists the folder as JSON, each file with its name
// and the address it is downloaded from. The listing is asked at fetch
// time, so a colourway added to the repository is found with no change
// here.
//
// GitHub limits an address that sends no token to sixty questions an
// hour. A fetch asks one.
//
// The repository is public. This package only fetches to the machine it
// runs on.
//
// The archive holds the same files wherever it is asked, and is asked
// only by name.
package chroma

import (
	"encoding/json"
	"io"
	"strings"

	"github.com/janearc/celestial-data"
	"github.com/janearc/celestial-data/web"
)

// The place GitHub lists the colourways.
const (
	// archive is the archive's name as the command line takes it.
	archive = "chroma"
	// sources is GitHub's listing of the folder, on the main branch.
	sources = "https://api.github.com/repos/janearc/chroma/contents/" +
		"colourways/sources?ref=main"
	// id is what the files belong to.
	id = "janearc/chroma"
	// css is how a colourway's name ends.
	css = ".css"
	// file is what GitHub calls an entry that is a file.
	file = "file"
	// about says what each file is.
	about = "a colourway from janearc/chroma, colourways/sources"
)

// entry is one thing GitHub lists in the folder.
type entry struct {
	Name        string `json:"name"`
	Type        string `json:"type"`
	DownloadURL string `json:"download_url"`
}

// Colourways is the chroma repository's colourways at GitHub.
type Colourways struct {
	base *celestialdata.Adapter
}

// New is the colourways asked through get. It refuses no web.
func New(get web.Web) (*Colourways, error) {
	base, err := celestialdata.NewAdapter(archive, get)
	if err != nil {
		return nil, err
	}
	return &Colourways{base: base}, nil
}

// Name is the adapter's name as the command line takes it.
func (colours *Colourways) Name() string { return colours.base.Name() }

// ByNameOnly says the archive is asked only when it is named: what it
// holds does not depend on a place in the sky.
func (*Colourways) ByNameOnly() bool { return true }

// Find is every .css file GitHub lists in the folder, whatever the
// circle. A listing of none is an error, since the folder is not the one
// that was meant. An entry that is not a file, is not a .css file, has no
// address, or is not one plain file name is passed over.
func (colours *Colourways) Find(*celestialdata.Near) (
	[]*celestialdata.Product, error) {
	body, _, err := colours.base.Web().Get(sources)
	if err != nil {
		return nil, colours.base.Wrap(err)
	}
	defer body.Close()
	raw, err := io.ReadAll(body)
	if err != nil {
		return nil, colours.base.Wrap(err)
	}
	var listed []entry
	if err := json.Unmarshal(raw, &listed); err != nil {
		return nil, colours.base.Fail("the listing at %s is not a "+
			"list of files: %w", sources, err)
	}
	var products []*celestialdata.Product
	for _, each := range listed {
		if each.Type != file || !strings.HasSuffix(each.Name, css) {
			continue
		}
		product, err := colours.base.Product(id, each.Name,
			each.DownloadURL, celestialdata.Unsized, about)
		if err != nil {
			// an entry with no address, or a name that leads out
			// of the folder, is not the folder's file.
			continue
		}
		products = append(products, product)
	}
	if len(products) == none {
		return nil, colours.base.Fail("no .css file is listed at %s. "+
			"Is that the colourways folder?", sources)
	}
	return products, nil
}

// none is no products. It's defined.
const none = 0
