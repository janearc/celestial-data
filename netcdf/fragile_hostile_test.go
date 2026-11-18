//go:build fragile

package netcdf_test

// Hostile headers, one case for each refusal of the reader, through the
// adapter. A case says what must come back: an error of a kind, a file
// that opens and reads, or a file that may open (the reader's choice,
// logged). Whatever it says, no panic, under a second, and under 64 MiB
// allocated beyond the input's own bytes.

import (
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/janearc/celestial-data"
	"github.com/janearc/celestial-data/netcdf"
)

// verdict is what a case says must come back.
type verdict int

// The three verdicts.
const (
	mustRefuse verdict = iota
	mustOpen
	mayOpen
)

// inputName is the name every case's source is given, so that a number in
// an error is the reader's and not the case's.
const inputName = "input"

// hostile is one input and what must come back.
type hostile struct {
	name  string
	raw   []byte
	head  *header
	size  int64
	kind  verdict
	want  []error
	after func(*netcdf.File) error
	later []error
	allow uint64
}

// versions are the three classic versions.
var versions = []int{cdfOne, cdfTwo, cdfFive}

// refuse is a header that must be refused as one of the kinds, corrupt
// when none is said.
func refuse(name string, head header, want ...error) hostile {
	if len(want) == 0 {
		want = []error{netcdf.ErrCorrupt}
	}
	return hostile{name: name, head: &head, kind: mustRefuse, want: want}
}

// refuseBytes is bytes that must be refused.
func refuseBytes(name string, raw []byte, want ...error) hostile {
	found := refuse(name, header{}, want...)
	found.head, found.raw = nil, raw
	return found
}

// accept is a header that must open.
func accept(name string, head header) hostile {
	return hostile{name: name, head: &head, kind: mustOpen}
}

// tolerate is a header the reader may open or refuse; it is logged when
// it opens, as a wrong file accepted.
func tolerate(name string, head header) hostile {
	return hostile{name: name, head: &head, kind: mayOpen}
}

// claiming gives the case a size the reader is told, beyond its bytes.
func (c hostile) claiming(size int64) hostile {
	c.size = size
	return c
}

// allowing lets the case allocate more than the limit, when the claimed
// size is what asks for it.
func (c hostile) allowing(extra uint64) hostile {
	c.allow = extra
	return c
}

// then gives the case a step to run on the opened file.
func (c hostile) then(step func(*netcdf.File) error) hostile {
	c.after = step
	return c
}

// failing says the step after opening must fail as one of the kinds.
func (c hostile) failing(want ...error) hostile {
	c.later = want
	return c
}

// onBytes gives the case the bytes of the file, in place of its header's.
func (c hostile) onBytes(raw []byte) hostile {
	c.raw = raw
	return c
}

// there is the input of the case.
func (c hostile) there() *notThere {
	raw := c.raw
	if raw == nil {
		raw = c.head.File()
	}
	there := fromBytes(raw)
	if c.size != 0 {
		there.size = c.size
	}
	return there
}

// readEverything reads every variable of a file whole.
func readEverything(file *netcdf.File) error {
	for _, variable := range file.Variables() {
		if _, err := variable.Values(); err != nil {
			return err
		}
	}
	return nil
}

// readValues reads one variable whole.
func readValues(name string) func(*netcdf.File) error {
	return func(file *netcdf.File) error {
		variable, err := file.Variable(name)
		if err != nil {
			return fmt.Errorf("harness: no variable %q", name)
		}
		_, err = variable.Values()
		return err
	}
}

// readSlab reads a hyperslab of one variable.
func readSlab(name string, start, count []int64) func(*netcdf.File) error {
	return func(file *netcdf.File) error {
		variable, err := file.Variable(name)
		if err != nil {
			return fmt.Errorf("harness: no variable %q", name)
		}
		_, err = variable.Slab(netcdf.Slab{Start: start, Count: count})
		return err
	}
}

