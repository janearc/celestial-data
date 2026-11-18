//go:build fragile

package netcdf_test

// A small random model of a classic file and the generators of its parts.
// A model has three renderings: written through the adapter's writer,
// encoded by the harness's own encoder (header and data, placed by the
// grammar), and compared with a file that was read. The property tests
// draw a model and ask the two writers and the two readers about it.

import (
	"errors"
	"fmt"
	"math"
	"slices"
	"strings"

	"pgregory.net/rapid"

	"github.com/janearc/celestial-data/netcdf"
)

// The bounds of a drawn model.
const (
	// maxDimensions is the most dimensions a model has.
	maxDimensions = 4
	// maxLength is the longest a dimension is, and maxRecords the most
	// records the unlimited one holds.
	maxLength, maxRecords = 6, 6
	// maxVariables is the most variables a model has.
	maxVariables = 5
	// maxRank is the most dimensions a variable runs over.
	maxRank = 3
	// maxAttributes is the most attributes the file, or a variable, has.
	maxAttributes = 3
	// maxValues is the most values an attribute holds.
	maxValues = 5
	// maxInside is the most runes of a name between its first and last.
	maxInside = 5
	// fillFraction is one variable in this many is given a _FillValue.
	fillFraction = 3
)

// fillName is the attribute that overrides a variable's fill value.
const fillName = "_FillValue"

// modelDim is a dimension of a model. A length of zero is the unlimited
// one, which is always the first.
type modelDim struct {
	Name   string
	Length int64
}

// modelAttr is an attribute: its values as a slice of the Go type of its
// netCDF type.
type modelAttr struct {
	Name   string
	Type   netcdf.Type
	Values any
}

// modelVar is a variable of a model, over the dimensions at the indices
// in Dims, which only rise, so that the unlimited one is first.
type modelVar struct {
	Name   string
	Type   netcdf.Type
	Dims   []int
	Attrs  []modelAttr
	Values any
}

// fragModel is a file: its format, the records it holds, its dimensions,
// its global attributes and its variables with their values.
type fragModel struct {
	Format  netcdf.Format
	Records int64
	Dims    []modelDim
	Attrs   []modelAttr
	Vars    []modelVar
}

// modelWants says what a property needs of the model drawn.
type modelWants struct {
	// dims is the fewest dimensions.
	dims int
	// vars is the fewest variables.
	vars int
	// ranked asks that the first variable has a dimension and data.
	ranked bool
	// record asks that the first variable runs over the unlimited
	// dimension, which holds at least one record.
	record bool
	// fills lets variables be given a _FillValue.
	fills bool
}

// nameFirst are the runes a name begins with, and nameInside those it may
// hold after that. A name never ends in a space. The non-ASCII runes are
// all in their normal form, so that no writer need change them.
var (
	nameFirst  = []rune("abcxyzABCXYZ019_é名ж\U0001F600")
	nameInside = []rune("abcxyz019_.- é名ж")
)

// nameGenerator is names the format allows.
func nameGenerator() *rapid.Generator[string] {
	return rapid.Custom(func(t *rapid.T) string {
		name := string(rapid.SampledFrom(nameFirst).Draw(t, "first rune"))
		inside := rapid.SliceOfN(rapid.SampledFrom(nameInside), 0,
			maxInside).Draw(t, "inside runes")
		if len(inside) == 0 {
			return name
		}
		last := rapid.SampledFrom(nameFirst).Draw(t, "last rune")
		return name + string(inside) + string(last)
	})
}

// drawNames is count names, none twice.
func drawNames(t *rapid.T, label string, count int) []string {
	return rapid.SliceOfNDistinct(nameGenerator(), count, count,
		func(name string) string { return name }).Draw(t, label)
}

// drawSlice is count values of a generator.
func drawSlice[kind any](t *rapid.T, each *rapid.Generator[kind], count int,
	label string) []kind {
	return rapid.SliceOfN(each, count, count).Draw(t, label)
}

// drawValues is count values of a netCDF type, as a slice of its Go type.
func drawValues(t *rapid.T, typ netcdf.Type, count int, label string) any {
	switch typ {
	case netcdf.TypeByte:
		return drawSlice(t, rapid.Int8(), count, label)
	case netcdf.TypeShort:
		return drawSlice(t, rapid.Int16(), count, label)
	case netcdf.TypeInt:
		return drawSlice(t, rapid.Int32(), count, label)
	case netcdf.TypeFloat:
		return drawSlice(t, rapid.Float32(), count, label)
	case netcdf.TypeDouble:
		return drawSlice(t, rapid.Float64(), count, label)
	case netcdf.TypeUShort:
		return drawSlice(t, rapid.Uint16(), count, label)
	case netcdf.TypeUInt:
		return drawSlice(t, rapid.Uint32(), count, label)
	case netcdf.TypeInt64:
		return drawSlice(t, rapid.Int64(), count, label)
	case netcdf.TypeUInt64:
		return drawSlice(t, rapid.Uint64(), count, label)
	}
	return drawSlice(t, rapid.Byte(), count, label)
}

