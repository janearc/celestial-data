package arxiv

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/url"
	"sync"

	"github.com/janearc/celestial-data"
	"github.com/janearc/celestial-data/web"
)

// nothingCame is the length beside an error. It's defined by web.Web:
// the error says what happened, and no bytes came.
const nothingCame = 0

// paced is a web whose every request goes through a gate.
type paced struct {
	ctx  context.Context
	get  web.Web
	gate *Gate
}

// Paced is a web whose every request goes through the gate, waiting
// there while ctx lasts. One goes at a time, each begun at least the
// gate's pace after the last began. A Get holds the gate until its body
// is closed, so a slow download is still the only request.
//
// It refuses a missing context, web or gate.
func Paced(ctx context.Context, get web.Web, gate *Gate) (web.Web,
	error) {
	if ctx == nil || get == nil || gate == nil {
		return nil, celestialdata.Refuse("a paced web needs a " +
			"context, a web and a gate")
	}
	return paced{ctx: ctx, get: get, gate: gate}, nil
}

// Get is the web's Get, through the gate. The gate is held until the
// body is closed, or let go at once when there is none. A 429 or 503 is
// arXiv asking us to wait, and Get says so, with the wait asked.
func (pacing paced) Get(address string) (io.ReadCloser, int64, error) {
	if err := pacing.gate.enter(pacing.ctx); err != nil {
		return nil, nothingCame, err
	}
	body, length, err := pacing.get.Get(address)
	if err != nil {
		err = pacing.halted(err)
		pacing.gate.leave()
		return nil, length, err
	}
	return &held{body: body, gate: pacing.gate}, length, nil
}

// Head is the web's Head, through the gate. A 429 or 503 refusal is
// returned as Get would.
func (pacing paced) Head(address string) (int64, error) {
	if err := pacing.gate.enter(pacing.ctx); err != nil {
		return nothingCame, err
	}
	defer pacing.gate.leave()
	length, err := pacing.get.Head(address)
	return length, pacing.halted(err)
}

// halted says a refusal of 403, 429 or 503 as arXiv asking us to stop,
// with the request refused and the wait asked, and has the gate keep
// that wait. Any other error, or none, is given as it is. A wait whose
// record will not write is said beside the halt.
func (pacing paced) halted(err error) error {
	var status *web.Status
	if !errors.As(err, &status) || !stopping(status.Code()) {
		return err
	}
	noted := pacing.gate.stopUntil(status.RetryAfter())
	stop := &halt{status: status.Error(), retryAfter: status.RetryAfter()}
	return errors.Join(stop, noted)
}

// stopping is whether a status's code is arXiv asking us to stop.
func stopping(code int) bool {
	switch code {
	case http.StatusForbidden, http.StatusTooManyRequests,
		http.StatusServiceUnavailable:
		return true
	}
	return false
}

// Post is the web's Post, through the gate.
func (pacing paced) Post(address string, form url.Values) (string,
	error) {
	if err := pacing.gate.enter(pacing.ctx); err != nil {
		return "", err
	}
	defer pacing.gate.leave()
	return pacing.get.Post(address, form)
}

// held is a body that holds the gate until it is closed, once.
type held struct {
	body io.ReadCloser
	gate *Gate
	once sync.Once
}

// Read reads the body.
func (holding *held) Read(into []byte) (int, error) {
	return holding.body.Read(into)
}

// Close closes the body, then lets the gate go, so the next request
// begins after this one's connection is done. Closing again closes the
// body again and lets go of nothing.
func (holding *held) Close() error {
	err := holding.body.Close()
	holding.once.Do(holding.gate.leave)
	return err
}
