package web

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"time"
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

// The names of a record's lines. Last and until are a name, a space and
// a time, and are the two lines a record has always had.
//
// Asked is a name and, as the host wrote it, the Retry-After that gave
// the until; it is written only when the host asked for one.
//
// An overrode line is a name, the time it was said, the wait it
// overrode, and what the host asked for, as it wrote it, or one of the
// two words below.
const (
	lastLine     = "last"
	untilLine    = "until"
	askedLine    = "asked"
	overrodeLine = "overrode"
)

// circuitAsked stands in an overrode line where the host's Retry-After
// goes, when what was overridden was our own circuit. It's defined: the
// record's word for it.
const circuitAsked = "circuit"

// noneAsked stands in an overrode line when the host named no Retry-After.
// It's defined: the record's word for it.
const noneAsked = "none"

// override is one time a caller asked anyway: when, the time it was told
// to leave the host alone until, and what the host's Retry-After said, or
// one of the record's words for none and for the circuit.
type override struct {
	when, until time.Time
	asked       string
}

// kept is what a record holds: when the last request began, any time
// the host asked to be left until and what its Retry-After said, and the
// overrides, oldest first.
type kept struct {
	last, until time.Time
	askedFor    string
	overrides   []override
}

// RecordPath is where a host's record is kept in a folder.
func RecordPath(folder, host string) string {
	return filepath.Join(folder, host+"-gate")
}

// RecordRefused is why a record may not be held: another run holds it, or
// what is there is not a record. Any other failure of Hold, a folder or a
// file that will not open, is no refusal, and is given as it came. Find it
// with errors.As.
type RecordRefused struct {
	Why string
}

// Error is why.
func (refused *RecordRefused) Error() string { return refused.Why }

// Hold has the gentle keep its pace and its waits across runs, for this
// user on this machine. Its record at path holds when the last request
// began, any time the host asked to wait until, what its Retry-After
// said, and each override.
//
// The policy's Keeping says how. A shared record, the default, is locked
// only while a request begins and while a stop or an override is noted,
// so any number of runs share one pace and one stop, and none is
// refused. A record held for the run is locked from here until Release.
//
// Either refuses a record it cannot read as a record. A shared record
// that cannot be made, locked or written does not refuse: the gentle
// says so once, to the writer it was given for saying things, and keeps
// its pace in memory for the run.
//
// A record held for the run gives those as errors, as they came.
func (gentle *Gentle) Hold(path string) error {
	if gentle.policy.Keeping == KeepsForTheRun {
		return gentle.holdForTheRun(path)
	}
	return gentle.share(path)
}

