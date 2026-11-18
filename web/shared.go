package web

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"syscall"
	"time"
)

// lockPolls is how many times a request tries a record's lock while it
// waits for it. It's defined: the wait is cut in this many parts, and a
// lock let go in the first part is had at once.
const lockPolls = 20

// shares is whether the gentle keeps a shared record.
func (gentle *Gentle) shares() bool {
	gentle.guard.Lock()
	defer gentle.guard.Unlock()
	return gentle.sharing != ""
}

// behind is whether a shared record lacks a note the gentle holds.
func (gentle *Gentle) behind() bool {
	gentle.guard.Lock()
	defer gentle.guard.Unlock()
	return gentle.sharing != "" && gentle.unwritten
}

// share has the gentle keep a shared record at path. A record that
// cannot be read as a record refuses, since it may hold a stop. A folder
// that cannot be made, or a record that cannot be opened, is said once,
// and the gentle goes on with its pace in memory.
func (gentle *Gentle) share(path string) error {
	gentle.guard.Lock()
	defer gentle.guard.Unlock()
	if err := os.MkdirAll(filepath.Dir(path),
		recordFolderMode); err != nil {
		gentle.unkept(path, err)
		return nil
	}
	gentle.sharing = path
	return gentle.reload()
}

// say writes a line to the writer the gentle was given for saying things,
// or else the one it insists by, once; with neither it is quiet. The
// guard is held.
func (gentle *Gentle) say(line string) {
	if gentle.told[line] {
		return
	}
	if gentle.told == nil {
		gentle.told = map[string]bool{}
	}
	gentle.told[line] = true
	say := gentle.speaks
	if say == nil {
		say = gentle.insisting
	}
	if say != nil {
		fmt.Fprintln(say, line)
	}
}

// unkept says that a record cannot be kept, which path and why, and that
// the pace is kept for this run only. The guard is held.
func (gentle *Gentle) unkept(path string, why error) {
	gentle.say(fmt.Sprintf("%s: the record %s cannot be kept (%v); the "+
		"pace is kept for this run only, and no wait will be kept "+
		"between runs", gentle.policy.Host, path, why))
}

// lockShared takes the lock beside the shared record, waiting no longer
// than the policy says for another run to let go of it. It gives the lock
// to close, or nil when it could not be had, which it says once; the
// request goes ahead on what this run knows.
//
// It gives an error only when the context ends first. The guard is not
// held, so none waits on it.
func (gentle *Gentle) lockShared(ctx context.Context) (*os.File, error) {
	gentle.guard.Lock()
	path, wait := gentle.sharing+lockSuffix, gentle.policy.LockWait
	gentle.guard.Unlock()
	lock, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY, recordMode)
	if err == nil {
		err = gentle.waitFor(ctx, lock, wait)
	}
	if err == nil {
		return lock, nil
	}
	if lock != nil {
		lock.Close()
	}
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	gentle.guard.Lock()
	defer gentle.guard.Unlock()
	gentle.say(fmt.Sprintf("%s: could not lock %s (%v); going ahead on "+
		"what this run knows", gentle.policy.Host, path, err))
	return nil, nil
}

// waitFor takes the lock on a file, trying again a part of the wait later
// until the wait has passed on the gentle's clock.
func (gentle *Gentle) waitFor(ctx context.Context, lock *os.File,
	wait time.Duration) error {
	deadline := gentle.clock.Now().Add(wait)
	for {
		taken := syscall.Flock(int(lock.Fd()),
			syscall.LOCK_EX|syscall.LOCK_NB)
		if taken == nil {
			return nil
		}
		if !gentle.clock.Now().Before(deadline) {
			return fmt.Errorf("another run held it for %v: %w",
				wait, taken)
		}
		if err := gentle.clock.Sleep(ctx, max(wait/lockPolls,
			time.Nanosecond)); err != nil {
			return err
		}
	}
}

