//go:build fragile

package netcdf_test

// An encoder of classic netCDF headers, written from the grammar of the
// NetCDF Classic Format Specification and not from the reader's own
// writer: a second writer is itself a check. Every field has an honest
// value and may be set to a lie, so a case states only its lie.

import (
	"encoding/binary"
	"fmt"
)

// The tags of the grammar's lists, and the marker that says the record
// count is not stored.
const (
	tagDimension    uint32 = 10
	tagVariable     uint32 = 11
	tagAttribute    uint32 = 12
	tagAbsent       uint32 = 0
	streamingMarker        = 0xFFFFFFFF
)

// The three versions, by the number of their version byte.
const (
	cdfOne  = 1
	cdfTwo  = 2
	cdfFive = 5
)

// The type tags of the grammar, in the order of the specification.
const (
	wireByte uint32 = 1 + iota
	wireChar
	wireShort
	wireInt
	wireFloat
	wireDouble
	wireUByte
	wireUShort
	wireUInt
	wireInt64
	wireUInt64
)

// paddingWidth is the boundary names and values are padded to.
const paddingWidth = 4

// fileCap is the most an encoded file is padded to with zeros.
const fileCap = 1 << 22

// wireWidths are the bytes one value of each type takes.
var wireWidths = map[uint32]int{wireByte: 1, wireChar: 1, wireShort: 2,
	wireInt: 4, wireFloat: 4, wireDouble: 8, wireUByte: 1, wireUShort: 2,
	wireUInt: 4, wireInt64: 8, wireUInt64: 8}

// wireWidth is the width of a type, or one for a tag that is no type, so
// that a lie about a type still has a count.
func wireWidth(tag uint32) int { return max(1, wireWidths[tag]) }

// some is a pointer to a value, to say a field is a lie.
func some[kind any](it kind) *kind { return &it }

// dimSpec is one dimension. A length of zero is the unlimited one.
type dimSpec struct {
	Name       string
	Length     uint64
	NameLength *uint64
	Padding    []byte
}

// attrSpec is one attribute: its values as the bytes of the file.
type attrSpec struct {
	Name        string
	Type        uint32
	Values      []byte
	Count       *uint64
	Padding     []byte
	NameLength  *uint64
	NamePadding []byte
}

// varSpec is one variable. BeginShift moves the honest begin, to place
// a variable inside the one before it.
type varSpec struct {
	Name       string
	DimIDs     []uint64
	Rank       *uint64
	Attrs      []attrSpec
	AttrTag    *uint32
	AttrCount  *uint64
	Type       uint32
	VSize      *uint64
	Begin      *uint64
	BeginShift int64
	NameLength *uint64
}

// header is a classic header. Version says the widths of its fields; the
// version byte written is that unless VersionByte lies.
type header struct {
	Version     int
	Magic       string
	VersionByte *byte
	Records     uint64
	NumRecs     *uint64
	Dims        []dimSpec
	DimTag      *uint32
	DimCount    *uint64
	Attrs       []attrSpec
	AttrTag     *uint32
	AttrCount   *uint64
	Vars        []varSpec
	VarTag      *uint32
	VarCount    *uint64
}

// encoder appends the fields of a header, at the widths of its version.
type encoder struct {
	version int
	out     []byte
}

// word appends 32 bits.
func (e *encoder) word(x uint32) {
	e.out = binary.BigEndian.AppendUint32(e.out, x)
}

// long appends 64 bits.
func (e *encoder) long(x uint64) {
	e.out = binary.BigEndian.AppendUint64(e.out, x)
}

// size appends a size_t: 32 bits, or 64 in CDF-5.
func (e *encoder) size(x uint64) {
	if e.version == cdfFive {
		e.long(x)
		return
	}
	e.word(uint32(x))
}

// offset appends an offset: 32 bits in CDF-1, 64 after.
func (e *encoder) offset(x uint64) {
	if e.version == cdfOne {
		e.word(uint32(x))
		return
	}
	e.long(x)
}

