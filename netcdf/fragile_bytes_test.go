//go:build fragile

package netcdf_test

// The instruments of the harness: a reader over bytes that are not
// there, a reader that ends a full read with io.EOF, and a run that
// reports a panic, a hang, the time and the bytes allocated.

import (
	"errors"
	"fmt"
	"io"
	"runtime"
	"runtime/debug"
	"strings"
	"sync"
	"time"

	"github.com/janearc/celestial-data"
	"github.com/janearc/celestial-data/netcdf"
)

// What every input is held to.
const (
	// timeLimit is the longest an input may take.
	timeLimit = time.Second
	// hangLimit is how long a run is waited for before it is a hang.
	hangLimit = 20 * time.Second
	// allocLimit is the bytes an input may make the reader allocate
	// beyond the input's own bytes.
	allocLimit = 64 << 20
	// framesShown is how many frames of a panic are reported.
	framesShown = 4
	// examplesKept is how many examples of a class are kept.
	examplesKept = 40
)

// part is bytes that are there: a run of the input at an offset.
type part struct {
	at   int64
	data []byte
}

// notThere is an io.ReaderAt of a stated size whose bytes are the parts
// it holds, and zeros elsewhere, so that a limit guarded by "does it fit
// in the file" is reached without a large file. It records each offset
// it was asked to read.
type notThere struct {
	size  int64
	parts []part
	lock  sync.Mutex
	asked []int64
}

// fromBytes is a notThere that is only its bytes.
func fromBytes(raw []byte) *notThere {
	return &notThere{size: int64(len(raw)), parts: []part{{data: raw}}}
}

// real is how many bytes are there.
func (there *notThere) real() int64 {
	total := int64(0)
	for _, each := range there.parts {
		total += int64(len(each.data))
	}
	return total
}

// source is the adapter's Source of it.
func (there *notThere) source(name string) netcdf.Source {
	return netcdf.Source{Name: name, At: there, Size: there.size}
}

// ReadAt reads as a file of that size would, short at the end with
// io.EOF, and answers nil for a read that ends exactly at the end.
func (there *notThere) ReadAt(into []byte, offset int64) (int, error) {
	there.lock.Lock()
	there.asked = append(there.asked, offset)
	there.lock.Unlock()
	if offset < 0 {
		return 0, errors.New("negative offset")
	}
	if offset >= there.size {
		return 0, io.EOF
	}
	count, short := int64(len(into)), false
	if count > there.size-offset {
		count, short = there.size-offset, true
	}
	clear(into[:count])
	for _, each := range there.parts {
		start := max(offset, each.at)
		end := min(offset+count, each.at+int64(len(each.data)))
		if start < end {
			copy(into[start-offset:end-offset], each.data[start-each.at:])
		}
	}
	if short {
		return int(count), io.EOF
	}
	return int(count), nil
}

// forgotten clears the offsets asked, and gives them.
func (there *notThere) forgotten() []int64 {
	there.lock.Lock()
	defer there.lock.Unlock()
	found := there.asked
	there.asked = nil
	return found
}

// eofAtEnd is an io.ReaderAt that answers io.EOF with a full read that
// ends at the end of the input, as io.ReaderAt allows.
type eofAtEnd struct {
	inner io.ReaderAt
	size  int64
}

// ReadAt is the inner read, with io.EOF where it ends at the end.
func (reader eofAtEnd) ReadAt(into []byte, offset int64) (int, error) {
	count, err := reader.inner.ReadAt(into, offset)
	if err == nil && offset+int64(count) == reader.size {
		err = io.EOF
	}
	return count, err
}

// outcome is what a run did.
type outcome struct {
	panicked  any
	stack     string
	elapsed   time.Duration
	allocated uint64
	timedOut  bool
}

