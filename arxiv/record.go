package arxiv

import (
	"bufio"
	"errors"
	"fmt"
	"math"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/janearc/celestial-data"
	"github.com/janearc/celestial-data/web"
)

// The modes of the record's folder and files. They're defined: read and
// written by their owner, read by others.
const (
	recordFolderMode = 0o755 // defined: a folder its owner writes in
	recordMode       = 0o644 // defined: a file its owner writes
)

// lockSuffix is what the lock beside a record is named by. It's
// defined: the record's name and .lock.
const lockSuffix = ".lock"

// The names of a record's two lines, each a name, a space and a time.
const (
	lastLine  = "last"
	untilLine = "until"
)

// restUnsaid is how long the gate leaves arXiv alone after it asked us
// to stop without saying how long.
func restUnsaid() time.Duration {
	return web.Seconds(asks.Value("rest-unsaid"))
}

// Hold has the gate keep its pace across runs, for this user on this
// machine. Its record at path holds when the last request began, and
// any time arXiv asked it to wait until; it is written after each
// request, so a run that is stopped keeps the pace it had.
//
// A lock on a file beside the record stops a second run from asking
// while this one is. The system lets the lock go when the run ends,
// however it ends; Release writes the record and lets it go too.
//
// It refuses while another run holds the lock, and a record it cannot
// read.
func (gate *Gate) Hold(path string) error {
	folder := filepath.Dir(path)
	if err := os.MkdirAll(folder, recordFolderMode); err != nil {
		return err
	}
	lock, err := os.OpenFile(path+lockSuffix, os.O_CREATE|os.O_WRONLY,
		recordMode)
	if err != nil {
		return err
	}
	descriptor := int(lock.Fd())
	held := syscall.Flock(descriptor, syscall.LOCK_EX|syscall.LOCK_NB)
	if held != nil {
		lock.Close()
		return celestialdata.Refuse("another run is asking arXiv: it "+
			"holds %s (%v)", lock.Name(), held)
	}
	last, until, err := readRecord(path)
	if err != nil {
		lock.Close()
		return err
	}
	gate.record, gate.lock, gate.last, gate.until = path, lock, last,
		until
	return nil
}

// Release writes the record, so the next run keeps the pace from this
// one's last request and waits as long as arXiv asked, and lets the lock
// go. A gate that holds no record has nothing to release.
func (gate *Gate) Release() error {
	if gate.lock == nil {
		return nil
	}
	written := gate.note()
	// closing the lock's file is what lets the lock go
	closed := gate.lock.Close()
	gate.lock = nil
	return errors.Join(written, closed)
}

// note writes the record, when the gate holds one.
func (gate *Gate) note() error {
	if gate.lock == nil {
		return nil
	}
	return writeRecord(gate.record, gate.last, gate.until)
}

// readRecord is the times a record holds. No record is none: a run that
// has never asked.
func readRecord(path string) (last, until time.Time, err error) {
	file, err := os.Open(path)
	if errors.Is(err, os.ErrNotExist) {
		return last, until, nil
	}
	if err != nil {
		return last, until, err
	}
	defer file.Close()
	lines := bufio.NewScanner(file)
	for lines.Scan() {
		name, written, _ := strings.Cut(lines.Text(), " ")
		at, err := time.Parse(time.RFC3339Nano, written)
		switch {
		case err != nil:
			why := celestialdata.Refuse("%s is not the gate's "+
				"record: %v. Remove it and run again", path,
				err)
			return last, until, why
		case name == lastLine:
			last = at
		case name == untilLine:
			until = at
		}
	}
	return last, until, lines.Err()
}

// writeRecord writes the times into a part beside the record, then puts
// the part in its place, so a record is never half written.
func writeRecord(path string, last, until time.Time) error {
	text := fmt.Sprintf("%s %s\n%s %s\n", lastLine,
		last.Format(time.RFC3339Nano), untilLine,
		until.Format(time.RFC3339Nano))
	part := path + ".part"
	if err := os.WriteFile(part, []byte(text), recordMode); err != nil {
		return err
	}
	return os.Rename(part, path)
}

// stopUntil has the gate leave arXiv alone as long as a Retry-After
// asks: so many seconds, or until a date. With none it can read, the
// gate rests as long as rest-unsaid says.
//
// The wait is written to the record at once, while the run still holds
// it, so a run killed before Release still leaves it for the next; a
// record that will not write is said.
func (gate *Gate) stopUntil(retryAfter string) error {
	now := gate.clock.Now()
	until := now.Add(restUnsaid())
	if seconds, err := strconv.Atoi(retryAfter); err == nil &&
		seconds >= noSeconds {
		lasts := time.Duration(min(seconds, longestSeconds))
		until = now.Add(lasts * time.Second)
	} else if at, err := http.ParseTime(retryAfter); err == nil {
		until = at
	}
	// no request goes out while an earlier wait is left, so no stop
	// can come before it has passed: the wait asked now is the one
	gate.until = until
	return gate.note()
}

// noSeconds is a Retry-After of no wait. It's defined: none.
const noSeconds = 0

// longestSeconds is the longest Retry-After the gate keeps, in seconds.
// It's defined by time.Duration: the most seconds it holds, some 292
// years; a longer one is kept as this.
const longestSeconds = math.MaxInt64 / int(time.Second)