// wantRecords checks the file's record count.
func wantRecords(count int64) func(*netcdf.File) error {
	return func(file *netcdf.File) error {
		if got := file.NumRecs(); got != count {
			return fmt.Errorf("harness: %d records, want %d", got, count)
		}
		return nil
	}
}

// both runs a step and then another, and gives the first error.
func both(first, second func(*netcdf.File) error) func(*netcdf.File) error {
	return func(file *netcdf.File) error {
		if err := first(file); err != nil {
			return err
		}
		return second(file)
	}
}

// sameAs checks that a file says and holds what the honest bytes do.
func sameAs(honest []byte) func(*netcdf.File) error {
	return func(file *netcdf.File) error {
		other, err := netcdf.Read(fromBytes(honest).source("honest"))
		if err != nil {
			return fmt.Errorf("harness: honest file: %v", err)
		}
		defer other.Close()
		if got, want := signature(file), signature(other); got != want {
			return fmt.Errorf("harness: %s", firstDifference(want, got))
		}
		for _, variable := range file.Variables() {
			mine, _ := variable.Values()
			twin, _ := other.Variable(variable.Name())
			theirs, _ := twin.Values()
			if !reflect.DeepEqual(mine, theirs) {
				return fmt.Errorf("harness: %q reads differently",
					variable.Name())
			}
		}
		return nil
	}
}

// vague is whether an error names no place and no thing and gives no
// number: a refusal that says nothing useful.
func vague(err error) bool {
	text := strings.ReplaceAll(err.Error(), inputName, "")
	return !strings.Contains(text, "offset") &&
		!strings.ContainsAny(text, `"0123456789`)
}

// judge says what is wrong with what came back from a case.
func (c hostile) judge(openErr, laterErr error) []string {
	switch {
	case c.kind == mustRefuse:
		return c.judgeRefusal(openErr)
	case openErr != nil && c.kind == mustOpen:
		return []string{"refused a file it must open: " + openErr.Error()}
	case openErr != nil:
		if isOneOf(openErr, openKinds) {
			return nil
		}
		return []string{"untyped refusal: " + openErr.Error()}
	case c.kind == mayOpen:
		note(&ledger.accepted, c.name)
	}
	return c.judgeLater(laterErr)
}

// judgeRefusal checks the error of a file that must be refused.
func (c hostile) judgeRefusal(openErr error) []string {
	if openErr == nil {
		return []string{"opened a file it must refuse"}
	}
	for _, kind := range c.want {
		if errors.Is(openErr, kind) {
			if vague(openErr) {
				note(&ledger.vague, c.name+": "+openErr.Error())
			}
			return nil
		}
	}
	return []string{fmt.Sprintf("refused as none of %v: %v", c.want, openErr)}
}

// judgeLater checks the step that ran on the opened file.
func (c hostile) judgeLater(laterErr error) []string {
	switch {
	case len(c.later) == 0 && laterErr != nil:
		return []string{"step after opening failed: " + laterErr.Error()}
	case len(c.later) == 0:
		return nil
	case laterErr == nil:
		return []string{"step after opening must fail and did not"}
	}
	for _, kind := range c.later {
		if errors.Is(laterErr, kind) {
			return nil
		}
	}
	return []string{fmt.Sprintf("step failed as none of %v: %v", c.later,
		laterErr)}
}

// What is worth a line in the notes, though it is no break.
const (
	// heavyNote is the bytes allocated that are worth a note.
	heavyNote = 8 << 20
	// slowNote is the time taken that is worth a note.
	slowNote = 200 * time.Millisecond
)

// remark notes what a case did that is worth knowing: a file it must
// refuse that opened, and how it opened; an input that took much time or
// memory.
func (c hostile) remark(got outcome, openErr error, opened string) {
	if c.kind == mustRefuse && openErr == nil {
		note(&ledger.accepted, "OPENED "+c.name+" AS "+strings.ReplaceAll(
			opened, "\n", " | "))
	}
	if got.allocated > heavyNote || got.elapsed > slowNote {
		note(&ledger.heavy, fmt.Sprintf("%s: %d bytes allocated, %v",
			c.name, got.allocated, got.elapsed))
	}
}