// attempt runs work and reports a panic with its stack, the time it took
// and the bytes it allocated. A run that does not return in hangLimit is
// left running and reported as a hang.
func attempt(work func()) outcome {
	done := make(chan outcome, 1)
	var before runtime.MemStats
	runtime.ReadMemStats(&before)
	start := time.Now()
	go func() {
		var got outcome
		defer func() {
			if caught := recover(); caught != nil {
				got.panicked, got.stack = caught, string(debug.Stack())
			}
			got.elapsed = time.Since(start)
			var after runtime.MemStats
			runtime.ReadMemStats(&after)
			got.allocated = after.TotalAlloc - before.TotalAlloc
			done <- got
		}()
		work()
	}()
	select {
	case got := <-done:
		return got
	case <-time.After(hangLimit):
		return outcome{timedOut: true, elapsed: hangLimit}
	}
}

// frames are the first places of a panic's stack that are not the
// runtime's or the harness's.
func frames(stack string) string {
	var shown []string
	for _, line := range strings.Split(stack, "\n") {
		text := strings.TrimSpace(line)
		skip := strings.Contains(text, "/runtime/") ||
			strings.Contains(text, "/testing/") ||
			strings.Contains(text, "fragile_")
		if !strings.Contains(text, ".go:") || skip {
			continue
		}
		place, _, _ := strings.Cut(text, " +0x")
		shown = append(shown, place)
		if len(shown) == framesShown {
			break
		}
	}
	return strings.Join(shown, " <- ")
}

// problems are the ways an outcome broke the limits. real is the bytes
// of input that are there; allow is what the case may allocate beyond
// allocLimit, for a case that says why.
func (got outcome) problems(real int64, allow uint64) []string {
	var found []string
	if got.timedOut {
		return append(found, fmt.Sprintf("no result in %v: a hang", hangLimit))
	}
	if got.panicked != nil {
		found = append(found, fmt.Sprintf("panic: %v at %s", got.panicked,
			frames(got.stack)))
	}
	if got.elapsed > timeLimit {
		found = append(found, fmt.Sprintf("took %v, the limit is %v",
			got.elapsed, timeLimit))
	}
	extra := int64(got.allocated) - real
	if extra > int64(allocLimit+allow) {
		found = append(found, fmt.Sprintf("allocated %d bytes beyond "+
			"the input's %d, the limit is %d", extra, real,
			int64(allocLimit+allow)))
	}
	return found
}

// ledger is the count of what held and what broke, and the notes that
// are not breaks.
var ledger struct {
	sync.Mutex
	cases, held, broke       int
	mutants, mutantsBroke    int
	accepted, vague, untyped []string
	heavy                    []string
}

// settle counts a case, held when it has no problem.
func settle(problems []string) {
	ledger.Lock()
	defer ledger.Unlock()
	ledger.cases++
	if len(problems) == 0 {
		ledger.held++
		return
	}
	ledger.broke++
}

// note keeps a line in one of the ledger's lists, to a limit.
func note(list *[]string, line string) {
	ledger.Lock()
	defer ledger.Unlock()
	if len(*list) < examplesKept*examplesKept {
		*list = append(*list, line)
	}
}

// openKinds and readKinds are the errors of the reader and the adapter
// that a refusal may be found as.
var (
	openKinds = []error{netcdf.ErrNotNetCDF, netcdf.ErrCorrupt,
		netcdf.ErrOutOfRange, netcdf.ErrUnsupportedFormat}
	readKinds = []error{netcdf.ErrCorrupt, netcdf.ErrOutOfRange,
		netcdf.ErrTypeMismatch}
)

// isOneOf is whether an error is one of the kinds, or the adapter's own
// refusal.
func isOneOf(err error, kinds []error) bool {
	if errors.Is(err, celestialdata.Refused()) {
		return true
	}
	for _, kind := range kinds {
		if errors.Is(err, kind) {
			return true
		}
	}
	return false
}

// trial is what happened to an input that was opened and read.
type trial struct {
	opened, read bool
	signature    string
	untyped      []string
}