// holdForTheRun keeps the record in a file read when it is held, written
// after each request begins, so a run that is stopped keeps the pace it
// had.
//
// A lock on a file beside the record stops a second run from asking
// while this one is. The system lets the lock go when the run ends,
// however it ends; Release writes the record and lets it go too.
//
// It refuses while another run holds the lock, and a record it cannot
// read.
func (gentle *Gentle) holdForTheRun(path string) error {
	gentle.guard.Lock()
	defer gentle.guard.Unlock()
	folder := filepath.Dir(path)
	if err := os.MkdirAll(folder, recordFolderMode); err != nil {
		return err
	}
	lock, err := os.OpenFile(path+lockSuffix, os.O_CREATE|os.O_WRONLY,
		recordMode)
	if err != nil {
		return err
	}
	held := syscall.Flock(int(lock.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
	if held != nil {
		lock.Close()
		return &RecordRefused{Why: fmt.Sprintf("another run is asking "+
			"%s: it holds %s (%v)", gentle.policy.Host, lock.Name(),
			held)}
	}
	read, err := readRecord(path)
	if err != nil {
		lock.Close()
		return err
	}
	gentle.record, gentle.lock = path, lock
	gentle.last, gentle.until = read.last, read.until
	gentle.askedFor, gentle.overrides = read.askedFor, read.overrides
	gentle.trim()
	return nil
}

// Release writes the record, so the next run keeps the pace from this
// one's last request, waits as long as the host asked, and knows what was
// overridden, and lets the lock go. A gentle that holds no record has
// nothing to release.
//
// A shared record is written as it goes, so there is nothing to let go;
// only a note that could not be written then, for want of the lock, is
// tried once more.
func (gentle *Gentle) Release() error {
	if gentle.behind() {
		gentle.noteShared()
	}
	gentle.guard.Lock()
	defer gentle.guard.Unlock()
	if gentle.lock == nil {
		return nil
	}
	written := gentle.note()
	// closing the lock's file is what lets the lock go
	closed := gentle.lock.Close()
	gentle.lock = nil
	return errors.Join(written, closed)
}

// note writes the record, when the gentle holds one. The guard is held.
func (gentle *Gentle) note() error {
	if gentle.lock == nil {
		return nil
	}
	return writeRecord(gentle.record, kept{last: gentle.last,
		until: gentle.until, askedFor: gentle.askedFor,
		overrides: gentle.overrides})
}

// readRecord is what a record holds. No record is none: a run that has
// never asked. A blank line is ignored. A line of a name it does not know
// is ignored when it ends in a time it can read; any other line it cannot
// read, whatever its name, refuses the record.
func readRecord(path string) (kept, error) {
	var read kept
	file, err := os.Open(path)
	if errors.Is(err, os.ErrNotExist) {
		return read, nil
	}
	if err != nil {
		return read, err
	}
	defer file.Close()
	lines := bufio.NewScanner(file)
	for lines.Scan() {
		if strings.TrimSpace(lines.Text()) == "" {
			continue
		}
		if err := read.take(lines.Text()); err != nil {
			return kept{}, &RecordRefused{Why: fmt.Sprintf("%s is "+
				"not the gate's record: %v. Remove it and run "+
				"again", path, err)}
		}
	}
	return read, lines.Err()
}

// take reads one line of a record into what is kept.
func (read *kept) take(line string) error {
	name, written, _ := strings.Cut(line, " ")
	if name == askedLine {
		read.askedFor = written
		return nil
	}
	if name == overrodeLine {
		one, err := readOverride(written)
		read.overrides = append(read.overrides, one)
		return err
	}
	at, err := time.Parse(time.RFC3339Nano, written)
	switch {
	case err != nil:
		return err
	case name == lastLine:
		read.last = at
	case name == untilLine:
		read.until = at
	}
	return nil
}

// readOverride reads what follows the name on an overrode line: two
// times, then what the host asked as it wrote it, which may hold spaces.
func readOverride(written string) (override, error) {
	var one override
	parts := strings.SplitN(written, " ", overrodeParts)
	if len(parts) != overrodeParts {
		return one, errors.New("an overrode line has three parts")
	}
	when, whenErr := time.Parse(time.RFC3339Nano, parts[0])
	until, untilErr := time.Parse(time.RFC3339Nano, parts[1])
	return override{when: when, until: until, asked: parts[2]},
		errors.Join(whenErr, untilErr)
}

// overrodeParts is how many parts follow the name on an overrode line.
// It's defined by the line's shape: when, until, and what was asked.
const overrodeParts = 3

// writeRecord writes what is kept into a part beside the record, makes
// sure it is on disk, then puts the part in its place, so a record is
// never half written, and a crash does not leave an empty one.
func writeRecord(path string, what kept) error {
	var text strings.Builder
	fmt.Fprintf(&text, "%s %s\n%s %s\n", lastLine,
		what.last.Format(time.RFC3339Nano), untilLine,
		what.until.Format(time.RFC3339Nano))
	if what.askedFor != "" {
		fmt.Fprintf(&text, "%s %s\n", askedLine, what.askedFor)
	}
	for _, one := range what.overrides {
		fmt.Fprintf(&text, "%s %s %s %s\n", overrodeLine,
			one.when.Format(time.RFC3339Nano),
			one.until.Format(time.RFC3339Nano), one.asked)
	}
	part := path + ".part"
	var file *os.File
	for _, step := range []func() error{
		func() (err error) {
			file, err = os.OpenFile(part, os.O_WRONLY|os.O_CREATE|
				os.O_TRUNC, recordMode)
			return err
		},
		func() error {
			_, err := file.WriteString(text.String())
			return err
		},
		func() error { return file.Sync() },
		func() error { return file.Close() },
		func() error { return os.Rename(part, path) },
	} {
		if err := step(); err != nil {
			if file != nil {
				file.Close()
			}
			return err
		}
	}
	return nil
}