// runHostile runs one case and counts it.
func runHostile(t *testing.T, c hostile) {
	t.Helper()
	there := c.there()
	var openErr, laterErr error
	var opened string
	got := attempt(func() {
		file, err := netcdf.Read(there.source(inputName))
		if err != nil {
			openErr = err
			return
		}
		defer file.Close()
		if c.kind == mustRefuse {
			opened = signature(file)
		}
		if c.after != nil {
			laterErr = c.after(file)
		}
	})
	problems := got.problems(there.real(), c.allow)
	if !got.timedOut {
		problems = append(problems, c.judge(openErr, laterErr)...)
		c.remark(got, openErr, opened)
	}
	settle(problems)
	for _, each := range problems {
		t.Error(each)
	}
}

// Every refusal of the reader, and every file it must open.
func TestHostileHeaders(t *testing.T) {
	for _, each := range hostileCases() {
		t.Run(each.name, func(t *testing.T) { runHostile(t, each) })
	}
}

// hostileCases are all the cases, in groups.
func hostileCases() []hostile {
	var all []hostile
	for _, group := range [][]hostile{controlCases(), overflowCases(),
		limitCases(), rangeCases(), positionCases(), listCases(),
		dimensionCases(), typeCases(), magicCases(), shortCases(),
		streamingCases(), promiseCases(), ignoredCases(), shapeCases(),
		nameCases()} {
		all = append(all, group...)
	}
	return all
}

// mixedHeader has a fixed variable and a record variable, count records.
func mixedHeader(version int, count uint64) header {
	head := emptyHeader(version)
	head.Records = count
	head.Dims = []dimSpec{{Name: "x", Length: 2}, {Name: "t"}}
	head.Vars = []varSpec{
		{Name: "v", DimIDs: []uint64{0}, Type: wireInt},
		{Name: "r", DimIDs: []uint64{1, 0}, Type: wireInt}}
	return head
}

// controlCases are valid files of the shapes the other cases bend: the
// encoder and the reader must agree on these.
func controlCases() []hostile {
	var out []hostile
	for _, version := range versions {
		name := fmt.Sprintf("control/CDF-%d ", version)
		out = append(out,
			accept(name+"fixed", goodHeader(version)).then(readEverything),
			accept(name+"one record variable",
				goodRecords(version, 2)).then(both(wantRecords(2),
				readEverything)),
			accept(name+"two record variables",
				withRecords(recordVariables(version, 2, 4, wireInt), 2)).
				then(both(wantRecords(2), readEverything)),
			accept(name+"fixed and record",
				mixedHeader(version, 2)).then(both(wantRecords(2),
				readEverything)),
			accept(name+"no list at all",
				emptyHeader(version)).then(readEverything))
	}
	return out
}

// withRecords is a header holding a count of records.
func withRecords(head header, count uint64) header {
	head.Records = count
	return head
}

// overflowCases are the arithmetic the reader checks.
func overflowCases() []hostile {
	beyond := fixedVariable(cdfFive, wireByte, 1<<62)
	beyond.Vars[0].Begin = some(uint64(1<<63 - 16))
	records := goodRecords(cdfFive, 0)
	records.NumRecs = some(uint64(1 << 62))
	wide := recordVariables(cdfTwo, 1, 0xFFFFFFFF, wireByte)
	wide.NumRecs = some(uint64(0xFFFFFFFE))
	return []hostile{
		refuse("overflow/dimension product",
			fixedVariable(cdfFive, wireByte, 1<<40, 1<<40)),
		refuse("overflow/variable bytes",
			fixedVariable(cdfFive, wireDouble, 1<<61)),
		refuse("overflow/record size sum",
			recordVariables(cdfFive, 2, 1<<62, wireByte)),
		refuse("overflow/round up to four, fixed",
			fixedVariable(cdfFive, wireByte, 1<<63-2)),
		refuse("overflow/round up to four, record",
			recordVariables(cdfFive, 2, 1<<63-2, wireByte)),
		refuse("overflow/begin plus span", beyond),
		refuse("overflow/record count times record size, CDF-5", records),
		refuse("overflow/record count times record size, CDF-2", wide),
	}
}

