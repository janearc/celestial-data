// Package celestialdata finds and fetches the files astronomy archives hold for
// a patch of sky. Each archive has its own adapter package. The adapter
// knows how its archive is laid out and what it holds near a position.
//
// This package holds the parts the adapters share. Fetch brings a product
// home.
package celestialdata

import (
	"compress/gzip"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"strings"

	"github.com/janearc/celestial-data/web"
)

// The bounds of the sky, in degrees.
const (
	raStart   = 0   // defined: where right ascension begins
	raEnd     = 360 // defined: right ascension goes once round
	southPole = -90 // defined: the declination of the south pole
	northPole = 90  // defined: the declination of the north pole
	widest    = 180 // defined: a circle on the sphere at its widest
	pointlike = 0   // defined: the radius of a circle that is no circle
)

// eitherSign asks math.IsInf about both infinities. It's defined by
// math.IsInf.
const eitherSign = 0

// The lengths Fetch compares, in bytes.
const (
	// unknown is the length web gives when the server says none. It's
	// defined by web.Web.
	unknown = -1
	// unsized is the size of a product whose archive says none. It's
	// defined: no bytes are promised.
	unsized = 0
)

// The modes Fetch makes things with, by the unix permission bits.
const (
	folderMode = 0o755 // defined: a folder anyone may read and list
	fileMode   = 0o644 // defined: a file anyone may read
)

// Position is a place on the sky, in ICRS degrees.
type Position struct {
	ra, dec float64
}

// NewPosition is the place at a right ascension and declination. It
// refuses numbers that are not on the sky.
func NewPosition(ra, dec float64) (*Position, error) {
	if !finite(ra) || ra < raStart || ra >= raEnd {
		return nil, fmt.Errorf("right ascension %v is not in [%d, %d) "+
			"degrees", ra, raStart, raEnd)
	}
	if !finite(dec) || dec < southPole || dec > northPole {
		return nil, fmt.Errorf("declination %v is not in [%d, %d] "+
			"degrees", dec, southPole, northPole)
	}
	return &Position{ra: ra, dec: dec}, nil
}

// RA is the right ascension, in degrees.
func (place Position) RA() float64 { return place.ra }

// Dec is the declination, in degrees.
func (place Position) Dec() float64 { return place.dec }

// Near is a circle on the sky: a centre and a radius in degrees.
type Near struct {
	centre Position
	radius float64
}

// NewNear is the circle of a radius round a centre. It refuses a missing
// centre, and a radius that is not a size a circle on the sky can have.
func NewNear(centre *Position, radius float64) (*Near, error) {
	if centre == nil {
		return nil, fmt.Errorf("%w: a circle needs a centre",
			errRefused)
	}
	if !finite(radius) || radius <= pointlike || radius > widest {
		return nil, fmt.Errorf("radius %v is not in (0, %d] degrees",
			radius, widest)
	}
	return &Near{centre: *centre, radius: radius}, nil
}

// Centre is the circle's centre.
func (circle Near) Centre() Position { return circle.centre }

// Radius is the circle's radius, in degrees.
func (circle Near) Radius() float64 { return circle.radius }

// finite is whether a number is neither infinite nor not a number.
func finite(number float64) bool {
	return !math.IsNaN(number) && !math.IsInf(number, eitherSign)
}

// Product is one file an archive holds.
type Product struct {
	archive, id, name, url, about string
	size                          int64
}

// newProduct is a file an archive holds; adapters make one through their
// base's Product. archive is the name of the adapter that found it. id is
// the archive's own name for what the file belongs to. That may be an
// observation or a survey tile.
//
// name is the file's name, and it keeps that name at home. url is where
// the archive holds it. size is its length in bytes. It is zero when the
// archive does not say.
//
// It refuses an empty address. It refuses a name that is not one plain
// file name. It refuses a size below zero. The archive's name was checked
// when its adapter was made.
func newProduct(archive, id, name, url string, size int64) (*Product,
	error) {
	if url == "" {
		return nil, fmt.Errorf("%w: a product needs an address",
			errRefused)
	}
	if err := plain(name); err != nil {
		return nil, err
	}
	if size < unsized {
		return nil, fmt.Errorf("%w: a size of %d bytes", errRefused,
			size)
	}
	return &Product{archive: archive, id: id, name: name, url: url,
		size: size}, nil
}

