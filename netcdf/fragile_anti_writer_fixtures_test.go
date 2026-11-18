//go:build fragile

package netcdf_test

// The anti-tests of writer_fixtures_test.go, which compares the bytes his
// writer makes with the files netCDF-C wrote. An anti writes a model that
// differs in one field and asserts that the bytes differ in the place the
// field lives: the first byte that differs between the two files his
// writer makes is the first byte that differs between the two files the
// harness's encoder makes of the same two models. Two writers that
// disagree about where a field lives, or a writer that does not write the
// field, fail here, which a comparison with one fixture cannot say.

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/janearc/celestial-data/netcdf"
)

// rampValues is count values of a type, each its position made a number
// of the type: 7 times the position, plus one.
func rampValues(typ netcdf.Type, count int) any {
	switch typ {
	case netcdf.TypeByte:
		return rampOf(count, func(at int) int8 { return int8(7*at + 1) })
	case netcdf.TypeShort:
		return rampOf(count, func(at int) int16 { return int16(7*at + 1) })
	case netcdf.TypeInt:
		return rampOf(count, func(at int) int32 { return int32(7*at + 1) })
	case netcdf.TypeFloat:
		return rampOf(count, func(at int) float32 { return float32(7*at) + 1 })
	case netcdf.TypeDouble:
		return rampOf(count, func(at int) float64 { return float64(7*at) + 1 })
	case netcdf.TypeUShort:
		return rampOf(count, func(at int) uint16 { return uint16(7*at + 1) })
	case netcdf.TypeUInt:
		return rampOf(count, func(at int) uint32 { return uint32(7*at + 1) })
	case netcdf.TypeInt64:
		return rampOf(count, func(at int) int64 { return int64(7*at + 1) })
	case netcdf.TypeUInt64:
		return rampOf(count, func(at int) uint64 { return uint64(7*at + 1) })
	}
	return rampOf(count, func(at int) byte { return byte(7*at + 1) })
}

// rampOf is count values, each made from its position.
func rampOf[kind any](count int, one func(int) kind) []kind {
	found := make([]kind, count)
	for at := range found {
		found[at] = one(at)
	}
	return found
}

// reramped is the model with the values of each variable made again, for
// the shape it now has.
func (model fragModel) reramped() fragModel {
	found := model
	found.Vars = append([]modelVar(nil), model.Vars...)
	for at, each := range found.Vars {
		found.Vars[at].Values = rampValues(each.Type,
			int(countOf(found.shape(each))))
	}
	return found
}

// antiModel is a model with a record dimension of two records, a fixed
// dimension of three and one of two, two attributes, and four variables
// of four types, each with an attribute, over those.
func antiModel(format netcdf.Format) fragModel {
	model := fragModel{Format: format, Records: 2,
		Dims: []modelDim{{"t", 0}, {"x", 3}, {"y", 2}},
		Attrs: []modelAttr{{"title", netcdf.TypeChar, []byte("a title")},
			{"scale", netcdf.TypeDouble, []float64{1.5}}},
		Vars: []modelVar{
			{Name: "a", Type: netcdf.TypeFloat, Dims: []int{1, 2},
				Attrs: []modelAttr{{"units", netcdf.TypeChar, []byte("m")}}},
			{Name: "b", Type: netcdf.TypeInt, Dims: []int{2}},
			{Name: "r", Type: netcdf.TypeShort, Dims: []int{0, 2}},
			{Name: "c", Type: netcdf.TypeByte, Dims: []int{0}}}}
	return model.reramped()
}

// antiChange is one change to a model: what it is, whether it is in the
// header only, and the change.
type antiChange struct {
	what   string
	header bool
	apply  func(*fragModel)
}