// listLie sets the count of one of the three lists.
func listLie(head *header, which string, count uint64) {
	switch which {
	case "dimension":
		head.DimCount = some(count)
	case "attribute":
		head.AttrCount = some(count)
	default:
		head.VarCount = some(count)
	}
}

// attributeOf is a header whose one global attribute is of a type and
// declares a count of values it does not hold.
func attributeOf(version int, tag uint32, count uint64) header {
	head := goodHeader(version)
	head.Attrs = []attrSpec{{Name: "a", Type: tag, Count: some(count)}}
	return head
}

// limitCases are the limits that keep a header from an allocation.
func limitCases() []hostile {
	const claimed = 1 << 40
	longName := goodHeader(cdfTwo)
	longName.Dims[0].NameLength = some(uint64(64<<10 + 1))
	hugeName := goodHeader(cdfFive)
	hugeName.Dims[0].NameLength = some(uint64(1 << 62))
	atLimit := goodHeader(cdfTwo)
	atLimit.Dims[0].Name = strings.Repeat("n", 64<<10)
	out := []hostile{
		refuse("limit/attribute over 128 MiB, bytes",
			attributeOf(cdfTwo, wireByte, 128<<20+1)).claiming(claimed),
		refuse("limit/attribute over 128 MiB, doubles",
			attributeOf(cdfTwo, wireDouble, 128<<20/8+1)).claiming(claimed),
		refuse("limit/attribute of 1<<62 values",
			attributeOf(cdfFive, wireByte, 1<<62)).claiming(claimed),
		accept("limit/attribute at 128 MiB, held in memory",
			attributeOf(cdfTwo, wireByte, 128<<20)).claiming(claimed).
			allowing(256 << 20),
		refuse("limit/name over 64 KiB", longName).claiming(claimed),
		refuse("limit/name of 1<<62 bytes", hugeName).claiming(claimed),
		accept("limit/name at 64 KiB", atLimit).then(readEverything),
	}
	return append(out, listLimits(claimed)...)
}

// listLimits are the cases of the lists' and the rank's limits.
func listLimits(claimed int64) []hostile {
	var out []hostile
	for _, which := range []string{"dimension", "attribute", "variable"} {
		over, atLimit := goodHeader(cdfTwo), goodHeader(cdfTwo)
		listLie(&over, which, 1_000_001)
		listLie(&atLimit, which, 1_000_000)
		out = append(out,
			refuse("limit/over a million "+which+"s", over).
				claiming(claimed),
			refuse("limit/a million "+which+"s, claimed 1 GiB", atLimit).
				claiming(1<<30))
	}
	rank := func(count int) header {
		head := fixedVariable(cdfTwo, wireInt, 1)
		head.Vars[0].DimIDs = make([]uint64, count)
		return head
	}
	lie := rank(1)
	lie.Vars[0].Rank = some(uint64(1 << 40))
	lie.Version = cdfFive
	return append(out,
		refuse("limit/rank 1025", rank(1025)),
		accept("limit/rank 1024", rank(1024)).then(readEverything),
		refuse("limit/rank of 1<<40, CDF-5", lie).claiming(claimed))
}