// beginShared is a request's start against a shared record: under its
// lock, the stops and the wait out of the pace worked out from the newest
// of what any run wrote, and this request's start written; then the lock
// is let go, and the pace is slept without it.
func (gentle *Gentle) beginShared(ctx context.Context) error {
	lock, err := gentle.lockShared(ctx)
	if err != nil {
		return err
	}
	wait, err := gentle.startShared(lock != nil)
	if lock != nil {
		lock.Close()
	}
	if err != nil {
		return err
	}
	if wait > noWait {
		if err := gentle.clock.Sleep(ctx, wait); err != nil {
			return err
		}
	}
	gentle.guard.Lock()
	defer gentle.guard.Unlock()
	gentle.asked++
	return nil
}

// startShared works out a request's wait from the record just read, when
// the lock was had, and writes its start, which is when the wait ends.
// It gives the stop instead when the host may not be asked.
func (gentle *Gentle) startShared(locked bool) (time.Duration, error) {
	gentle.guard.Lock()
	defer gentle.guard.Unlock()
	if locked {
		if err := gentle.reload(); err != nil {
			return noWait, err
		}
	}
	now := gentle.clock.Now()
	if refused := gentle.refused(now); refused != nil {
		return noWait, refused
	}
	// a last that a clock set wrong puts ahead is waited no more than the
	// pace
	wait := max(min(gentle.last.Add(gentle.policy.Pace).Sub(now),
		gentle.policy.Pace), noWait)
	gentle.last = now.Add(wait)
	gentle.keepShared(locked)
	return wait, nil
}

// noteShared writes a stop or an override the gentle holds that the
// record does not, under the record's lock. Without the lock the note
// stays in memory, and the next request or Release tries again.
func (gentle *Gentle) noteShared() {
	lock, _ := gentle.lockShared(context.Background())
	gentle.guard.Lock()
	defer gentle.guard.Unlock()
	if lock == nil {
		gentle.keepShared(false)
		return
	}
	defer lock.Close()
	if err := gentle.reload(); err != nil {
		gentle.say(err.Error())
		return
	}
	gentle.keepShared(true)
}

// keepShared writes the shared record when the lock was had. A record that
// will not write is said once, and the gentle goes on in memory. The guard
// is held.
func (gentle *Gentle) keepShared(locked bool) {
	if !locked {
		gentle.unwritten = true
		return
	}
	err := writeRecord(gentle.sharing, kept{last: gentle.last,
		until: gentle.until, askedFor: gentle.askedFor,
		overrides: gentle.overrides})
	gentle.unwritten = err != nil
	if err != nil {
		gentle.unkept(gentle.sharing, err)
	}
}

// reload reads the shared record into the gentle: the newest start, the
// wait any run asked for, and every override any run kept. A wait this
// gentle holds that the record does not is kept, for it is newer.
//
// A record that is not a record refuses; one that will not read is said
// once and left. The guard is held.
func (gentle *Gentle) reload() error {
	read, err := readRecord(gentle.sharing)
	var refused *RecordRefused
	if errors.As(err, &refused) {
		return err
	}
	if err != nil {
		gentle.unkept(gentle.sharing, err)
		return nil
	}
	if read.last.After(gentle.last) {
		gentle.last = read.last
	}
	if !gentle.unwritten {
		gentle.until, gentle.askedFor = read.until, read.askedFor
	}
	gentle.overrides = joined(read.overrides, gentle.overrides)
	gentle.trim()
	return nil
}

// joined is the overrides of two lists, each once, the oldest first.
func joined(first, second []override) []override {
	var both []override
	for _, one := range slices.Concat(first, second) {
		if !slices.ContainsFunc(both, one.same) {
			both = append(both, one)
		}
	}
	slices.SortStableFunc(both, func(a, b override) int {
		return a.when.Compare(b.when)
	})
	return both
}

// same is whether two overrides are one, however their times are zoned.
func (one override) same(other override) bool {
	return one.when.Equal(other.when) && one.until.Equal(other.until) &&
		one.asked == other.asked
}