// Found is a find of one product, or of none and why. It takes what an
// adapter's Product gives. So an adapter that names one file hands a
// refusal on with no branch of its own.
func Found(product *Product, err error) ([]*Product, error) {
	if err != nil {
		return nil, err
	}
	return []*Product{product}, nil
}

// Archive is the name of the adapter that found the product.
func (product Product) Archive() string { return product.archive }

// ID is the archive's own name for what the file belongs to.
func (product Product) ID() string { return product.id }

// Name is the file's name.
func (product Product) Name() string { return product.name }

// URL is where the archive holds the file.
func (product Product) URL() string { return product.url }

// About says what the file is, in a few words.
func (product Product) About() string { return product.about }

// Archive is one archive's adapter.
type Archive interface {
	// Name is the adapter's name as the command line takes it: one word.
	Name() string
	// Find is what the archive holds near a position.
	Find(near *Near) ([]*Product, error)
}

// suffix is what a file is called while it is still coming in.
const suffix = ".part"

// errRefused is what Refused gives.
var errRefused = errors.New("refused")

// Refused is wrapped by each of the refusals here. NewProduct and Fetch
// refuse a name that is not one plain file name. Fetch refuses when a file
// or a .part is already there under the name.
func Refused() error { return errRefused }

// Refuse is a refusal that says why, as fmt.Errorf writes it. It wraps
// Refused, so the adapters' refusals are found as this package's are.
func Refuse(format string, args ...any) error {
	why := fmt.Sprintf(format, args...)
	return fmt.Errorf("%w: %s", errRefused, why)
}

// plain refuses a name that is not one plain file name.
func plain(name string) error {
	if name == "" || name != filepath.Base(name) || name == "." ||
		name == ".." || strings.HasSuffix(name, suffix) {
		return fmt.Errorf("%w: %q is not one plain file name",
			errRefused, name)
	}
	return nil
}

// Fetch brings a product's file into the folder into and returns its path.
// It makes the folder when it is missing.
//
// It never replaces a file. It refuses when a file is already there under
// the name. It refuses a .part already there too, since that may be
// another fetch's.
//
// The bytes go to a .part file of its own first. That file takes the name
// only once its length is what the server and the product said. For a gzip
// file the stream must also read to its end. Its own .part is removed when
// anything fails.
func Fetch(get web.Web, product *Product, into string) (string, error) {
	if product == nil {
		return "", fmt.Errorf("%w: no product", errRefused)
	}
	if err := plain(product.name); err != nil {
		return "", err
	}
	if err := os.MkdirAll(into, folderMode); err != nil {
		return "", err
	}
	home := filepath.Join(into, product.name)
	switch _, err := os.Lstat(home); {
	case err == nil:
		return "", fmt.Errorf("%w: %s is already there", errRefused,
			home)
	case !errors.Is(err, os.ErrNotExist):
		return "", err
	}
	part := home + suffix
	file, err := os.OpenFile(part, os.O_CREATE|os.O_EXCL|os.O_WRONLY,
		fileMode)
	if errors.Is(err, os.ErrExist) {
		return "", fmt.Errorf("%w: %s is already there", errRefused,
			part)
	}
	if err != nil {
		return "", err
	}
	// from here the part is this call's own, and it is removed on failure
	err = fill(get, product, file, part)
	if err == nil {
		err = os.Rename(part, home)
	}
	if err != nil {
		os.Remove(part)
		return "", err
	}
	return home, nil
}

// fill writes the product into the new file and checks what came.
func fill(get web.Web, product *Product, file *os.File, part string) error {
	body, promised, err := get.Get(product.url)
	if err != nil {
		file.Close()
		return err
	}
	got, copied := io.Copy(file, body)
	body.Close()
	if err := errors.Join(copied, file.Close()); err != nil {
		return err
	}
	if promised != unknown && got != promised {
		return fmt.Errorf("%s: %d bytes came, the server said %d",
			product.url, got, promised)
	}
	if product.size != unsized && got != product.size {
		return fmt.Errorf("%s: %d bytes came, the archive said %d",
			product.url, got, product.size)
	}
	if strings.HasSuffix(product.name, ".gz") {
		return whole(part)
	}
	return nil
}

// whole reads a gzip file to its end, so a cut-off one is caught.
func whole(path string) error {
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer file.Close()
	unzipped, err := gzip.NewReader(file)
	if err != nil {
		return fmt.Errorf("%s: %w", path, err)
	}
	if _, err := io.Copy(io.Discard, unzipped); err != nil {
		return fmt.Errorf("%s: %w", path, err)
	}
	return unzipped.Close()
}