// rangeCases are the values above what the format's integers hold.
func rangeCases() []hostile {
	const top = 1 << 63
	lies := []struct {
		name  string
		apply func(*header)
	}{
		{"dimension length", func(h *header) { h.Dims[0].Length = top }},
		{"variable begin", func(h *header) { h.Vars[0].Begin = some(uint64(top)) }},
		{"record count", func(h *header) { h.NumRecs = some(uint64(top)) }},
		{"record count all ones", func(h *header) {
			h.NumRecs = some(uint64(1<<64 - 1))
		}},
		{"dimension list count", func(h *header) { listLie(h, "dimension", top) }},
		{"attribute list count", func(h *header) { listLie(h, "attribute", top) }},
		{"variable list count", func(h *header) { listLie(h, "variable", top) }},
		{"name length", func(h *header) { h.Dims[0].NameLength = some(uint64(top)) }},
		{"rank", func(h *header) { h.Vars[0].Rank = some(uint64(top)) }},
		{"dimension id", func(h *header) { h.Vars[0].DimIDs[0] = top }},
		{"attribute value count", func(h *header) {
			h.Attrs[0].Count = some(uint64(top))
		}},
		{"vsize", func(h *header) { h.Vars[0].VSize = some(uint64(top)) }},
	}
	var out []hostile
	for _, lie := range lies {
		head := goodHeader(cdfFive)
		lie.apply(&head)
		out = append(out, refuse("range/CDF-5 "+lie.name+" above int64", head))
	}
	begin := goodHeader(cdfTwo)
	begin.Vars[0].Begin = some(uint64(top))
	last := goodHeader(cdfOne)
	last.Vars[0].Begin = some(uint64(0x7FFFFFFF))
	return append(out,
		refuse("range/CDF-2 begin above int64", begin),
		refuse("range/CDF-1 begin 0x80000000", withBegin(cdfOne, 0x80000000)),
		refuse("range/CDF-1 begin 0xFFFFFFFF", withBegin(cdfOne, 0xFFFFFFFF)),
		accept("range/CDF-1 begin 0x7FFFFFFF, past the file", last).
			then(readEverything).failing(netcdf.ErrCorrupt))
}

// withBegin is a good header whose variable begins where it is told.
func withBegin(version int, begin uint64) header {
	head := goodHeader(version)
	head.Vars[0].Begin = some(begin)
	return head
}

// positionCases are variables placed where they cannot be.
func positionCases() []hostile {
	twoFixed := func(version int, shift int64) header {
		head := emptyHeader(version)
		head.Dims = []dimSpec{{Name: "x", Length: 2}}
		head.Vars = []varSpec{
			{Name: "a", DimIDs: []uint64{0}, Type: wireInt},
			{Name: "b", DimIDs: []uint64{0}, Type: wireInt, BeginShift: shift}}
		return head
	}
	var out []hostile
	for _, version := range versions {
		name := fmt.Sprintf("position/CDF-%d ", version)
		overlap := recordVariables(version, 2, 4, wireInt)
		overlap.Vars[1].BeginShift = -4
		unordered := recordVariables(version, 2, 4, wireInt)
		unordered.Vars[0].BeginShift = 32
		early := mixedHeader(version, 0)
		early.Vars[1].BeginShift = -4
		out = append(out,
			refuse(name+"begins at 0", withBegin(version, 0)),
			refuse(name+"begins inside the header", withBegin(version, 4)),
			refuse(name+"before the previous variable's end",
				twoFixed(version, -4)),
			refuse(name+"record variable overlaps the one before", overlap),
			refuse(name+"record variables out of order", unordered),
			refuse(name+"record section before the fixed end", early))
	}
	return out
}

// tagLie sets the tag of one of the three lists.
func tagLie(head *header, which string, tag uint32) {
	switch which {
	case "dimension":
		head.DimTag = some(tag)
	case "attribute":
		head.AttrTag = some(tag)
	default:
		head.VarTag = some(tag)
	}
}

// ownTags are the tags of the lists, by name.
var ownTags = map[string]uint32{"dimension": tagDimension,
	"attribute": tagAttribute, "variable": tagVariable}

