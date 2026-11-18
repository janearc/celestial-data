package dasch

import (
	"bytes"
	"encoding/json/v2"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/janearc/celestial-data/web"
)

// root is where the public tier of Harvard's Starglass API is kept: the
// tier that asks for no key, with the lower rate limit. The full tier is
// kept beside it and asks for a key in a header; it is not used here.
// Read 2026-11-08 from the archive's DR7 page on the web APIs.
const root = "https://api.starglass.cfa.harvard.edu/public"

// The two DR7 endpoints asked, as the web APIs page names them.
// Both are POST, with a JSON body, and answer a JSON list of strings,
// each a line of CSV, the first the names of the columns.
const (
	queryPath = "/dasch/dr7/querycat"
	curvePath = "/dasch/dr7/lightcurve"
)

// Doer is what a question is sent through. An *http.Client is one, and so
// is what web.Through gives, which is how a run on the network asks: see
// Policy.
type Doer interface {
	Do(request *http.Request) (*http.Response, error)
}

// mostBytes is the most an answer may hold. Picked on 2026-11-09:
// 32 MiB, some eighty times the 413,014 bytes of a light
// curve of 2,820 plates. A longer answer is refused, not read.
const mostBytes = 32 << 20

// onePast is the byte read beyond the limit. It is defined: one.
const onePast = 1

// ErrStopped is Starglass not being asked: it asked us to stop, the wait
// it asked for has not passed, or the circuit is open for this run. The
// error also holds the *web.Stopped that says which. A fetch of many
// light curves stops with the first stop.
var ErrStopped = errors.New("DASCH is not being asked")

// stoppedPost is a stop of Starglass seen from a question: the error the
// doer gave, said as it says itself, that is both ErrStopped and the
// *web.Stopped inside it.
type stoppedPost struct {
	cause error
}

// Error is the stop, said as the doer said it.
func (stopped *stoppedPost) Error() string { return stopped.cause.Error() }

// Unwrap is ErrStopped and the doer's own error, so errors.Is finds the
// first and errors.As finds the *web.Stopped.
func (stopped *stoppedPost) Unwrap() []error {
	return []error{ErrStopped, stopped.cause}
}

// asker sends the questions through a doer. It sends each once and no
// more: pacing, stopping when asked to, and any retry are the doer's
// work, which is a web.Through for a run on the network.
type asker struct {
	doer  Doer
	agent string
	root  string
}

// newAsker is an asker at the API's root.
func newAsker(doer Doer, agent string) *asker {
	return &asker{doer: doer, agent: agent, root: root}
}

// catalogue is the body of a box search of a reference catalogue.
type catalogue struct {
	Refcat string  `json:"refcat"`
	RA     float64 `json:"ra_deg"`
	Dec    float64 `json:"dec_deg"`
	Radius float64 `json:"radius_arcsec"`
}

// curve is the body of the question for one source's light curve. The
// numbers are those the catalogue search gave for the source.
type curve struct {
	Refcat string `json:"refcat"`
	Number uint64 `json:"ref_number"`
	Bin    uint32 `json:"gsc_bin_index"`
}

// post sends a body to an endpoint and gives the lines it answers with.
// A status but 200 is an error that names it. A stop of Starglass, which
// the doer gives as a *web.Stopped, is an error that is ErrStopped too.
// An answer that is not a list of lines, or is an empty one, is an error.
func (asking *asker) post(path string, body any) ([]string, error) {
	raw, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}
	request, err := http.NewRequest(http.MethodPost, asking.root+path,
		bytes.NewReader(raw))
	if err != nil {
		return nil, err
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Accept", "application/json")
	request.Header.Set("User-Agent", asking.agent)
	answer, err := asking.doer.Do(request)
	if err != nil {
		return nil, failed(path, err)
	}
	defer answer.Body.Close()
	if answer.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("POST %s answered with status %d", path,
			answer.StatusCode)
	}
	return lines(path, answer.Body)
}

// failed is the error of a question the doer could not answer. A stop is
// kept whole, so a caller finds it; any other error is named with the
// question.
func failed(path string, err error) error {
	var stop *web.Stopped
	if errors.As(err, &stop) {
		return &stoppedPost{cause: err}
	}
	return fmt.Errorf("POST %s failed: %w", path, err)
}

// lines is the list of lines a body holds, refused past mostBytes.
func lines(path string, body io.Reader) ([]string, error) {
	raw, err := io.ReadAll(io.LimitReader(body, mostBytes+onePast))
	if err != nil {
		return nil, fmt.Errorf("POST %s: reading the answer: %w",
			path, err)
	}
	if len(raw) > mostBytes {
		return nil, fmt.Errorf("POST %s answered with more than "+
			"%d bytes",
			path, mostBytes)
	}
	var listed []string
	if err := json.Unmarshal(raw, &listed); err != nil {
		return nil, fmt.Errorf("POST %s answered with something "+
			"that is not a list of lines: %w", path, err)
	}
	if len(listed) == none {
		return nil, fmt.Errorf("POST %s answered with no lines", path)
	}
	return listed, nil
}

// none is no lines. It's defined.
const none = 0

// split is a line of CSV in its fields. The archive's lines have no quoted
// field: daschlab, which reads them, splits at each comma.
func split(line string) []string {
	return strings.Split(line, ",")
}