// drawAttributes is up to maxAttributes attributes of types a format has.
func drawAttributes(t *rapid.T, label string,
	types []netcdf.Type) []modelAttr {
	count := rapid.IntRange(0, maxAttributes).Draw(t, label+" count")
	var found []modelAttr
	for _, name := range drawNames(t, label+" names", count) {
		typ := rapid.SampledFrom(types).Draw(t, label+" type")
		size := rapid.IntRange(0, maxValues).Draw(t, label+" size")
		found = append(found, modelAttr{name, typ,
			drawValues(t, typ, size, label+" values")})
	}
	return found
}

// drawModel is a random model that has what the property wants.
func drawModel(t *rapid.T, want modelWants) fragModel {
	format := rapid.SampledFrom(formatsOf).Draw(t, "format")
	types := typesOf(format)
	needs := want.ranked || want.record
	low := max(want.dims, boolToInt(needs))
	count := rapid.IntRange(low, maxDimensions).Draw(t, "dimension count")
	model := fragModel{Format: format}
	unlimited := want.record || (count > 0 &&
		rapid.Bool().Draw(t, "has an unlimited dimension"))
	for at, name := range drawNames(t, "dimension names", count) {
		length := int64(rapid.IntRange(1, maxLength).Draw(t, "length"))
		if at == 0 && unlimited {
			length = 0
		}
		model.Dims = append(model.Dims, modelDim{name, length})
	}
	model.Attrs = drawAttributes(t, "global attribute", types)
	model.Vars = drawVariables(t, model, types, want)
	model.Records = int64(rapid.IntRange(boolToInt(needs),
		maxRecords).Draw(t, "records"))
	if !slices.ContainsFunc(model.Vars, model.record) {
		model.Records = 0
	}
	model.valued(t)
	return model
}

// boolToInt is one for true and none for false.
func boolToInt(yes bool) int {
	if yes {
		return 1
	}
	return 0
}

// drawVariables is the variables of a model, without their values.
func drawVariables(t *rapid.T, model fragModel, types []netcdf.Type,
	want modelWants) []modelVar {
	count := rapid.IntRange(max(want.vars, boolToInt(want.ranked ||
		want.record)), maxVariables).Draw(t, "variable count")
	var found []modelVar
	for at, name := range drawNames(t, "variable names", count) {
		each := modelVar{Name: name,
			Type: rapid.SampledFrom(types).Draw(t, "variable type")}
		each.Dims = drawDims(t, model, at == 0, want)
		each.Attrs = drawAttributes(t, "variable attribute", types)
		if want.fills && rapid.IntRange(1, fillFraction).Draw(t,
			"fill roll") == 1 {
			fill := drawValues(t, each.Type, 1, "fill value")
			each.Attrs = append([]modelAttr{{fillName, each.Type, fill}},
				each.Attrs...)
		}
		found = append(found, each)
	}
	return found
}

// drawDims is the dimensions a variable runs over, in rising order. The
// first variable runs over the unlimited one when the property says so.
func drawDims(t *rapid.T, model fragModel, first bool,
	want modelWants) []int {
	limit := min(maxRank, len(model.Dims))
	low := boolToInt(first && (want.ranked || want.record))
	rank := rapid.IntRange(low, limit).Draw(t, "rank")
	var chosen []int
	from := 0
	if first && want.record {
		chosen, from, rank = []int{0}, 1, rank-1
	}
	if rank > 0 {
		more := rapid.SliceOfNDistinct(rapid.IntRange(from,
			len(model.Dims)-1), rank, rank, func(at int) int { return at }).
			Draw(t, "dimensions")
		chosen = append(chosen, more...)
	}
	slices.Sort(chosen)
	return chosen
}

// valued draws the values of every variable, once the records are known.
func (model *fragModel) valued(t *rapid.T) {
	for at := range model.Vars {
		each := &model.Vars[at]
		count := int(countOf(model.shape(*each)))
		each.Values = drawValues(t, each.Type, count, "values")
	}
}

// record is whether a variable runs over the unlimited dimension.
func (model fragModel) record(each modelVar) bool {
	return len(each.Dims) > 0 && model.Dims[each.Dims[0]].Length == 0
}

