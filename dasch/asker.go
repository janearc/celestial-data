package dasch

import (
	"bytes"
	"encoding/json/v2"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// root is where the public tier of Harvard's Starglass API is kept: the
// tier that asks for no key, with the lower rate limit. The full tier is
// kept beside it and asks for a key in a header; it is not used here.
// Read 2026-11-08 from the DASCH DR7 page on the web APIs.
const root = "https://api.starglass.cfa.harvard.edu/public"

// The two DASCH DR7 endpoints asked, as the web APIs page names them.
// Both are POST, with a JSON body, and answer a JSON list of strings,
// each a line of CSV, the first the names of the columns.
const (
	queryPath = "/dasch/dr7/querycat"
	curvePath = "/dasch/dr7/lightcurve"
)

// Doer is what a question is sent through. An *http.Client is one.
type Doer interface {
	Do(request *http.Request) (*http.Response, error)
}

// pace is the least time from the start of one question an asker sends
// to the start of the next; its first goes at once. Starglass gives the
// public tier a lower rate limit and no number for it, so one a second
// was picked on 2026-11-09, for review 018.
const pace = time.Second

// rest is how long an asker leaves the API alone after a 429 whose
// Retry-After it cannot read, or that has none. Picked, as arXiv's
// adapter's rest is: five minutes.
const rest = 5 * time.Minute

// mostBytes is the most an answer may hold. Picked on 2026-11-09:
// 32 MiB, some eighty times the 413,014 bytes of a light
// curve of 2,820 plates. A longer answer is refused, not read.
const mostBytes = 32 << 20

// onePast is the byte read past mostBytes, which tells an answer that
// holds more from one that holds exactly so much. It's defined.
const onePast = 1

// ErrStopped is the API asking us to stop, by a 429. An asker that has
// been asked sends nothing more until the time the API gave, so a fetch
// of many light curves stops with the first refusal.
//
//ninja:vow well a sentinel errors.Is compares against; nothing sets it
var ErrStopped = errors.New("the API asked us to stop")

// asker sends DASCH's questions through a doer, one request each and
// none again: there are no retries here. Each question waits out the
// pace since the one before began, and none is sent while the API has
// asked us to stop.
type asker struct {
	doer        Doer
	agent       string
	root        string
	last, until time.Time
	now         func() time.Time
	sleep       func(time.Duration)
}

// newAsker is an asker at the API's root, on the wall's clock.
func newAsker(doer Doer, agent string) *asker {
	return &asker{doer: doer, agent: agent, root: root, now: time.Now,
		sleep: time.Sleep}
}

// wait refuses while the API's stop lasts, and otherwise waits out the
// pace since the last question began; a clock set back waits no more
// than the pace.
func (asking *asker) wait(path string) error {
	now := asking.now()
	if asking.until.After(now) {
		return fmt.Errorf("%w: POST %s not sent; it asked to be left "+
			"until %s", ErrStopped, path,
			asking.until.Format(time.RFC3339))
	}
	if asking.last.IsZero() {
		return nil
	}
	if left := asking.last.Add(pace).Sub(now); left > noWait {
		asking.sleep(min(left, pace))
	}
	return nil
}

// noWait is a wait with nothing left of it. It's defined: none.
const noWait time.Duration = 0

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
// A status but 200 is an error that names it, and, for a 429, which is
// the API asking us to stop, when it asked us to wait until. An answer
// that is not a list of lines, or is an empty one, is an error.
func (asking *asker) post(path string, body any) ([]string, error) {
	if err := asking.wait(path); err != nil {
		return nil, err
	}
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
	asking.last = asking.now()
	answer, err := asking.doer.Do(request)
	if err != nil {
		return nil, fmt.Errorf("POST %s failed: %w", path, err)
	}
	defer answer.Body.Close()
	if answer.StatusCode != http.StatusOK {
		return nil, asking.stopped(path, answer)
	}
	return lines(path, answer.Body)
}

// stopped is the error of an answer whose status was not 200. A 429 is
// the API asking us to stop: the asker keeps the time it gave, as
// seconds or a date, or rests when it gave none it can read, and the
// error is ErrStopped.
func (asking *asker) stopped(path string, answer *http.Response) error {
	if answer.StatusCode != http.StatusTooManyRequests {
		return fmt.Errorf("POST %s answered with status %d", path,
			answer.StatusCode)
	}
	now := asking.now()
	wait := answer.Header.Get("Retry-After")
	asking.until = now.Add(rest)
	if seconds, err := strconv.Atoi(wait); err == nil &&
		seconds >= noSeconds {
		asking.until = now.Add(time.Duration(min(seconds,
			longestSeconds)) * time.Second)
	} else if at, err := http.ParseTime(wait); err == nil {
		asking.until = at
	}
	return fmt.Errorf("%w: POST %s, Retry-After %q; nothing more is "+
		"asked until %s", ErrStopped, path, wait,
		asking.until.Format(time.RFC3339))
}

// noSeconds is a Retry-After of no wait. It's defined: none.
const noSeconds = 0

// longestSeconds is the longest Retry-After kept, in seconds: the most a
// Duration holds, some 292 years. It's defined by time.Duration.
const longestSeconds = int(^uint64(0)>>1) / int(time.Second)

// lines is the list of lines a body holds, refused past mostBytes.
func lines(path string, body io.Reader) ([]string, error) {
	raw, err := io.ReadAll(io.LimitReader(body, mostBytes+onePast))
	if err != nil {
		return nil, fmt.Errorf("POST %s: reading the answer: %w", path, err)
	}
	if len(raw) > mostBytes {
		return nil, fmt.Errorf("POST %s answered with more than %d bytes",
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

// split is a line of CSV in its fields. DASCH's lines have no quoted
// field: daschlab, which reads them, splits at each comma.
func split(line string) []string {
	return strings.Split(line, ",")
}
