package netcdf

import (
	"fmt"

	classic "github.com/scigolib/netcdf"
)

// Writer writes the values of a file made from a Layout. Close it, which
// writes the record count into the header.
type Writer struct {
	path    string
	inner   *classic.Writer
	records map[string]bool // the record variables, by name
}

// VariableWriter writes one variable's values.
type VariableWriter struct {
	writer *Writer
	inner  *classic.VarWriter
}

// Format is the format the file is written in.
func (writer *Writer) Format() Format { return writer.inner.Format() }

// NumRecs is how many records the file's record variables hold so far,
// taken from the first of them, since the writer keeps them level. It
// is none when the file has no record variable, even if it declares an
// unlimited dimension; File.NumRecs counts that dimension instead.
func (writer *Writer) NumRecs() int64 {
	for _, each := range writer.inner.Variables() {
		if writer.records[each.Name()] {
			return each.Shape()[firstValue]
		}
	}
	return noValues
}

// Variables are the writers of the file's variables, in the order of the
// layout.
func (writer *Writer) Variables() []*VariableWriter {
	var found []*VariableWriter
	for _, each := range writer.inner.Variables() {
		found = append(found,
			&VariableWriter{writer: writer, inner: each})
	}
	return found
}

// Variable is the writer of the variable of a name, or a refusal that
// names the file and lists its variables.
func (writer *Writer) Variable(name string) (*VariableWriter, error) {
	each, found := writer.inner.Variable(name)
	if !found {
		return nil, refuse("%s has no variable %q to write; it has %s",
			writer.path, name, names(writer.Variables()))
	}
	return &VariableWriter{writer: writer, inner: each}, nil
}

// Close writes the record count and closes the file. A second Close is
// not an error.
func (writer *Writer) Close() error {
	if err := writer.inner.Close(); err != nil {
		return failure(writer.path, "cannot be closed", err)
	}
	return nil
}

// Name is the variable's name.
func (variable *VariableWriter) Name() string { return variable.inner.Name() }

// Type is the variable's type.
func (variable *VariableWriter) Type() Type { return variable.inner.Type() }

// Record is whether the variable's first dimension is the unlimited one.
func (variable *VariableWriter) Record() bool {
	return variable.writer.records[variable.Name()]
}

// Shape is the variable's shape so far; for a record variable the first
// is the record count.
func (variable *VariableWriter) Shape() []int64 {
	return variable.inner.Shape()
}

// failed is an error from the writer, with the file and the variable
// named in front of it.
func (variable *VariableWriter) failed(err error) error {
	return failure(variable.writer.path, variableWhat(variable.Name()), err)
}

// Write writes the whole variable, from a slice of the Go type it is
// stored as (see Attribute.Value). A fixed variable's values must fill
// its shape. A record variable's are written from record 0, and extend
// the record dimension by as many records as they cover.
func (variable *VariableWriter) Write(values any) error {
	if err := variable.store(values, nil); err != nil {
		return variable.failed(err)
	}
	return nil
}

// WriteSlab writes a hyperslab. A record variable written past its
// record count extends the file, and the records between are filled.
func (variable *VariableWriter) WriteSlab(values any, slab Slab) error {
	if err := variable.store(values, &slab); err != nil {
		return variable.failed(err)
	}
	return nil
}

// storers is the writer for each of the eleven types, as readers is the
// reader: the reader under this package takes a slice of one Go type, so
// the variable's type chooses which.
var storers = map[Type]storer{
	TypeByte:   put[int8],
	TypeChar:   put[byte],
	TypeShort:  put[int16],
	TypeInt:    put[int32],
	TypeFloat:  put[float32],
	TypeDouble: put[float64],
	TypeUByte:  put[byte],
	TypeUShort: put[uint16],
	TypeUInt:   put[uint32],
	TypeInt64:  put[int64],
	TypeUInt64: put[uint64],
}

// storer writes a whole variable, or a slab of it, from values.
type storer func(inner *classic.VarWriter, values any, slab *Slab) error

// put writes from a slice of one Go type. With no slab it is the whole
// variable, written with Write; with one, WriteSlice. Values of another
// type are an error that wraps ErrTypeMismatch, as the reader's own.
func put[stored any](inner *classic.VarWriter, values any,
	slab *Slab) error {
	list, ok := values.([]stored)
	if !ok {
		return mismatch(inner, values)
	}
	if slab == nil {
		return inner.Write(list)
	}
	return inner.WriteSlice(list, slab.Start, slab.Count)
}

// mismatch is the error for values that are not of the type a variable
// holds, in the words the reader under this package uses.
func mismatch(inner *classic.VarWriter, values any) error {
	return fmt.Errorf("%w: got %T, variable %q holds %s values",
		ErrTypeMismatch, values, inner.Name(), inner.Type())
}

// store writes a whole variable, or a slab of it, with the writer of the
// variable's type.
func (variable *VariableWriter) store(values any, slab *Slab) error {
	write, found := storers[variable.Type()]
	if !found {
		return mismatch(variable.inner, values)
	}
	return write(variable.inner, values, slab)
}
