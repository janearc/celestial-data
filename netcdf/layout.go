package netcdf

import (
	classic "github.com/scigolib/netcdf"
)

// Layout is the design of a file to be written: its dimensions, its
// variables and their attributes, and its global attributes. Create
// writes the file from it.
type Layout struct {
	inner *classic.Schema
}

// LayoutDimension is a dimension of a layout. A length of zero is the
// unlimited (record) dimension, of which a layout may have one.
type LayoutDimension struct {
	layout *Layout
	inner  *classic.SchemaDim
}

// LayoutVariable is a variable of a layout, with its attributes.
type LayoutVariable struct {
	layout *Layout
	inner  *classic.SchemaVar
}

// VariableSpec is what AddVariable is given: a name, a type, and the
// dimensions, from this layout, that the variable runs over.
type VariableSpec struct {
	Name       string
	Type       Type
	Dimensions []*LayoutDimension
}

// NewLayout is an empty layout for a file of a format. It refuses a
// format that is not one of the classic three.
func NewLayout(format Format) (*Layout, error) {
	switch format {
	case FormatClassic, FormatClassic64, FormatClassic64Data:
		return &Layout{inner: classic.NewSchema(format)}, nil
	}
	return nil, refuse("writes classic netCDF files, not %s",
		FormatName(format))
}

// Format is the format the layout will be written in.
func (layout *Layout) Format() Format { return layout.inner.Format() }

// AddDimension adds a dimension. It refuses an empty name and a negative
// length; the reader's own checks of the name come with Create.
func (layout *Layout) AddDimension(name string,
	length int64) (*LayoutDimension, error) {
	if name == "" || length < noValues {
		return nil, refuse("a dimension needs a name and a length of "+
			"none or more, not %q and %d", name, length)
	}
	return &LayoutDimension{layout: layout,
		inner: layout.inner.AddDimension(name, length)}, nil
}

// AddVariable adds a variable. It refuses an empty name and a dimension
// that another layout made.
func (layout *Layout) AddVariable(spec VariableSpec) (*LayoutVariable,
	error) {
	if spec.Name == "" {
		return nil, refuse("a variable needs a name")
	}
	inner := make([]*classic.SchemaDim, len(spec.Dimensions))
	for at, each := range spec.Dimensions {
		if each == nil || each.layout != layout {
			return nil, refuse("variable %q runs over a dimension "+
				"that this layout did not make", spec.Name)
		}
		inner[at] = each.inner
	}
	added := layout.inner.AddVariable(spec.Name, spec.Type, inner...)
	return &LayoutVariable{layout: layout, inner: added}, nil
}

// Dimensions are the layout's dimensions, in the order added.
func (layout *Layout) Dimensions() []*LayoutDimension {
	var found []*LayoutDimension
	for _, each := range layout.inner.Dimensions() {
		found = append(found,
			&LayoutDimension{layout: layout, inner: each})
	}
	return found
}

// Variables are the layout's variables, in the order added.
func (layout *Layout) Variables() []*LayoutVariable {
	var found []*LayoutVariable
	for _, each := range layout.inner.Variables() {
		found = append(found,
			&LayoutVariable{layout: layout, inner: each})
	}
	return found
}

// Set sets a global attribute, its type taken from the Go value: a string
// or []byte is text, []int8 a byte, []int16 a short, []int32 an int,
// []float32 a float, []float64 a double, []uint16, []uint32, []int64 and
// []uint64 their own. A name set twice takes the last.
func (layout *Layout) Set(name string, values any) error {
	return layoutError("the layout", name,
		layout.inner.SetAttr(name, values))
}

// SetTyped sets a global attribute of a type that the Go value does not
// say, such as an unsigned byte.
func (layout *Layout) SetTyped(name string, typ Type, values any) error {
	return layoutError("the layout", name,
		layout.inner.SetAttrTyped(name, typ, values))
}

// Name is the dimension's name.
func (dimension *LayoutDimension) Name() string {
	return dimension.inner.Name()
}

// Length is the dimension's length; the unlimited one is none.
func (dimension *LayoutDimension) Length() int64 {
	return dimension.inner.Len()
}

// Unlimited is whether this is the record dimension.
func (dimension *LayoutDimension) Unlimited() bool {
	return dimension.inner.Unlimited()
}

// Name is the variable's name.
func (variable *LayoutVariable) Name() string { return variable.inner.Name() }

// Type is the variable's type.
func (variable *LayoutVariable) Type() Type { return variable.inner.Type() }

// Dimensions are the variable's dimensions, in order.
func (variable *LayoutVariable) Dimensions() []*LayoutDimension {
	var found []*LayoutDimension
	for _, each := range variable.inner.Dims() {
		found = append(found, &LayoutDimension{layout: variable.layout,
			inner: each})
	}
	return found
}

// Attributes are the variable's attributes, in the order set.
func (variable *LayoutVariable) Attributes() []*Attribute {
	var found []*Attribute
	for _, each := range variable.inner.Attributes() {
		found = append(found, &Attribute{inner: each})
	}
	return found
}

// Set sets an attribute of the variable as Layout.Set does.
func (variable *LayoutVariable) Set(name string, values any) error {
	return layoutError(variableWhat(variable.Name()), name,
		variable.inner.SetAttr(name, values))
}

// SetTyped sets an attribute of the variable of a stated type.
func (variable *LayoutVariable) SetTyped(name string, typ Type,
	values any) error {
	return layoutError(variableWhat(variable.Name()), name,
		variable.inner.SetAttrTyped(name, typ, values))
}

// layoutError says which attribute of what the reader's error is about.
func layoutError(what, name string, err error) error {
	if err == nil {
		return nil
	}
	return failure(what, "attribute "+name, err)
}

// Create writes the file at a path from the layout and gives the writer
// for its values. The layout is checked here by the reader's writer: a
// fault in it is an error that wraps ErrInvalidSchema. The file's
// fixed variables are filled with their fill value at once.
func (layout *Layout) Create(path string) (*Writer, error) {
	if path == "" {
		return nil, refuse("is asked to create a file with no name")
	}
	inner, err := classic.Create(path, layout.inner)
	if err != nil {
		return nil, failure(path, "cannot be created", err)
	}
	return &Writer{path: path, inner: inner,
		records: layout.recordNames()}, nil
}

// recordNames are the names of the layout's record variables: those
// whose first dimension is the unlimited one. The writer under the
// adapter does not say which of its variables are.
func (layout *Layout) recordNames() map[string]bool {
	records := make(map[string]bool)
	for _, each := range layout.Variables() {
		dimensions := each.Dimensions()
		records[each.Name()] = len(dimensions) > noValues &&
			dimensions[firstValue].Unlimited()
	}
	return records
}
