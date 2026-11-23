package netcdf

import (
	"fmt"

	classic "github.com/scigolib/netcdf"
)

// Dimension is one dimension of a file, shared by the variables that use
// it.
type Dimension struct {
	inner *classic.Dimension
}

// Name is the dimension's name.
func (dimension *Dimension) Name() string { return dimension.inner.Name() }

// Length is the dimension's length. For the unlimited dimension it is
// the file's record count.
func (dimension *Dimension) Length() int64 { return dimension.inner.Len() }

// Unlimited is whether this is the record dimension.
func (dimension *Dimension) Unlimited() bool {
	return dimension.inner.Unlimited()
}

// Attribute is one attribute, global or on a variable. Its values are
// kept as the file stores them; Value, Text and Bytes give them.
type Attribute struct {
	inner *classic.Attribute
}

// Name is the attribute's name.
func (attribute *Attribute) Name() string { return attribute.inner.Name() }

// Type is the attribute's netCDF type.
func (attribute *Attribute) Type() Type { return attribute.inner.Type() }

// Length is how many values the attribute holds.
func (attribute *Attribute) Length() int { return attribute.inner.Len() }

// Bytes is the values as the file stores them, big-endian, a copy.
func (attribute *Attribute) Bytes() []byte { return attribute.inner.Bytes() }

// Text is a text attribute's value, and nothing for any other type.
func (attribute *Attribute) Text() string { return attribute.inner.String() }

// Value is the values as a slice of their Go type: []int8 for a byte,
// []byte for a char or an unsigned byte, and []int16, []uint16, []int32,
// []uint32, []float32, []float64, []int64 or []uint64 for the rest.
func (attribute *Attribute) Value() any { return attribute.inner.Value() }

// Number is the first value of a numeric attribute as a float64, and
// whether there is one. A text attribute has none.
func (attribute *Attribute) Number() (float64, bool) {
	if attribute.Length() == noValues || attribute.Type() == TypeChar {
		return 0, false
	}
	list := widened(attribute.Value())
	if len(list) == noValues {
		return 0, false
	}
	return list[firstValue], true
}

// noValues is an attribute with nothing in it. It's defined.
const noValues = 0

// firstValue is the place of the first value of a list. It's defined.
const firstValue = 0

// Variable is one variable of a file.
type Variable struct {
	file  *File
	inner *classic.Variable
}

// Name is the variable's name.
func (variable *Variable) Name() string { return variable.inner.Name() }

// Type is the variable's netCDF type.
func (variable *Variable) Type() Type { return variable.inner.Type() }

// Dimensions are the variable's dimensions, in order.
func (variable *Variable) Dimensions() []*Dimension {
	var found []*Dimension
	for _, each := range variable.inner.Dims() {
		found = append(found, &Dimension{inner: each})
	}
	return found
}

// Shape is the length along each dimension. For a record variable the
// first is the file's record count.
func (variable *Variable) Shape() []int64 { return variable.inner.Shape() }

// Record is whether the variable's first dimension is the unlimited one.
func (variable *Variable) Record() bool {
	dimensions := variable.inner.Dims()
	return len(dimensions) > noValues && dimensions[firstValue].Unlimited()
}

// Length is how many values the variable holds.
func (variable *Variable) Length() int64 { return variable.inner.Len() }

// Attributes are the variable's attributes, in the order written.
func (variable *Variable) Attributes() []*Attribute {
	var found []*Attribute
	for _, each := range variable.inner.Attributes() {
		found = append(found, &Attribute{inner: each})
	}
	return found
}

// Attribute is the variable's attribute of a name, or a refusal that
// names the file and the variable.
func (variable *Variable) Attribute(name string) (*Attribute, error) {
	each, found := variable.inner.Attribute(name)
	if !found {
		return nil, refusedIn(variable.file.name,
			variableWhat(variable.Name()),
			"has no attribute %q; it has %s", name,
			names(variable.Attributes()))
	}
	return &Attribute{inner: each}, nil
}

// failed is an error from the reader, with the file and the variable
// named in front of it.
func (variable *Variable) failed(err error) error {
	return failure(variable.file.name, variableWhat(variable.Name()), err)
}

// refusedIn is a refusal that names a file and what is in it, and says
// why.
func refusedIn(file, what, format string, args ...any) error {
	return refuseText(fmt.Sprintf("%s: %s %s", file, what,
		fmt.Sprintf(format, args...)))
}

// refuseText is a refusal whose words are made already.
func refuseText(text string) error { return refuse("%s", text) }