// listCases are lists with the wrong tag or a count that cannot be true.
func listCases() []hostile {
	var out []hostile
	for _, version := range versions {
		for which, own := range ownTags {
			name := fmt.Sprintf("list/CDF-%d %s ", version, which)
			for _, tag := range []uint32{0, 1, 13, 0xFFFFFFFF, own ^ 1} {
				empty := emptyHeader(version)
				tagLie(&empty, which, tag)
				full := goodHeader(version)
				tagLie(&full, which, tag)
				out = append(out, refuse(fmt.Sprintf("%slist, tag %d",
					name, tag), full))
				if tag != tagAbsent {
					out = append(out, refuse(fmt.Sprintf(
						"%sempty list, tag %d", name, tag), empty))
				}
			}
			own := emptyHeader(version)
			tagLie(&own, which, ownTags[which])
			fits, near := goodHeader(version), goodHeader(version)
			listLie(&fits, which, 1000)
			listLie(&near, which, 2)
			out = append(out,
				tolerate(name+"empty list with its own tag", own),
				refuse(name+"count that cannot fit", fits),
				refuse(name+"count of one too many", near))
		}
	}
	return out
}

// dimensionCases are the unlimited dimension in the wrong place, and
// dimension ids that are not there.
func dimensionCases() []hostile {
	var out []hostile
	for _, version := range versions {
		name := fmt.Sprintf("dimension/CDF-%d ", version)
		notFirst := emptyHeader(version)
		notFirst.Dims = []dimSpec{{Name: "x", Length: 3}, {Name: "t"}}
		notFirst.Vars = []varSpec{{Name: "v", DimIDs: []uint64{0, 1},
			Type: wireInt}}
		twice := emptyHeader(version)
		twice.Dims = []dimSpec{{Name: "t"}, {Name: "u"}}
		twice.Vars = []varSpec{{Name: "v", DimIDs: []uint64{0}, Type: wireInt}}
		out = append(out,
			refuse(name+"unlimited not first", notFirst),
			refuse(name+"two unlimited", twice))
		for _, id := range []uint64{1, 0xFFFFFFFF} {
			head := goodHeader(version)
			head.Vars[0].DimIDs[0] = id
			out = append(out, refuse(fmt.Sprintf("%sdimension id %d", name,
				id), head))
		}
	}
	far := goodHeader(cdfFive)
	far.Vars[0].DimIDs[0] = 1 << 40
	return append(out, refuse("dimension/CDF-5 dimension id 1<<40", far))
}

// typeCases are type tags that are no type, and types a version lacks.
func typeCases() []hostile {
	var out []hostile
	for _, version := range versions {
		for _, tag := range []uint32{0, 12, 255, 257, 0xFFFFFFFF} {
			out = append(out, refuseType(version, tag)...)
		}
	}
	for tag := wireUByte; tag <= wireUInt64; tag++ {
		for _, version := range versions {
			if version == cdfFive {
				out = append(out, acceptType(version, tag)...)
				continue
			}
			out = append(out, refuseType(version, tag)...)
		}
	}
	return out
}

// typedHeaders are a header whose global attribute is of a tag and one
// whose variable is.
func typedHeaders(version int, tag uint32) (attribute, variable header) {
	attribute, variable = goodHeader(version), goodHeader(version)
	attribute.Attrs = []attrSpec{{Name: "a", Type: tag,
		Values: make([]byte, 2*wireWidth(tag))}}
	variable.Vars[0].Type = tag
	return attribute, variable
}

// refuseType is the two cases of a tag that must be refused.
func refuseType(version int, tag uint32) []hostile {
	attribute, variable := typedHeaders(version, tag)
	name := fmt.Sprintf("type/CDF-%d tag %d ", version, tag)
	return []hostile{refuse(name+"in an attribute", attribute),
		refuse(name+"in a variable", variable)}
}

// acceptType is the two cases of a tag that must open.
func acceptType(version int, tag uint32) []hostile {
	attribute, variable := typedHeaders(version, tag)
	name := fmt.Sprintf("type/CDF-%d tag %d ", version, tag)
	return []hostile{
		accept(name+"in an attribute", attribute).then(readEverything),
		accept(name+"in a variable", variable).then(readEverything)}
}