// padded appends the padding after length bytes: zeros to the boundary,
// or the bytes given when they are a lie.
func (e *encoder) padded(length int, lie []byte) {
	if lie != nil {
		e.out = append(e.out, lie...)
		return
	}
	e.out = append(e.out, make([]byte, (paddingWidth-length%paddingWidth)%
		paddingWidth)...)
}

// name appends a length, the name and its padding.
func (e *encoder) name(text string, length *uint64, padding []byte) {
	declared := uint64(len(text))
	if length != nil {
		declared = *length
	}
	e.size(declared)
	e.out = append(e.out, text...)
	e.padded(len(text), padding)
}

// listHead appends a list's tag and count.
func (e *encoder) listHead(want uint32, length int, tag *uint32,
	count *uint64) {
	declaredTag, declaredCount := want, uint64(length)
	if length == 0 {
		declaredTag = tagAbsent
	}
	if tag != nil {
		declaredTag = *tag
	}
	if count != nil {
		declaredCount = *count
	}
	e.word(declaredTag)
	e.size(declaredCount)
}

// attribute appends one attribute.
func (e *encoder) attribute(each attrSpec) {
	e.name(each.Name, each.NameLength, each.NamePadding)
	e.word(each.Type)
	count := uint64(len(each.Values) / wireWidth(each.Type))
	if each.Count != nil {
		count = *each.Count
	}
	e.size(count)
	e.out = append(e.out, each.Values...)
	e.padded(len(each.Values), each.Padding)
}

// attributes appends an attribute list.
func (e *encoder) attributes(list []attrSpec, tag *uint32, count *uint64) {
	e.listHead(tagAttribute, len(list), tag, count)
	for _, each := range list {
		e.attribute(each)
	}
}

// variable appends one variable, with the begin and vsize that are
// honest unless the spec lies.
func (e *encoder) variable(each varSpec, begin, vsize uint64) {
	e.name(each.Name, each.NameLength, nil)
	rank := uint64(len(each.DimIDs))
	if each.Rank != nil {
		rank = *each.Rank
	}
	e.size(rank)
	for _, id := range each.DimIDs {
		e.size(id)
	}
	e.attributes(each.Attrs, each.AttrTag, each.AttrCount)
	e.word(each.Type)
	if each.VSize != nil {
		vsize = *each.VSize
	}
	e.size(vsize)
	if each.Begin != nil {
		begin = *each.Begin
	}
	e.offset(begin + uint64(each.BeginShift))
}

// placement is where the data of a header's variables honestly lies.
type placement struct {
	begins, sizes           []uint64
	recordBegin, recordSize uint64
	end                     uint64
}

// isRecord is whether a variable's first dimension is the unlimited one.
func (h header) isRecord(each varSpec) bool {
	if len(each.DimIDs) == 0 || each.DimIDs[0] >= uint64(len(h.Dims)) {
		return false
	}
	return h.Dims[each.DimIDs[0]].Length == 0
}

// dataSize is the bytes of one slab of a variable, wrapping silently when
// the lie in the dimensions is an overflow.
func (h header) dataSize(each varSpec) uint64 {
	product := uint64(1)
	for at, id := range each.DimIDs {
		if id >= uint64(len(h.Dims)) || (at == 0 && h.isRecord(each)) {
			continue
		}
		product *= h.Dims[id].Length
	}
	return product * uint64(wireWidth(each.Type))
}

// roundUp is a size to the next multiple of four.
func roundUp(size uint64) uint64 {
	return (size + paddingWidth - 1) &^ (paddingWidth - 1)
}

// place lays the variables out as the specification says: the fixed ones
// in order after the header, each padded, then the record variables, each
// slab padded unless there is only one.
func (h header) place(start uint64) placement {
	found := placement{begins: make([]uint64, len(h.Vars)),
		sizes: make([]uint64, len(h.Vars))}
	records, position := 0, start
	for _, each := range h.Vars {
		if h.isRecord(each) {
			records++
		}
	}
	for at, each := range h.Vars {
		if !h.isRecord(each) {
			found.begins[at] = position
			found.sizes[at] = roundUp(h.dataSize(each))
			position += found.sizes[at]
		}
	}
	found.recordBegin = position
	for at, each := range h.Vars {
		if h.isRecord(each) {
			slab := h.dataSize(each)
			if records > 1 {
				slab = roundUp(slab)
			}
			found.begins[at], found.sizes[at] = position, slab
			position += slab
			found.recordSize += slab
		}
	}
	found.end = found.recordBegin + h.Records*found.recordSize
	return found
}

