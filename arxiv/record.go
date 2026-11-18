package arxiv

import (
	"errors"

	"github.com/janearc/celestial-data"
	"github.com/janearc/celestial-data/web"
)

// Hold has the gate keep its pace across runs, for this user on this
// machine, and any wait arXiv asked for, in a record at path. The
// record is written after each request begins, so a run that is stopped
// keeps the pace it had; Release writes it again and lets the lock go.
//
// It refuses while another run holds the lock, and a record it cannot
// read. See web.Gentle.Hold, which keeps the record. Those two are
// refusals; a folder or file that will not open is given as it came.
func (gate *Gate) Hold(path string) error {
	err := gate.gentle.Hold(path)
	var refused *web.RecordRefused
	if errors.As(err, &refused) {
		return celestialdata.Refuse("%v", err)
	}
	return err
}

// Release writes the record again and lets the lock go, so the next run
// may ask. A gate that holds no record has nothing to write.
func (gate *Gate) Release() error {
	return gate.gentle.Release()
}