// shape is the lengths a variable has, the records counted for the
// unlimited dimension.
func (model fragModel) shape(each modelVar) []int64 {
	found := make([]int64, len(each.Dims))
	for at, id := range each.Dims {
		found[at] = model.Dims[id].Length
		if found[at] == 0 {
			found[at] = model.Records
		}
	}
	return found
}

// countOf is the values a shape holds.
func countOf(shape []int64) int64 {
	total := int64(1)
	for _, each := range shape {
		total *= each
	}
	return total
}

// String is a model in a line or two, for a failure to say what it was.
func (model fragModel) String() string {
	var out strings.Builder
	fmt.Fprintf(&out, "%s records %d dims", netcdf.FormatName(model.Format),
		model.Records)
	for _, each := range model.Dims {
		fmt.Fprintf(&out, " %q=%d", each.Name, each.Length)
	}
	fmt.Fprintf(&out, "; global %s", attributesText(model.Attrs))
	for _, each := range model.Vars {
		fmt.Fprintf(&out, "; var %q %s over %v shape %v %s", each.Name,
			netcdf.TypeName(each.Type), each.Dims, model.shape(each),
			attributesText(each.Attrs))
	}
	return out.String()
}

// attributesText is attributes in a few words.
func attributesText(list []modelAttr) string {
	var found []string
	for _, each := range list {
		found = append(found, fmt.Sprintf("%q %s %v", each.Name,
			netcdf.TypeName(each.Type), each.Values))
	}
	return "[" + strings.Join(found, ", ") + "]"
}

// bitsOf is values as the bits they are stored in, so that a NaN is
// compared by its bits and a signed value by its width.
func bitsOf(values any) []uint64 {
	switch each := values.(type) {
	case []int8:
		return widenBits(each, func(x int8) uint64 { return uint64(uint8(x)) })
	case []byte:
		return widenBits(each, func(x byte) uint64 { return uint64(x) })
	case []int16:
		return widenBits(each, func(x int16) uint64 {
			return uint64(uint16(x))
		})
	case []uint16:
		return widenBits(each, func(x uint16) uint64 { return uint64(x) })
	case []int32:
		return widenBits(each, func(x int32) uint64 {
			return uint64(uint32(x))
		})
	case []uint32:
		return widenBits(each, func(x uint32) uint64 { return uint64(x) })
	case []int64:
		return widenBits(each, func(x int64) uint64 { return uint64(x) })
	case []uint64:
		return each
	case []float32:
		return widenBits(each, func(x float32) uint64 {
			return uint64(math.Float32bits(x))
		})
	case []float64:
		return widenBits(each, math.Float64bits)
	}
	panic(fmt.Sprintf("bitsOf: no bits for %T", values))
}

// widenBits is a list with each of its values changed by one.
func widenBits[from any](list []from, one func(from) uint64) []uint64 {
	found := make([]uint64, len(list))
	for at, each := range list {
		found[at] = one(each)
	}
	return found
}

// encodeValues is values as the file stores them: big-endian, each as
// wide as its type.
func encodeValues(typ netcdf.Type, values any) []byte {
	width := wireWidth(uint32(typ))
	var found []byte
	for _, each := range bitsOf(values) {
		for shift := width - 1; shift >= 0; shift-- {
			found = append(found, byte(each>>(8*shift)))
		}
	}
	return found
}

// versionOf is the number of the version byte of a format.
func versionOf(format netcdf.Format) int {
	switch format {
	case netcdf.FormatClassic64:
		return cdfTwo
	case netcdf.FormatClassic64Data:
		return cdfFive
	}
	return cdfOne
}

// specsOf is attributes as the encoder's specs.
func specsOf(list []modelAttr) []attrSpec {
	var found []attrSpec
	for _, each := range list {
		found = append(found, attrSpec{Name: each.Name, Type: uint32(each.Type),
			Values: encodeValues(each.Type, each.Values)})
	}
	return found
}

// header is the model as the harness's encoder takes it.
func (model fragModel) header() header {
	head := header{Version: versionOf(model.Format),
		Records: uint64(model.Records), Attrs: specsOf(model.Attrs)}
	for _, each := range model.Dims {
		head.Dims = append(head.Dims, dimSpec{Name: each.Name,
			Length: uint64(each.Length)})
	}
	for _, each := range model.Vars {
		ids := make([]uint64, len(each.Dims))
		for at, id := range each.Dims {
			ids[at] = uint64(id)
		}
		head.Vars = append(head.Vars, varSpec{Name: each.Name, DimIDs: ids,
			Attrs: specsOf(each.Attrs), Type: uint32(each.Type)})
	}
	return head
}