// magicCases are bytes that are not a classic file by their first four.
func magicCases() []hostile {
	var out []hostile
	for _, version := range []byte{0, 3, 4, 6, 255} {
		head := goodHeader(cdfOne)
		head.VersionByte = some(version)
		out = append(out, refuse(fmt.Sprintf("magic/version byte %d",
			version), head, netcdf.ErrNotNetCDF))
	}
	for _, magic := range []string{"CDG", "cdf", "HDF", "\x00\x00\x00"} {
		head := goodHeader(cdfOne)
		head.Magic = magic
		out = append(out, refuse(fmt.Sprintf("magic/%q", magic), head,
			netcdf.ErrNotNetCDF))
	}
	signature := []byte("\x89HDF\r\n\x1a\n")
	return append(out, refuseBytes("magic/NetCDF-4",
		append(signature, make([]byte, 100)...),
		netcdf.ErrUnsupportedFormat))
}

// shortCases are inputs of nought to sixteen bytes of a good file.
func shortCases() []hostile {
	var out []hostile
	for _, version := range versions {
		file := goodHeader(version).File()
		for length := range 17 {
			out = append(out, refuseBytes(fmt.Sprintf(
				"short/CDF-%d, %d bytes", version, length), file[:length],
				netcdf.ErrNotNetCDF, netcdf.ErrCorrupt))
		}
	}
	return out
}

// streamingCases are files whose record count is the marker.
func streamingCases() []hostile {
	var out []hostile
	for _, version := range []int{cdfOne, cdfTwo} {
		name := fmt.Sprintf("streaming/CDF-%d ", version)
		none := goodHeader(version)
		none.NumRecs = some(uint64(streamingMarker))
		short := goodRecords(version, 0)
		short.NumRecs = some(uint64(streamingMarker))
		beyond := goodRecords(version, 0)
		beyond.NumRecs = some(uint64(streamingMarker))
		beyond.Vars[0].BeginShift = 4096
		half := goodRecords(version, 2)
		half.NumRecs = some(uint64(streamingMarker))
		out = append(out,
			accept(name+"with no record variable", none).
				then(both(wantRecords(0), readEverything)),
			accept(name+"with no records in the file", short).
				then(both(wantRecords(0), readEverything)),
			accept(name+"file shorter than its record section", beyond).
				then(wantRecords(0)),
			accept(name+"two and a half records", half).
				onBytes(append(half.File(), make([]byte, 6)...)).
				then(both(wantRecords(2), readEverything)))
	}
	five := goodHeader(cdfFive)
	five.NumRecs = some(uint64(streamingMarker))
	return append(out, tolerate("streaming/CDF-5 marker is a count", five))
}

// promiseCases are files whose header promises more records than the
// file holds: they open, and the last record cannot be read.
func promiseCases() []hostile {
	var out []hostile
	for _, version := range versions {
		name := fmt.Sprintf("promise/CDF-%d ", version)
		more := goodRecords(version, 2)
		more.NumRecs = some(uint64(1000))
		huge := goodRecords(version, 2)
		huge.NumRecs = some(uint64(0xFFFFFFF0))
		cut := goodRecords(version, 3)
		out = append(out,
			accept(name+"a thousand records, two there", more).
				then(readSlab("r", []int64{999, 0}, []int64{1, 3})).
				failing(netcdf.ErrCorrupt),
			accept(name+"four billion records, read whole", huge).
				then(readValues("r")).
				failing(netcdf.ErrCorrupt, celestialdata.Refused()),
			accept(name+"last record cut short", cut).
				onBytes(cut.File()[:len(cut.File())-3]).
				then(readSlab("r", []int64{2, 0}, []int64{1, 3})).
				failing(netcdf.ErrCorrupt),
			accept(name+"data cut short", goodHeader(version)).
				onBytes(goodHeader(version).File()[:len(goodHeader(
					version).File())-4]).
				then(readValues("v")).failing(netcdf.ErrCorrupt))
	}
	return out
}

