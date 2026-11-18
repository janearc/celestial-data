package celestialdata

import (
	"fmt"
	"slices"
	"strings"

	"github.com/janearc/celestial-data/web"
)

// Unsized is a product's size when the archive does not say it ahead.
// It's defined: NewProduct takes none as not known.
const Unsized = 0

// Adapter is the base every archive's adapter is built on: its name, as
// the command line takes it, the web it asks through, and, for a survey
// of one part of the sky, its footprint. An adapter holds one, so its
// products, refusals and errors carry its name without its typing it.
type Adapter struct {
	name  string
	get   web.Web
	patch *Patch
}

// NewAdapter is an adapter by a name, asking through get. It refuses a
// name that is not one word and a missing web.
func NewAdapter(name string, get web.Web) (*Adapter, error) {
	named, err := Named(name)
	if err != nil {
		return nil, err
	}
	if get == nil {
		return nil, named.Refuse("needs a web")
	}
	named.get = get
	return named, nil
}

// NewField is an adapter by a name, asking through get, for a survey that
// covers one part of the sky, patch. A circle that does not reach it is
// asked nothing. It refuses what NewAdapter does, and a missing patch.
func NewField(name string, get web.Web, patch *Patch) (*Adapter, error) {
	adapter, err := NewAdapter(name, get)
	if err != nil {
		return nil, err
	}
	if patch == nil {
		return nil, adapter.Refuse("needs its footprint")
	}
	adapter.patch = patch
	return adapter, nil
}

// Named is an adapter by a name with no web of its own, for one that
// asks through a host it is given or asks nothing. It refuses a name that
// is not one word.
func Named(name string) (*Adapter, error) {
	if name == "" || strings.ContainsAny(name, " \t\n,") {
		return nil, Refuse("an adapter's name is one word, not %q",
			name)
	}
	return &Adapter{name: name}, nil
}

// Name is the adapter's name as the command line takes it.
func (adapter *Adapter) Name() string { return adapter.name }

// Web is what the adapter asks through, or nil for one Named.
func (adapter *Adapter) Web() web.Web { return adapter.get }

// Footprint is the part of the sky a field covers, or nil for an adapter
// that is not a field.
func (adapter *Adapter) Footprint() *Patch { return adapter.patch }

// Reaches is whether a circle reaches a field's footprint. An adapter
// that is not a field reaches every circle.
func (adapter *Adapter) Reaches(near *Near) bool {
	return adapter.patch == nil || near.Reaches(adapter.patch)
}

// Product is one file the archive holds, with the adapter's name for the
// archive and a few words on what it is, none when about is empty.
//
// id is the archive's own name for what the file belongs to; file is its
// name, one plain file name; url is where it is kept; size is its length
// in bytes, Unsized when the archive does not say.
//
// It refuses an empty address, a name that is not one plain file name,
// and a size below none.
func (adapter *Adapter) Product(id, file, url string, size int64,
	about string) (*Product, error) {
	product, err := newProduct(adapter.name, id, file, url, size)
	if err != nil {
		return nil, err
	}
	product.about = about
	return product, nil
}

// Refuse is a refusal that begins with the adapter's name and says why,
// as fmt.Errorf writes it. It wraps Refused, as Refuse does.
func (adapter *Adapter) Refuse(format string, args ...any) error {
	why := fmt.Sprintf(format, args...)
	return Refuse("%s %s", adapter.name, why)
}

// Wrap is an error that begins with the adapter's name, or none for none.
func (adapter *Adapter) Wrap(err error) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf("%s: %w", adapter.name, err)
}

// Fail is an error that begins with the adapter's name and says what went
// wrong, as fmt.Errorf writes it, %w and all.
func (adapter *Adapter) Fail(format string, args ...any) error {
	return adapter.Wrap(fmt.Errorf(format, args...))
}