// image is the whole file the grammar says the model is: the header, and
// each variable's values where the placement puts them, the padding zero.
func (model fragModel) image() []byte {
	head := model.header()
	first := head.Bytes()
	found := head.place(uint64(len(first)))
	raw := make([]byte, max(found.end, uint64(len(first))))
	copy(raw, first)
	for _, each := range model.regions(found) {
		copy(raw[each.At:], each.Data)
	}
	return raw
}

// region is bytes that a file holds at an offset: the values of a
// variable, or of one record of it.
type region struct {
	Name string
	At   uint64
	Data []byte
}

// regions are where each variable's values lie in a file, a fixed
// variable in one, a record variable in one for each record.
func (model fragModel) regions(found placement) []region {
	var all []region
	for at, each := range model.Vars {
		data := encodeValues(each.Type, each.Values)
		if !model.record(each) {
			all = append(all, region{each.Name, found.begins[at], data})
			continue
		}
		size := len(data) / int(max(model.Records, 1))
		for record := range int(model.Records) {
			begin := found.begins[at] + uint64(record)*found.recordSize
			all = append(all, region{fmt.Sprintf("%s record %d", each.Name,
				record), begin, data[record*size : (record+1)*size]})
		}
	}
	return all
}

// layout is the model as the adapter's layout.
func (model fragModel) layout() (*netcdf.Layout, error) {
	layout, err := netcdf.NewLayout(model.Format)
	if err != nil {
		return nil, err
	}
	var dims []*netcdf.LayoutDimension
	for _, each := range model.Dims {
		made, err := layout.AddDimension(each.Name, each.Length)
		if err != nil {
			return nil, err
		}
		dims = append(dims, made)
	}
	for _, each := range model.Attrs {
		if err := layout.SetTyped(each.Name, each.Type,
			each.Values); err != nil {
			return nil, err
		}
	}
	for _, each := range model.Vars {
		if err := addVariable(layout, dims, each); err != nil {
			return nil, err
		}
	}
	return layout, nil
}

// addVariable adds a variable and its attributes to a layout.
func addVariable(layout *netcdf.Layout, dims []*netcdf.LayoutDimension,
	each modelVar) error {
	var over []*netcdf.LayoutDimension
	for _, at := range each.Dims {
		over = append(over, dims[at])
	}
	made, err := layout.AddVariable(netcdf.VariableSpec{Name: each.Name,
		Type: each.Type, Dimensions: over})
	if err != nil {
		return err
	}
	for _, attr := range each.Attrs {
		if err := made.SetTyped(attr.Name, attr.Type, attr.Values); err != nil {
			return err
		}
	}
	return nil
}

// create makes the file of the model at a path and gives its writer,
// nothing yet written.
func (model fragModel) create(path string) (*netcdf.Writer, error) {
	layout, err := model.layout()
	if err != nil {
		return nil, err
	}
	return layout.Create(path)
}

// fill writes every variable of the model whole, in the order of the
// model, but those that hold no values.
func (model fragModel) fill(writer *netcdf.Writer) error {
	for _, each := range model.Vars {
		if countOf(model.shape(each)) == 0 {
			continue
		}
		target, err := writer.Variable(each.Name)
		if err != nil {
			return err
		}
		if err := target.Write(each.Values); err != nil {
			return err
		}
	}
	return nil
}

// save writes the whole model to a path and closes it.
func (model fragModel) save(path string) error {
	writer, err := model.create(path)
	if err != nil {
		return err
	}
	err = model.fill(writer)
	return errors.Join(err, writer.Close())
}

// differs is the first way a file that was read is not the model, or
// nothing when it is the model.
func (model fragModel) differs(file *netcdf.File) string {
	if file.Format() != model.Format || file.NumRecs() != model.Records {
		return fmt.Sprintf("format %s records %d, want %s records %d",
			netcdf.FormatName(file.Format()), file.NumRecs(),
			netcdf.FormatName(model.Format), model.Records)
	}
	if line := model.dimensionsDiffer(file); line != "" {
		return line
	}
	if line := attributesDiffer("global", model.Attrs,
		file.Attributes()); line != "" {
		return line
	}
	return model.variablesDiffer(file)
}

