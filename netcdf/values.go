package netcdf

import (
	classic "github.com/scigolib/netcdf"
)

// reader reads a whole variable, or a slab of it, into a new slice of
// the Go type the variable is stored as. total is how many values.
type reader func(variable *classic.Variable, slab *Slab,
	total int64) (any, error)

// readers is the reader for each of the eleven types.
var readers = map[Type]reader{
	TypeByte:   fetch[int8],
	TypeChar:   fetch[byte],
	TypeShort:  fetch[int16],
	TypeInt:    fetch[int32],
	TypeFloat:  fetch[float32],
	TypeDouble: fetch[float64],
	TypeUByte:  fetch[byte],
	TypeUShort: fetch[uint16],
	TypeUInt:   fetch[uint32],
	TypeInt64:  fetch[int64],
	TypeUInt64: fetch[uint64],
}

// fetch reads into a new slice of one Go type. With no slab it is the
// whole variable, read with Read; with one, ReadSlice.
func fetch[stored any](variable *classic.Variable, slab *Slab,
	total int64) (any, error) {
	values := make([]stored, total)
	var err error
	if slab == nil {
		err = variable.Read(&values)
	} else {
		err = variable.ReadSlice(&values, slab.Start, slab.Count)
	}
	return values, err
}

// Values is the whole variable, read once, as a slice of the Go type it
// is stored as: see Attribute.Value for which. A variable that claims
// more values than the file has bytes is refused before anything is
// allocated.
func (variable *Variable) Values() (any, error) { return variable.read(nil) }

// Slab is a hyperslab of the variable, as a slice of the Go type it is
// stored as. Start and Count hold one value for each dimension.
func (variable *Variable) Slab(slab Slab) (any, error) {
	return variable.read(&slab)
}

// Numbers is the whole variable widened to float64, as stored: no
// scale, no offset, no fill. A text variable is refused. A 64-bit
// integer beyond 2 to the 53rd loses its low bits.
func (variable *Variable) Numbers() ([]float64, error) {
	return variable.numbers(nil)
}

// NumbersSlab is a slab of the variable widened as Numbers does.
func (variable *Variable) NumbersSlab(slab Slab) ([]float64, error) {
	return variable.numbers(&slab)
}

// numbers reads a selection widened.
func (variable *Variable) numbers(slab *Slab) ([]float64, error) {
	if variable.Type() == TypeChar {
		return nil, refusedIn(variable.file.name,
			variableWhat(variable.Name()), "is text, not numbers")
	}
	stored, err := variable.read(slab)
	if err != nil {
		return nil, err
	}
	return widened(stored), nil
}

// read reads a selection after checking it against the file.
func (variable *Variable) read(slab *Slab) (any, error) {
	read, found := readers[variable.Type()]
	if !found {
		return nil, refusedIn(variable.file.name,
			variableWhat(variable.Name()),
			"is of the type %d, which is not one of the eleven",
			variable.Type())
	}
	count := variable.Shape()
	if slab != nil {
		count = slab.Count
	}
	total, err := variable.counted(count)
	if err != nil {
		return nil, err
	}
	values, err := read(variable.inner, slab, total)
	if err != nil {
		return nil, variable.failed(err)
	}
	return values, nil
}

// counted is how many values a count takes. It refuses a negative count,
// an overflow, and more values than the file has bytes, so that a corrupt
// shape cannot ask for memory the file cannot fill.
func (variable *Variable) counted(count []int64) (int64, error) {
	limit := variable.file.size / int64(TypeSize(variable.Type()))
	total := int64(1)
	for _, each := range count {
		if each < noValues || (each > noValues && total > limit/each) {
			return 0, refusedIn(variable.file.name,
				variableWhat(variable.Name()),
				"is asked for a count of %v, which the "+
					"file's %d bytes cannot hold", count,
				variable.file.size)
		}
		total *= each
	}
	return total, nil
}

// number is what a value widens to a float64 from.
type number interface {
	~int8 | ~int16 | ~int32 | ~int64 | ~uint8 | ~uint16 | ~uint32 |
		~uint64 | ~float32 | ~float64
}

// widen is a list as float64s.
func widen[stored number](list []stored) []float64 {
	wide := make([]float64, len(list))
	for at, each := range list {
		wide[at] = float64(each)
	}
	return wide
}

// widened is a slice of any stored Go type as float64s, or nil for a
// type it does not know.
func widened(stored any) []float64 {
	switch list := stored.(type) {
	case []int8:
		return widen(list)
	case []byte:
		return widen(list)
	case []int16:
		return widen(list)
	case []uint16:
		return widen(list)
	case []int32:
		return widen(list)
	case []uint32:
		return widen(list)
	case []int64:
		return widen(list)
	case []uint64:
		return widen(list)
	case []float32:
		return widen(list)
	case []float64:
		return widen(list)
	}
	return nil
}