// encode is the header's bytes, with the variables placed as given.
func (h header) encode(found placement) []byte {
	e := &encoder{version: h.Version}
	magic := h.Magic
	if magic == "" {
		magic = "CDF"
	}
	e.out = append(e.out, magic...)
	versionByte := byte(h.Version)
	if h.VersionByte != nil {
		versionByte = *h.VersionByte
	}
	e.out = append(e.out, versionByte)
	records := h.Records
	if h.NumRecs != nil {
		records = *h.NumRecs
	}
	e.size(records)
	e.listHead(tagDimension, len(h.Dims), h.DimTag, h.DimCount)
	for _, each := range h.Dims {
		e.name(each.Name, each.NameLength, each.Padding)
		e.size(each.Length)
	}
	e.attributes(h.Attrs, h.AttrTag, h.AttrCount)
	e.listHead(tagVariable, len(h.Vars), h.VarTag, h.VarCount)
	for at, each := range h.Vars {
		var begin, vsize uint64
		if at < len(found.begins) {
			// vsize is padded even for a lone record variable, whose
			// records are not (netCDF-C's single_rec fixture: vsize 4,
			// one byte a record).
			begin, vsize = found.begins[at], roundUp(found.sizes[at])
		}
		e.variable(each, begin, vsize)
	}
	return e.out
}

// Bytes is the header alone, its begin offsets honest.
func (h header) Bytes() []byte {
	first := h.encode(placement{})
	return h.encode(h.place(uint64(len(first))))
}

// File is the header followed by zeros as far as the data honestly
// reaches, to at most fileCap bytes.
func (h header) File() []byte {
	head := h.Bytes()
	end := min(h.place(uint64(len(head))).end, fileCap)
	if end > uint64(len(head)) {
		head = append(head, make([]byte, end-uint64(len(head)))...)
	}
	return head
}

// emptyHeader is a header with no list in it.
func emptyHeader(version int) header { return header{Version: version} }

// goodHeader is a valid header: one dimension, one global attribute and
// one integer variable over the dimension.
func goodHeader(version int) header {
	return header{Version: version,
		Dims: []dimSpec{{Name: "x", Length: 2}},
		Attrs: []attrSpec{{Name: "title", Type: wireChar,
			Values: []byte("fine")}},
		Vars: []varSpec{{Name: "v", DimIDs: []uint64{0}, Type: wireInt}}}
}

// goodRecords is a valid header with the unlimited dimension, a
// dimension of three and one float variable over both, holding count
// records.
func goodRecords(version int, count uint64) header {
	return header{Version: version, Records: count,
		Dims: []dimSpec{{Name: "t"}, {Name: "x", Length: 3}},
		Vars: []varSpec{{Name: "r", DimIDs: []uint64{0, 1},
			Type: wireFloat}}}
}

// fixedVariable is a header of one variable "v" of a type over
// dimensions of the lengths given.
func fixedVariable(version int, tag uint32, lengths ...uint64) header {
	head := emptyHeader(version)
	ids := make([]uint64, len(lengths))
	for at, length := range lengths {
		head.Dims = append(head.Dims, dimSpec{
			Name: fmt.Sprintf("d%d", at), Length: length})
		ids[at] = uint64(at)
	}
	head.Vars = []varSpec{{Name: "v", DimIDs: ids, Type: tag}}
	return head
}

// recordVariables is a header of count record variables over the
// unlimited dimension and one of a length, each of a type.
func recordVariables(version, count int, inner uint64, tag uint32) header {
	head := emptyHeader(version)
	head.Dims = []dimSpec{{Name: "t"}, {Name: "x", Length: inner}}
	for at := range count {
		head.Vars = append(head.Vars, varSpec{
			Name: fmt.Sprintf("r%d", at), DimIDs: []uint64{0, 1}, Type: tag})
	}
	return head
}