// dimensionsDiffer is the first dimension that is not the model's.
func (model fragModel) dimensionsDiffer(file *netcdf.File) string {
	got := file.Dimensions()
	if len(got) != len(model.Dims) {
		return fmt.Sprintf("%d dimensions, want %d", len(got),
			len(model.Dims))
	}
	for at, want := range model.Dims {
		length := want.Length
		if length == 0 {
			length = model.Records
		}
		if got[at].Name() != want.Name || got[at].Length() != length ||
			got[at].Unlimited() != (want.Length == 0) {
			return fmt.Sprintf("dimension %d is %q %d %t, want %q %d %t", at,
				got[at].Name(), got[at].Length(), got[at].Unlimited(),
				want.Name, length, want.Length == 0)
		}
	}
	return ""
}

// attributesDiffer is the first attribute of an owner that is not as
// the model has it.
func attributesDiffer(owner string, want []modelAttr,
	got []*netcdf.Attribute) string {
	if len(got) != len(want) {
		return fmt.Sprintf("%s has %d attributes, want %d", owner, len(got),
			len(want))
	}
	for at, each := range want {
		held := got[at]
		size := len(bitsOf(each.Values))
		if held.Name() != each.Name || held.Type() != each.Type ||
			held.Length() != size {
			return fmt.Sprintf("%s attribute %d is %q %s %d values, want %q "+
				"%s %d", owner, at, held.Name(), netcdf.TypeName(held.Type()),
				held.Length(), each.Name, netcdf.TypeName(each.Type), size)
		}
		if size > 0 && !slices.Equal(bitsOf(held.Value()),
			bitsOf(each.Values)) {
			return fmt.Sprintf("%s attribute %q holds %v, want %v", owner,
				each.Name, held.Value(), each.Values)
		}
	}
	return ""
}

// variablesDiffer is the first variable that is not the model's.
func (model fragModel) variablesDiffer(file *netcdf.File) string {
	got := file.Variables()
	if len(got) != len(model.Vars) {
		return fmt.Sprintf("%d variables, want %d", len(got), len(model.Vars))
	}
	for at, want := range model.Vars {
		if line := model.variableDiffers(got[at], want); line != "" {
			return fmt.Sprintf("variable %q: %s", want.Name, line)
		}
	}
	return ""
}

// variableDiffers is the first way a variable is not the model's.
func (model fragModel) variableDiffers(got *netcdf.Variable,
	want modelVar) string {
	shape := model.shape(want)
	if got.Name() != want.Name || got.Type() != want.Type ||
		got.Record() != model.record(want) || !slices.Equal(got.Shape(),
		shape) {
		return fmt.Sprintf("is %q %s %v record %t, want %s %v record %t",
			got.Name(), netcdf.TypeName(got.Type()), got.Shape(),
			got.Record(), netcdf.TypeName(want.Type), shape, model.record(want))
	}
	for at, each := range got.Dimensions() {
		if each.Name() != model.Dims[want.Dims[at]].Name {
			return fmt.Sprintf("dimension %d is %q", at, each.Name())
		}
	}
	if line := attributesDiffer(want.Name, want.Attrs,
		got.Attributes()); line != "" {
		return line
	}
	return valuesDiffer(got, want, shape)
}

// valuesDiffer is whether a variable reads as the values of the model.
func valuesDiffer(got *netcdf.Variable, want modelVar, shape []int64) string {
	if countOf(shape) == 0 {
		return ""
	}
	values, err := got.Values()
	if err != nil {
		return fmt.Sprintf("cannot be read: %v", err)
	}
	if !slices.Equal(bitsOf(values), bitsOf(want.Values)) {
		return fmt.Sprintf("reads %v, want %v", values, want.Values)
	}
	return ""
}

// defaultFill is the fill value of each type, as the bits it is stored
// in, from the specification's table and not from the reader's.
var defaultFill = map[netcdf.Type]uint64{
	netcdf.TypeByte:   0x81,   // -127
	netcdf.TypeChar:   0,      // NUL
	netcdf.TypeShort:  0x8001, // -32767
	netcdf.TypeInt:    0x80000001,
	netcdf.TypeFloat:  uint64(math.Float32bits(9.9692099683868690e+36)),
	netcdf.TypeDouble: math.Float64bits(9.9692099683868690e+36),
	netcdf.TypeUByte:  math.MaxUint8,
	netcdf.TypeUShort: math.MaxUint16,
	netcdf.TypeUInt:   math.MaxUint32,
	netcdf.TypeInt64:  0x8000000000000002, // -9223372036854775806
	netcdf.TypeUInt64: math.MaxUint64 - 1,
}

// fillOf is the bits an unwritten value of a variable holds: its own
// _FillValue, or the default of its type.
func fillOf(each modelVar) uint64 {
	for _, attr := range each.Attrs {
		if attr.Name == fillName {
			return bitsOf(attr.Values)[0]
		}
	}
	return defaultFill[each.Type]
}
