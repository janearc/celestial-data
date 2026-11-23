package netcdf

// These tests reach holdsCells and lonCell, which are not exported. The
// package's other tests are outside it and have no way in.

import (
	"math"
	"path/filepath"
	"strings"
	"testing"
)

// The numbers of the tests. The columns are a quarter turn apart.
const (
	cellsTolerance = 1e-9  // defined: the arithmetic here is exact enough
	cellsName      = "tcc" // defined: the variable the file holds
	hugeLongitude  = 1e17  // defined: far above 2^53, where floor loses
	cellsColumns   = 4     // defined: the length of cellsLons
	cellsUnits     = "hours since 1900-01-01 00:00:0.0"
)

// cellsLons are the longitude columns of the tests, in degrees.
var cellsLons = []float64{0, 90, 180, 270}

// cellsField is a field read from a small file written for the test,
// so that it has a real variable and file to name in a refusal.
func cellsField(t *testing.T) *Field {
	t.Helper()
	lats := []float64{-60, 0, 60}
	steps := [][]float64{make([]float64, len(lats)*cellsColumns)}
	path := filepath.Join(t.TempDir(), "cells.nc")
	err := WriteGrid(path, GridFile{Latitudes: lats,
		Longitudes: cellsLons, TimeUnits: cellsUnits, Times: []float64{0},
		Variables: []GridVariable{{Name: cellsName, Units: "1",
			Steps: steps}}})
	if err != nil {
		t.Fatal(err)
	}
	file, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = file.Close() })
	found, err := file.Field(cellsName)
	if err != nil {
		t.Fatal(err)
	}
	return found
}

// A field with no latitudes is refused, and one with no longitudes is
// refused, each in words that name the axis and the variable.
func TestHoldsCellsRefusesAnEmptyAxis(t *testing.T) {
	for _, each := range []struct {
		name string
		want string
		cut  func(*Field)
	}{
		{"latitudes", "has no latitudes", func(f *Field) { f.lats = nil }},
		{"longitudes", "has no longitudes", func(f *Field) { f.lons = nil }},
	} {
		field := cellsField(t)
		each.cut(field)
		err := field.holdsCells()
		if err == nil {
			t.Fatalf("no latitudes or longitudes: %s not refused", each.name)
		}
		if !strings.Contains(err.Error(), each.want) ||
			!strings.Contains(err.Error(), cellsName) {
			t.Errorf("%s: refusal is %q, want %q and the variable",
				each.name, err, each.want)
		}
	}
}

// Anti-test: a field with both axes is not refused, and the one that
// lacks only latitudes is not told it lacks longitudes.
func TestHoldsCellsKeepsAFieldWithBothAxes(t *testing.T) {
	field := cellsField(t)
	if err := field.holdsCells(); err != nil {
		t.Errorf("a field with both axes is refused: %v", err)
	}
	field.lats = nil
	err := field.holdsCells()
	if err == nil || strings.Contains(err.Error(), "no longitudes") {
		t.Errorf("an empty latitude axis gives %v", err)
	}
}

// Columns 0, 90, 180, 270. A longitude of 45 is half way from column 0
// to column 1: place 45, first column above it is 1, so i is 0, j is 1,
// and share is (45-0)/(90-0) = 0.5. A whole turn either way, or two,
// is the same place, so the cell and share are the same.
func TestLonCellWholeTurnsAreTheSamePlace(t *testing.T) {
	for _, lon := range []float64{45, 45 + 360, 45 - 360, 45 + 720} {
		i, j, share := lonCell(cellsLons, lon)
		if i != 0 || j != 1 || math.Abs(share-0.5) > cellsTolerance {
			t.Errorf("lon %v: cell %d, %d, share %v, want 0, 1, 0.5",
				lon, i, j, share)
		}
	}
}

// Anti-test: 135 is not a whole turn from 45. Place 135, first column
// above it is 2, so i is 1, j is 2, share (135-90)/90 = 0.5. The cell
// differs from 45's, and 100 differs in share: (100-90)/90 = 1/9.
func TestLonCellADifferentLongitudeIsADifferentCell(t *testing.T) {
	i, j, _ := lonCell(cellsLons, 135)
	if i != 1 || j != 2 {
		t.Errorf("lon 135: cell %d, %d, want 1, 2", i, j)
	}
	i, j, share := lonCell(cellsLons, 100)
	if i != 1 || j != 2 || math.Abs(share-1.0/9.0) > cellsTolerance {
		t.Errorf("lon 100: cell %d, %d, share %v, want 1, 2, 1/9",
			i, j, share)
	}
}

// One degree below the first column is 359 past it: no column is above
// 359, so i is 3, the last, and j wraps to 0. The span across the seam
// is 0-270+360 = 90, and share is (359-270)/90 = 89/90.
func TestLonCellJustBelowTheFirstColumnWraps(t *testing.T) {
	i, j, share := lonCell(cellsLons, -1)
	if i != cellsColumns-1 || j != 0 ||
		math.Abs(share-89.0/90.0) > cellsTolerance {
		t.Errorf("lon -1: cell %d, %d, share %v, want 3, 0, 89/90",
			i, j, share)
	}
}

// A longitude of 1e17 has no fraction left in a float, so dividing and
// flooring cannot place it. Whatever place it takes, it is inside the
// columns and its share is from 0 to 1, and it does not panic.
func TestLonCellAHugeLongitudeStaysInside(t *testing.T) {
	for _, lon := range []float64{hugeLongitude, -hugeLongitude} {
		i, j, share := lonCell(cellsLons, lon)
		if i < 0 || i >= len(cellsLons) || j < 0 || j >= len(cellsLons) {
			t.Errorf("lon %v: cell %d, %d is outside the columns", lon, i, j)
		}
		if share < 0 || share > 1 {
			t.Errorf("lon %v: share %v is not from 0 to 1", lon, share)
		}
	}
}