// Links is the links on a folder's page, in the order written, their
// entities read; an error begins with the adapter's name.
func (adapter *Adapter) Links(folder string) ([]string, error) {
	links, err := listing(adapter.get, folder)
	if err != nil {
		return nil, adapter.Wrap(err)
	}
	return links, nil
}

// EndsIn keeps the links that end in a suffix, as a file's type: EndsIn
// ".fits" for Files keeps a folder's FITS files.
func EndsIn(suffix string) func(link string) bool {
	return ending(suffix).holds
}

// ending is a suffix a link may end in.
type ending string

// holds is whether a link ends in the suffix.
func (end ending) holds(link string) bool {
	return strings.HasSuffix(link, string(end))
}

// Files is the files a folder's page lists that keep holds, each as a
// product of one id and one about. A link that is not one plain file name
// leads out of the folder; it is not the folder's, and is passed over.
func (adapter *Adapter) Files(folder string, keep func(link string) bool,
	id, about string) ([]*Product, error) {
	links, err := adapter.Links(folder)
	if err != nil {
		return nil, err
	}
	var products []*Product
	for _, link := range links {
		if !keep(link) {
			continue
		}
		product, err := adapter.Product(id, link, folder+link, Unsized,
			about)
		if err != nil {
			continue
		}
		products = append(products, product)
	}
	return products, nil
}

// Fixed is an archive whose files are known ahead, wherever on the sky or
// the earth they are asked for: a whole-earth image, a library, a map. It
// is asked only by name, never with every archive, since what it holds
// does not depend on the circle.
type Fixed struct {
	base     *Adapter
	products []*Product
}

// File is one file a fixed archive holds, described ahead. NewFile makes
// one; Summed gives it a digest; NewFixed makes it a product.
type File struct {
	id, name, url, about string
	size                 int64
	digest               *Digest
}

// NewFile describes a file ahead, as Adapter.Product takes one: what it
// belongs to, its name, where it is kept, its length in bytes (Unsized
// when not known) and a few words on what it is. Nothing is refused here;
// NewFixed refuses what Product would.
func NewFile(id, name, url string, size int64, about string) File {
	return File{id: id, name: name, url: url, size: size, about: about}
}

// Summed is the file held to the sha256 sum of its bytes: NewFixed
// gives its product the digest, and Fetch keeps the fetched file only
// when its bytes sum to it.
func (file File) Summed(digest Digest) File {
	file.digest = &digest
	return file
}

// NewFixed is an archive by a name whose files are described ahead. Each
// is made a product as Adapter.Product makes one, so an adapter has no
// refusal of its own to handle; a file that carries a digest is held to
// it.
//
// It refuses a name that is not one word, no files, and a file Product
// refuses.
func NewFixed(name string, files ...File) (*Fixed, error) {
	base, err := Named(name)
	if err != nil {
		return nil, err
	}
	if len(files) == noFiles {
		return nil, base.Refuse("needs its files")
	}
	products := make([]*Product, len(files))
	for at, file := range files {
		products[at], err = base.Product(file.id, file.name, file.url,
			file.size, file.about)
		if err != nil {
			return nil, base.Wrap(err)
		}
		if file.digest != nil {
			products[at] = products[at].Checked(*file.digest)
		}
	}
	return &Fixed{base: base, products: products}, nil
}

// noFiles is a fixed archive with no files. It's defined.
const noFiles = 0

// Name is the archive's name as the command line takes it.
func (fixed *Fixed) Name() string { return fixed.base.Name() }

// Find is the archive's files, whatever the circle.
func (fixed *Fixed) Find(*Near) ([]*Product, error) {
	return slices.Clone(fixed.products), nil
}

// ByNameOnly says the archive is asked only when it is named.
func (*Fixed) ByNameOnly() bool { return true }

// ByName is an archive that is asked only when it is named, never with
// every archive. Fixed is one.
type ByName interface {
	Archive
	ByNameOnly() bool
}
