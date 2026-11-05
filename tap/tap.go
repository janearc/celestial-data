// Package tap asks a Table Access Protocol service one question in ADQL
// and waits for the answer. The archives here that hold tables of
// observations and of stars answer this way.
package tap

import (
	"fmt"
	"net/url"
	"strconv"
	"strings"

	"github.com/janearc/celestial-data"
	"github.com/janearc/celestial-data/votable"
	"github.com/janearc/celestial-data/web"
)

// The formats an answer may be asked for in.
const (
	asVOTable = "votable" // defined: TAP 1.1, the format all give
	CSV       = "csv"     // defined: TAP 1.1, values split by commas
)

// The parts of a number as ADQL is given it.
const (
	plain    = 'f' // defined by strconv: no exponent, which ADQL may refuse
	shortest = -1  // defined by strconv: the fewest digits that read back
	bits     = 64  // defined: a float64 is 64 bits
)

// URL is the address that asks a service's sync endpoint one ADQL query,
// for an answer in a format.
func URL(sync, adql, format string) string {
	asked := url.Values{}
	asked.Set("REQUEST", "doQuery")
	asked.Set("LANG", "ADQL")
	asked.Set("FORMAT", format)
	asked.Set("QUERY", adql)
	return sync + "?" + asked.Encode()
}

// Query asks a service's sync endpoint one ADQL query and reads the
// VOTable it answers with. When the service says the query failed, Query
// gives what it said.
func Query(get web.Web, sync, adql string) (*votable.Table, error) {
	body, _, err := get.Get(URL(sync, adql, asVOTable))
	if err != nil {
		return nil, err
	}
	defer body.Close()
	table, err := votable.Read(body)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", sync, err)
	}
	return table, nil
}

// Within is the ADQL condition that a table's columns of right ascension
// and declination fall within a circle.
//
//ninja:vow practice a library: a TAP question of one's own needs it
func Within(near *celestialdata.Near, ra, dec string) string {
	centre := near.Centre()
	centreRA, centreDec := number(centre.RA()), number(centre.Dec())
	radius := number(near.Radius())
	return fmt.Sprintf("CONTAINS(POINT('ICRS', %s, %s), "+
		"CIRCLE('ICRS', %s, %s, %s)) = 1", ra, dec, centreRA, centreDec,
		radius)
}

// number is a number written as ADQL reads it.
func number(value float64) string {
	return strconv.FormatFloat(value, plain, shortest, bits)
}

// Every asks Search for every row. It's defined by Search.
const Every = 0

// Search is the ADQL query for some columns of a table's rows whose ra and
// dec columns fall in a circle. top keeps that many rows, or Every does
// not limit them. and is a further condition, or none. order is what the
// rows are sorted by, or none.
func Search(top int, columns, table string, near *celestialdata.Near, and,
	order string) string {
	var query strings.Builder
	query.WriteString("SELECT ")
	if top > Every {
		fmt.Fprintf(&query, "TOP %d ", top)
	}
	within := Within(near, "ra", "dec")
	fmt.Fprintf(&query, "%s FROM %s WHERE %s", columns, table, within)
	if and != "" {
		query.WriteString(" AND " + and)
	}
	if order != "" {
		query.WriteString(" ORDER BY " + order)
	}
	return query.String()
}