// exercise opens a source and reads every variable whole, and every
// field of it.
func exercise(source netcdf.Source) trial {
	var got trial
	file, err := netcdf.Read(source)
	if err != nil {
		if !isOneOf(err, openKinds) {
			got.untyped = append(got.untyped, "open: "+err.Error())
		}
		return got
	}
	defer file.Close()
	got.opened, got.read = true, true
	got.signature = signature(file)
	for _, variable := range file.Variables() {
		if _, err := variable.Values(); err != nil {
			got.read = false
			if !isOneOf(err, readKinds) {
				got.untyped = append(got.untyped, "read: "+err.Error())
			}
		}
		probe(variable)
	}
	for _, each := range file.Attributes() {
		touch(each)
	}
	for _, each := range file.Fields() {
		_, _ = each.All()
	}
	return got
}

// bigCount is a count no file holds.
const bigCount = 1 << 62

// probe asks a variable for slabs that are at its edges and past them,
// and for what an attribute says of itself. Whatever comes back, it must
// come back.
func probe(variable *netcdf.Variable) {
	shape := variable.Shape()
	last, ones := make([]int64, len(shape)), make([]int64, len(shape))
	negative, huge := make([]int64, len(shape)), make([]int64, len(shape))
	for at, each := range shape {
		last[at], ones[at] = max(each-1, 0), 1
		negative[at], huge[at] = -1, bigCount
	}
	for _, slab := range []netcdf.Slab{{Start: last, Count: ones},
		{Start: negative, Count: ones}, {Start: last, Count: huge},
		{Start: shape, Count: make([]int64, len(shape))}} {
		_, _ = variable.Slab(slab)
		_, _ = variable.NumbersSlab(slab)
	}
	_, _ = variable.Numbers()
	for _, each := range variable.Attributes() {
		touch(each)
	}
}

// touch asks an attribute for each of its forms.
func touch(each *netcdf.Attribute) {
	_ = each.Text()
	_ = each.Value()
	_, _ = each.Number()
	_ = each.Bytes()
	_ = each.Length()
}

// signature is everything the file says about itself but its data.
func signature(file *netcdf.File) string {
	lines := []string{fmt.Sprintf("format %s records %d",
		netcdf.FormatName(file.Format()), file.NumRecs())}
	for _, each := range file.Dimensions() {
		lines = append(lines, fmt.Sprintf("dimension %q %d %t",
			each.Name(), each.Length(), each.Unlimited()))
	}
	for _, each := range file.Attributes() {
		lines = append(lines, attributeLine("global", each))
	}
	for _, each := range file.Variables() {
		lines = append(lines, fmt.Sprintf("variable %q %s %v %t",
			each.Name(), netcdf.TypeName(each.Type()), each.Shape(),
			each.Record()))
		for _, kept := range each.Attributes() {
			lines = append(lines, attributeLine(each.Name(), kept))
		}
	}
	return strings.Join(lines, "\n")
}

// signedBytes is how many bytes of an attribute a signature holds.
const signedBytes = 64

// attributeLine is one attribute of an owner, as a signature line: its
// length, and the first of its bytes.
func attributeLine(owner string, each *netcdf.Attribute) string {
	held := each.Bytes()
	return fmt.Sprintf("attribute %s %q %s %d %x", owner, each.Name(),
		netcdf.TypeName(each.Type()), len(held), held[:min(len(held),
			signedBytes)])
}

// firstDifference says where two signatures first part.
func firstDifference(was, now string) string {
	before, after := strings.Split(was, "\n"), strings.Split(now, "\n")
	for at := range max(len(before), len(after)) {
		left, right := "(none)", "(none)"
		if at < len(before) {
			left = before[at]
		}
		if at < len(after) {
			right = after[at]
		}
		if left != right {
			return fmt.Sprintf("was [%s] now [%s]", left, right)
		}
	}
	return "no difference"
}