// ignoredCases are fields the reader does not hold the file to: the
// vsize it recomputes, and padding it skips.
func ignoredCases() []hostile {
	var out []hostile
	for _, version := range versions {
		honest := goodHeader(version).File()
		for _, vsize := range []uint64{0, 1, 0xFFFFFFFF} {
			head := goodHeader(version)
			head.Vars[0].VSize = some(vsize)
			out = append(out, accept(fmt.Sprintf(
				"ignored/CDF-%d vsize %#x", version, vsize), head).
				then(sameAs(honest)))
		}
		out = append(out, paddingCases(version)...)
	}
	return out
}

// paddingCases are names and values whose padding is not zeros, or not
// the width it must be.
func paddingCases(version int) []hostile {
	name := fmt.Sprintf("ignored/CDF-%d ", version)
	var out []hostile
	for label, padding := range map[string][]byte{"nonzero": {1, 2, 3},
		"too short": {}, "too long": make([]byte, 7)} {
		dimension := goodHeader(version)
		dimension.Dims[0].Padding = padding
		attribute := goodHeader(version)
		attribute.Attrs = []attrSpec{{Name: "a", Type: wireChar,
			Values: []byte("abc"), Padding: padding}}
		out = append(out,
			tolerate(name+label+" padding after a name", dimension),
			tolerate(name+label+" padding after a value", attribute))
	}
	count := goodHeader(version)
	count.Attrs[0].Count = some(uint64(0))
	more := goodHeader(version)
	more.Attrs[0].Count = some(uint64(5))
	return append(out,
		tolerate(name+"attribute declares none, holds four", count),
		tolerate(name+"attribute declares five, holds four", more))
}

// shapeCases are shapes that fit the integers and not the file.
func shapeCases() []hostile {
	two := fixedVariable(cdfTwo, wireByte, 0xFFFFFFFF)
	five := fixedVariable(cdfFive, wireByte, 1<<62)
	wideRecord := goodRecords(cdfFive, 0)
	wideRecord.Dims[1].Length = 1 << 40
	return []hostile{
		accept("shape/CDF-2 dimension 0xFFFFFFFF, read whole", two).
			then(readValues("v")).
			failing(netcdf.ErrCorrupt, celestialdata.Refused()),
		accept("shape/CDF-2 dimension 0xFFFFFFFF, last two", two).
			then(readSlab("v", []int64{0xFFFFFFFD}, []int64{2})).
			failing(netcdf.ErrCorrupt),
		accept("shape/CDF-5 dimension 1<<62, read whole", five).
			then(readValues("v")).
			failing(netcdf.ErrCorrupt, celestialdata.Refused()),
		accept("shape/CDF-5 dimension 1<<62, one value", five).
			then(readSlab("v", []int64{1 << 61}, []int64{1})).
			failing(netcdf.ErrCorrupt),
		accept("shape/CDF-5 record of 1<<40 values, none asked", wideRecord).
			then(readSlab("r", []int64{0, 1 << 39}, []int64{0, 1})),
	}
}

// nameCases are names the writer refuses and the reader may accept.
func nameCases() []hostile {
	var out []hostile
	for _, bad := range []string{"", "\xff\xfe", "a/b", "tab\there", " x", "x "} {
		head := goodHeader(cdfTwo)
		head.Vars[0].Name = bad
		out = append(out, tolerate(fmt.Sprintf("name/variable %q", bad),
			head))
	}
	twice := goodHeader(cdfTwo)
	twice.Vars = append(twice.Vars, twice.Vars[0])
	twiceDimension := goodHeader(cdfTwo)
	twiceDimension.Dims = append(twiceDimension.Dims, twiceDimension.Dims[0])
	twiceAttribute := goodHeader(cdfTwo)
	twiceAttribute.Attrs = append(twiceAttribute.Attrs, twiceAttribute.Attrs[0])
	return append(out,
		tolerate("name/variable twice", twice),
		tolerate("name/dimension twice", twiceDimension),
		tolerate("name/attribute twice", twiceAttribute))
}
