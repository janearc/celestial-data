package dasch

import (
	"bytes"
	"fmt"
	"io"
	"net/url"
	"strconv"
	"strings"

	"github.com/janearc/celestial-data/web"
)

// unsaid is the length of an answer no one has asked for yet. It's
// defined by web.Web.
const unsaid = -1

// Lightcurves is a web that asks the archive for a light curve. Asked for a
// light curve's address, as Find makes it, it sends the question as a
// POST, which is the only way DR7 gives one.
//
// It opens the answer as the CSV it holds: the first line the names of
// the columns, then a line a measure. It asks the web under it for
// anything else unchanged.
type Lightcurves struct {
	under  web.Web
	asking *asker
}

// NewLightcurves is a web of light curves over under, asking through
// doer and naming itself to the archive by agent. It refuses no web under it,
// no doer and no agent.
func NewLightcurves(under web.Web, doer Doer, agent string) (*Lightcurves,
	error) {
	if under == nil || doer == nil || agent == "" {
		return nil, fmt.Errorf("light curves need a web, a doer and "+
			"an agent, not %v, %v and %q", under, doer, agent)
	}
	return &Lightcurves{under: under, asking: newAsker(doer, agent)}, nil
}

// Get opens what an address holds. For a light curve's address, that is
// the light curve, as CSV; it asks the archive once, and no more. An address
// that is a light curve's but holds no question it takes is an error.
func (curves *Lightcurves) Get(address string) (io.ReadCloser, int64,
	error) {
	asked, isCurve := question(address)
	if !isCurve {
		return curves.under.Get(address)
	}
	body, err := asked.body()
	if err != nil {
		return nil, unsaid, fmt.Errorf("%s: %w", address, err)
	}
	listed, err := curves.asking.post(curvePath, body)
	if err != nil {
		return nil, unsaid, fmt.Errorf("%s: %w", address, err)
	}
	if err := curveShape(listed); err != nil {
		return nil, unsaid, fmt.Errorf("%s: %w", address, err)
	}
	csv := strings.Join(listed, "\n") + "\n"
	return io.NopCloser(bytes.NewReader([]byte(csv))), int64(len(csv)), nil
}

// curveColumn is the first column of a light curve, as DR7's answer
// names it in its first line.
const curveColumn = "date_jd"

// curveShape refuses an answer that is not a light curve, so a 200 that
// holds an error or a page is not kept as one. The answer is kept as it
// came, its spelling with it.
//
// The first line names the columns, beginning with date_jd in either
// spelling (see snake); every line after it has as many fields.
func curveShape(listed []string) error {
	names := split(listed[first])
	if _, err := columnsAt(names); err != nil {
		return fmt.Errorf("the answer is not a light curve: %w", err)
	}
	if snake(names[first]) != curveColumn {
		return fmt.Errorf("the answer is not a light curve: its first "+
			"line begins %q, not %q", names[first], curveColumn)
	}
	for at, line := range listed[first+header:] {
		if fields := len(split(line)); fields != len(names) {
			return fmt.Errorf("the answer is not a light "+
				"curve: line "+
				"%d has %d fields, not the %d of its columns",
				at+linesBefore, fields, len(names))
		}
	}
	return nil
}

// linesBefore is how far a line's place in the list after the header is
// from its line number, counted from one with the header first. It's
// defined.
const linesBefore = 2

// Head is the length of what an address holds. A light curve has none
// until it has been asked for, and asking would be the whole question,
// so for a light curve it is unsaid.
func (curves *Lightcurves) Head(address string) (int64, error) {
	if _, isCurve := question(address); isCurve {
		return unsaid, nil
	}
	return curves.under.Head(address)
}

// Post sends a form through the web under it.
func (curves *Lightcurves) Post(address string, form url.Values) (string,
	error) {
	return curves.under.Post(address, form)
}

// asked is the question a light curve's address holds, as written.
type asked struct {
	refcat, number, bin string
}

// question is what a light curve's address asks, and whether it is one.
func question(address string) (asked, bool) {
	where, err := url.Parse(address)
	if err != nil {
		return asked{}, false
	}
	form := where.Query()
	where.RawQuery = ""
	if where.String() != root+curvePath {
		return asked{}, false
	}
	return asked{refcat: form.Get("refcat"), number: form.Get("ref_number"),
		bin: form.Get("gsc_bin_index")}, true
}

// body is the body of the POST the question is sent as. The numbers are
// to be whole numbers, and the catalogue one of the two the archive has; a
// question that is not is no body.
func (what asked) body() (*curve, error) {
	number, numberErr := strconv.ParseUint(what.number, decimal, bits64)
	bin, binErr := strconv.ParseUint(what.bin, decimal, bits32)
	if numberErr != nil || binErr != nil {
		return nil, fmt.Errorf("ref_number %q and "+
			"gsc_bin_index %q are to be whole numbers", what.number,
			what.bin)
	}
	if what.refcat != refcat {
		return nil, fmt.Errorf("the catalogue %q is not %q",
			what.refcat, refcat)
	}
	return &curve{Refcat: what.refcat, Number: number,
		Bin: uint32(bin)}, nil
}