// antiChanges are the changes, each of one field.
func antiChanges() []antiChange {
	return []antiChange{
		{"a value of a", false, func(m *fragModel) {
			m.Vars[0].Values.([]float32)[3]++
		}},
		{"a value of r", false, func(m *fragModel) {
			m.Vars[2].Values.([]int16)[2]++
		}},
		{"the records", false, func(m *fragModel) {
			m.Records = 3
			*m = m.reramped()
		}},
		{"the length of x", true, func(m *fragModel) {
			m.Dims[1].Length = 4
			*m = m.reramped()
		}},
		{"the type of b", true, func(m *fragModel) {
			m.Vars[1].Type = netcdf.TypeDouble
			*m = m.reramped()
		}},
		{"the name of a", true, func(m *fragModel) { m.Vars[0].Name = "A" }},
		{"the name of x", true, func(m *fragModel) { m.Dims[1].Name = "X" }},
		{"the value of scale", true, func(m *fragModel) {
			m.Attrs[1].Values = []float64{2.5}
		}},
		{"the name of title", true, func(m *fragModel) {
			m.Attrs[0].Name = "Title"
		}},
		{"the units of a", true, func(m *fragModel) {
			m.Vars[0].Attrs[0].Values = []byte("s")
		}},
		{"an attribute more", true, func(m *fragModel) {
			m.Attrs = append(m.Attrs, modelAttr{"more", netcdf.TypeInt,
				[]int32{1}})
		}},
		{"the order of two variables", true, func(m *fragModel) {
			m.Vars[0], m.Vars[1] = m.Vars[1], m.Vars[0]
			m.Vars[0].Dims, m.Vars[1].Dims = []int{2}, []int{1, 2}
		}},
	}
}

// firstDifferent is the place two byte strings first differ, or the
// length of the shorter if one is a start of the other.
func firstDifferent(left, right []byte) int {
	at := 0
	for at < len(left) && at < len(right) && left[at] == right[at] {
		at++
	}
	return at
}

// fileOf is the bytes his writer makes of a model.
func fileOf(t *testing.T, model fragModel) []byte {
	t.Helper()
	path := filepath.Join(t.TempDir(), "model.nc")
	if err := model.save(path); err != nil {
		t.Fatalf("the writer refused %s: %v", model, err)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

// differAtTheSamePlace asks, for each change of a model in each format,
// that the first byte at which his two files differ is the first byte at
// which the encoder's two files differ, and that the two files do differ.
func differAtTheSamePlace(t *testing.T, headerOnly bool) {
	for _, format := range formatsOf {
		for _, each := range antiChanges() {
			if headerOnly && !each.header {
				continue
			}
			name := fmt.Sprintf("%s/%s", netcdf.FormatName(format), each.what)
			t.Run(name, func(t *testing.T) {
				was := antiModel(format)
				now := antiModel(format)
				each.apply(&now)
				mine, his := firstDifferent(was.image(), now.image()),
					firstDifferent(fileOf(t, was), fileOf(t, now))
				if bytes.Equal(fileOf(t, was), fileOf(t, now)) {
					t.Fatalf("the change is not in the file: %s", each.what)
				}
				if mine != his {
					t.Errorf("the writer first differs at byte %d, the "+
						"grammar says byte %d", his, mine)
				}
			})
		}
	}
}

// The test at writer_fixtures_test.go:190 asserts that the writer makes,
// for seventeen models, the bytes netCDF-C made. Its anti: a model that
// differs from a fixed one in one field, twelve of them, in each of the
// three formats, is a file that differs from the first file at the byte
// the field lives at.
func TestAntiCreateMatchesFixtures(t *testing.T) {
	differAtTheSamePlace(t, false)
}

// The test at writer_fixtures_test.go:244 asserts that the empty model is
// the 32 bytes of the empty fixture. Its anti: a model of one dimension
// and no variable is longer than 32 bytes and first differs from the
// empty one where the encoder says (the list tag, byte 11); and the empty
// model of another format first differs at the version byte.
func TestAntiCreateEmpty(t *testing.T) {
	empty := fileOf(t, fragModel{Format: netcdf.FormatClassic})
	if len(empty) != 32 {
		t.Fatalf("the empty file is %d bytes", len(empty))
	}
	one := fragModel{Format: netcdf.FormatClassic,
		Dims: []modelDim{{"x", 1}}}
	got := fileOf(t, one)
	want := firstDifferent(emptyHeader(cdfOne).Bytes(), one.header().Bytes())
	if len(got) <= 32 || firstDifferent(empty, got) != want {
		t.Errorf("one dimension: %d bytes, first differs at %d, the grammar "+
			"says %d", len(got), firstDifferent(empty, got), want)
	}
	other := fileOf(t, fragModel{Format: netcdf.FormatClassic64})
	if len(other) != 32 || firstDifferent(empty, other) != 3 {
		t.Errorf("the empty CDF-2 file is %d bytes and first differs from "+
			"CDF-1 at %d, want 32 and 3", len(other),
			firstDifferent(empty, other))
	}
}
