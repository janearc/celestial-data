// Package gaia is the adapter for ESA's Gaia archive: rows of Gaia DR3's
// gaia_source, as FITS, asked as a job of the archive's TAP service. A
// job runs under the session SignIn starts, so the archive knows whose
// it is.
//
// It reads what a job answered, too: a Catalogue is the product on
// disk, and a Source one of its rows as celestial's types, for
// celestial's galactocentric frame and orbit's solver to take.
//
// The archive's account of its service, read 2026-11-05:
// https://www.cosmos.esa.int/web/gaia-users/archive/programmatic-access
package gaia

import (
	"fmt"
	"net/url"
	"strconv"
	"strings"

	"github.com/janearc/celestial-data"
	"github.com/janearc/celestial-data/tap"
	"github.com/janearc/celestial-data/web"
)

// Service is the archive's TAP service. Its session is kept for it.
const Service = "https://gea.esac.esa.int/tap-server"

// What the archive is asked, by its programmatic-access page.
const (
	async  = Service + "/tap/async" // its endpoint for jobs
	signIn = Service + "/login"     // its form for signing in
	table  = "gaiadr3.gaia_source"  // DR3's table of every source
	format = "fits"                 // one of the formats it answers in
)

// whole is the radius of a circle that holds the whole sky. It's defined
// by celestialdata.NewNear, whose widest circle it is.
const whole = 180

// The parts of a number as a file name writes it.
const (
	plain    = 'f' // defined by strconv: no exponent
	shortest = -1  // defined by strconv: the fewest digits that read back
	bits     = 64  // defined: a float64 is 64 bits
)

// Archive is gaia_source, or a table joined to it, asked for some
// columns of the rows that meet a condition. Its files are named for
// what it asks: gaia_source's for gaia_source.
type Archive struct {
	base           *celestialdata.Adapter
	table, stem    string
	columns, where string
}

// New is the archive, asked for columns of the rows that meet where, or
// of every row when where is none. It refuses no columns.
func New(columns, where string) (*Archive, error) {
	return Called("gaia", columns, where)
}

// Called is the archive as New makes it, known by another name: one
// command can then ask gaia_source more than one question, each by its
// own name. It refuses a name that is not one word, and no columns.
func Called(name, columns, where string) (*Archive, error) {
	return From(name, table, columns, where)
}

// From is the archive asked of another table than gaia_source: a join
// of gaia_source to a table that has no places of its own, as the
// distances of Bailer-Jones et al. (2021) are, so a circle can still
// find their rows.
//
// A file holds every star the circle finds, its neighbours too, in no
// order the archive promises, and a star the joined table has no value
// for is still a row, its value empty or NaN.
//
// A reader takes the star it wants by its source_id, never by its row's
// place, and says what it does with a missing value; sorting does not
// make a place an identity.
//
// Its files are named for the name it is called by. It refuses a name
// that is not one word, no table and no columns.
func From(name, from, columns, where string) (*Archive, error) {
	base, err := celestialdata.Named(name)
	if err == nil && strings.TrimSpace(from) == "" {
		err = base.Refuse("needs its table, not %q", from)
	}
	if err == nil && strings.TrimSpace(columns) == "" {
		err = base.Refuse("needs its columns, not %q", columns)
	}
	if err != nil {
		return nil, err
	}
	stem := "gaia_source"
	if from != table {
		stem = strings.ReplaceAll(name, "-", "_")
	}
	return &Archive{base: base, table: from, stem: stem, columns: columns,
		where: where}, nil
}

// Name is the adapter's name as the command line takes it.
func (archive *Archive) Name() string { return archive.base.Name() }

// ByNameOnly says the archive is asked only when it is named: its job
// runs under someone's name, and can run for hours.
func (*Archive) ByNameOnly() bool { return true }

// Find is one file: the rows within the circle, asked as a job when the
// file is fetched. A circle as wide as the sky asks for every row, with
// no circle in its query.
func (archive *Archive) Find(near *celestialdata.Near) (
	[]*celestialdata.Product, error) {
	centre := near.Centre()
	ra, dec := number(centre.RA()), number(centre.Dec())
	radius := number(near.Radius())
	name := fmt.Sprintf("%s_%s_%s_r%s.fits", archive.stem, ra, dec, radius)
	address := tap.URL(async, archive.query(near), format)
	about := fmt.Sprintf("the rows of %s within %s degrees", archive.table,
		radius)
	return celestialdata.Found(archive.base.Product(archive.table, name,
		address, celestialdata.Unsized, about))
}

// query is the ADQL for the rows within the circle.
func (archive *Archive) query(near *celestialdata.Near) string {
	if near.Radius() < whole {
		return tap.Search(tap.Every, archive.columns, archive.table,
			near,
			archive.where, "")
	}
	query := "SELECT " + archive.columns + " FROM " + archive.table
	if archive.where != "" {
		query += " WHERE " + archive.where
	}
	return query
}

// number is a number as a file name writes it.
func number(value float64) string {
	return strconv.FormatFloat(value, plain, shortest, bits)
}

// SignIn signs a user in to the archive through get, which keeps the
// session the archive sets. The password goes in the form and nowhere
// else.
func SignIn(get web.Web, user, password string) error {
	_, err := get.Post(signIn, url.Values{"username": {user},
		"password": {password}})
	if err != nil {
		return fmt.Errorf("signing in to the gaia archive as %s: %w",
			user, err)
	}
	return nil
}
