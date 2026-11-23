package netcdf

import (
	classic "github.com/scigolib/netcdf"
)

// Format is a netCDF file format, as the reader under this package names
// it: the three classic ones, NetCDF-4 (HDF5) and unknown.
type Format = classic.Format

// The formats. This adapter reads and writes the classic three.
const (
	// FormatUnknown means the header matches no netCDF format.
	FormatUnknown = classic.FormatUnknown
	// FormatClassic is CDF-1, with 32-bit offsets.
	FormatClassic = classic.FormatClassic
	// FormatClassic64 is CDF-2, with 64-bit offsets.
	FormatClassic64 = classic.FormatClassic64
	// FormatClassic64Data is CDF-5, with 64-bit offsets and sizes.
	FormatClassic64Data = classic.FormatClassic64Data
	// FormatHDF5 is NetCDF-4, which this adapter does not read yet.
	FormatHDF5 = classic.FormatHDF5
)

// Type is a netCDF data type, as the reader under this package names it.
type Type = classic.Type

// The eleven types. CDF-1 and CDF-2 know the first six; CDF-5 all.
const (
	// TypeByte is a signed 8-bit integer.
	TypeByte = classic.TypeByte
	// TypeChar is a text character.
	TypeChar = classic.TypeChar
	// TypeShort is a signed 16-bit integer.
	TypeShort = classic.TypeShort
	// TypeInt is a signed 32-bit integer.
	TypeInt = classic.TypeInt
	// TypeFloat is an IEEE single.
	TypeFloat = classic.TypeFloat
	// TypeDouble is an IEEE double.
	TypeDouble = classic.TypeDouble
	// TypeUByte is an unsigned 8-bit integer (CDF-5).
	TypeUByte = classic.TypeUByte
	// TypeUShort is an unsigned 16-bit integer (CDF-5).
	TypeUShort = classic.TypeUShort
	// TypeUInt is an unsigned 32-bit integer (CDF-5).
	TypeUInt = classic.TypeUInt
	// TypeInt64 is a signed 64-bit integer (CDF-5).
	TypeInt64 = classic.TypeInt64
	// TypeUInt64 is an unsigned 64-bit integer (CDF-5).
	TypeUInt64 = classic.TypeUInt64
)

// The reader's errors, the same values, so that errors.Is finds them in
// what this package returns and says whose they are.
var (
	// ErrNotNetCDF is input that is not a netCDF file.
	ErrNotNetCDF = classic.ErrNotNetCDF
	// ErrCorrupt is a file whose header or data breaks the format.
	ErrCorrupt = classic.ErrCorrupt
	// ErrOutOfRange is a selection outside a variable, or a destination
	// of the wrong length.
	ErrOutOfRange = classic.ErrOutOfRange
	// ErrUnsupportedFormat is a format the reader does not read yet.
	ErrUnsupportedFormat = classic.ErrUnsupportedFormat
	// ErrTypeMismatch is a destination of the wrong element type.
	ErrTypeMismatch = classic.ErrTypeMismatch
	// ErrClosed is the use of a file after Close.
	ErrClosed = classic.ErrClosed
	// ErrInvalidSchema is a layout that cannot be written.
	ErrInvalidSchema = classic.ErrInvalidSchema
)

// AllTypes is the eleven types, in the order of their tags.
func AllTypes() []Type {
	return []Type{TypeByte, TypeChar, TypeShort, TypeInt, TypeFloat,
		TypeDouble, TypeUByte, TypeUShort, TypeUInt, TypeInt64,
		TypeUInt64}
}

// DetectFormat is the format a file's first bytes say. Eight bytes are
// enough; fewer are not an error, and give FormatUnknown unless they are
// a whole classic header.
func DetectFormat(header []byte) Format { return classic.DetectFormat(header) }

// FormatName is a format's name as the netCDF documentation writes it.
func FormatName(format Format) string { return format.String() }

// TypeName is a type's name as CDL writes it.
func TypeName(typ Type) string { return typ.String() }

// TypeSize is the bytes one value of a type takes in a file.
func TypeSize(typ Type) int { return typ.Size() }

// Slab is a hyperslab: for each dimension of a variable, the index of
// the first value and how many are taken. The record dimension is
// addressed by record number.
type Slab struct {
	Start, Count []int64
}
