package netcdf

import (
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	classic "github.com/scigolib/netcdf"
)

// unnamedBytes is the name a file read from bytes is given in errors when
// its Source gives none.
const unnamedBytes = "bytes"

// Source is a file's bytes: an io.ReaderAt and the length, and a name
// for the errors to say. It is what a file read from memory, a part of a
// larger file or a test fixture is given.
type Source struct {
	Name string
	At   io.ReaderAt
	Size int64
}

// File is an open classic netCDF file. It may be read from several
// goroutines at once; Close must not run beside reads.
type File struct {
	name  string
	size  int64
	inner *classic.File
}

// Open opens the file at a path. It refuses an empty path. A NetCDF-4
// (HDF5) file is refused in a sentence that says so, and its error
// wraps ErrUnsupportedFormat.
func Open(path string) (*File, error) {
	if path == "" {
		return nil, refuse("is asked to open a file with no name")
	}
	info, err := os.Stat(path)
	if err != nil {
		return nil, failure(path, "cannot be opened", err)
	}
	inner, err := classic.Open(path)
	if err != nil {
		return nil, openError(path, err)
	}
	return built(path, info.Size(), inner), nil
}

// Read reads a file from its bytes. It refuses no bytes and a negative
// size. Its errors are those of Open.
func Read(source Source) (*File, error) {
	name := source.Name
	if name == "" {
		name = unnamedBytes
	}
	if source.At == nil {
		return nil, refuse("%s is given no bytes to read", name)
	}
	if source.Size < 0 {
		return nil, refuse("%s has a size of %d bytes", name,
			source.Size)
	}
	inner, err := classic.NewReader(source.At, source.Size)
	if err != nil {
		return nil, openError(name, err)
	}
	return built(name, source.Size, inner), nil
}

// built is the file that wraps an opened one.
func built(name string, size int64, inner *classic.File) *File {
	return &File{name: name, size: size, inner: inner}
}

// openError says why a file did not open, in a sentence naming it, with
// the reader's error kept under it.
func openError(name string, err error) error {
	switch {
	case errors.Is(err, classic.ErrUnsupportedFormat):
		return fmt.Errorf("%s is a NetCDF-4 (HDF5) file; this adapter "+
			"reads classic netCDF files until the reader does: %w",
			name, err)
	case errors.Is(err, classic.ErrNotNetCDF):
		return fmt.Errorf("%s is not a netCDF file: %w", name, err)
	}
	return failure(name, "cannot be read as netCDF", err)
}

// Name is the file's name as errors say it: its path, or its Source's
// name.
func (file *File) Name() string { return file.name }

// Close releases the file. A file read from bytes leaves its bytes open.
func (file *File) Close() error {
	if err := file.inner.Close(); err != nil {
		return failure(file.name, "cannot be closed", err)
	}
	return nil
}

// Format is the file's format, one of the classic three.
func (file *File) Format() Format { return file.inner.Format() }

// NumRecs is the length of the file's unlimited dimension, or none for a
// file that has no such dimension.
//
// It counts the dimension, not the variables: a file that declares the
// dimension and has no record variable may still give a count above
// none, where Writer.NumRecs counts the records of record variables.
func (file *File) NumRecs() int64 {
	for _, each := range file.inner.Dimensions() {
		if each.Unlimited() {
			return each.Len()
		}
	}
	return noValues
}

// Dimensions are the file's dimensions, in the order written.
func (file *File) Dimensions() []*Dimension {
	var found []*Dimension
	for _, each := range file.inner.Dimensions() {
		found = append(found, &Dimension{inner: each})
	}
	return found
}

// Dimension is the dimension of a name, or a refusal that names the file
// and lists its dimensions.
func (file *File) Dimension(name string) (*Dimension, error) {
	each, found := file.inner.Dimension(name)
	if !found {
		return nil, refuse("%s has no dimension %q; it has %s",
			file.name, name, names(file.Dimensions()))
	}
	return &Dimension{inner: each}, nil
}

// Attributes are the file's global attributes, in the order written.
func (file *File) Attributes() []*Attribute {
	var found []*Attribute
	for _, each := range file.inner.Attributes() {
		found = append(found, &Attribute{inner: each})
	}
	return found
}

// Attribute is the global attribute of a name, or a refusal that names
// the file and lists its attributes.
func (file *File) Attribute(name string) (*Attribute, error) {
	each, found := file.inner.Attribute(name)
	if !found {
		return nil, refuse("%s has no global attribute %q; it has %s",
			file.name, name, names(file.Attributes()))
	}
	return &Attribute{inner: each}, nil
}

// Variables are the file's variables, in the order written.
func (file *File) Variables() []*Variable {
	var found []*Variable
	for _, each := range file.inner.Variables() {
		found = append(found, &Variable{file: file, inner: each})
	}
	return found
}

// Variable is the variable of a name, or a refusal that names the file
// and lists its variables.
func (file *File) Variable(name string) (*Variable, error) {
	each, found := file.inner.Variable(name)
	if !found {
		return nil, refuse("%s has no variable %q; it has %s",
			file.name, name, names(file.Variables()))
	}
	return &Variable{file: file, inner: each}, nil
}

// namer is anything with a name.
type namer interface{ Name() string }

// names lists names for an error, or says there are none.
func names[kind namer](list []kind) string {
	var each []string
	for _, item := range list {
		each = append(each, item.Name())
	}
	if len(each) == 0 {
		return "none"
	}
	return strings.Join(each, ", ")
}
