package starhorse_test

import (
	"bytes"
	"math"
	"testing"

	"pgregory.net/rapid"

	"github.com/janearc/celestial-data/internal/fitstest"
	"github.com/janearc/celestial-data/starhorse"
)

// drawRow draws a crafted row's seventeen numbers, NaN among them, and a
// source that is not the crafted column's TNULL.
func drawRow(draw *rapid.T) ([singles]float32, int64) {
	var numbers [singles]float32
	for at := range numbers {
		numbers[at] = rapid.Float32().Draw(draw, "number")
	}
	return numbers, rapid.Int64().Filter(notNull).Draw(draw, "source")
}

// notNull is whether a source is not the crafted column's TNULL.
func notNull(source int64) bool { return source != nullSource }

// same is whether a read double is a written single, NaN being NaN.
func same(read float64, written float32) bool {
	wrote := float64(written)
	return read == wrote || math.IsNaN(read) && math.IsNaN(wrote)
}

// the property: a row of any numbers and any source reads back as it
// was written, each in its own place, NaN as NaN.
func TestAnyRowReadsBackAsWritten(test *testing.T) {
	rapid.Check(test, readsBack)
}

// readsBack writes a drawn row, reads it, and holds each value to it.
func readsBack(draw *rapid.T) {
	numbers, source := drawRow(draw)
	row := fitstest.NewRow().Text(craftedID, idWidth).Long(source)
	for _, number := range numbers {
		row = row.Single(number)
	}
	row = row.Text("output", outputWidth).Text(craftedAge, flagWidth).
		Text("none", flagWidth)
	file := fitstest.File(layout(), row)
	catalogue, err := starhorse.OpenCatalogue(bytes.NewReader(file))
	if err != nil {
		draw.Fatal(err)
	}
	star, err := catalogue.Star(onlyRow)
	if err != nil {
		draw.Fatal(err)
	}
	low, high := star.AgeWide()
	mass, distance := star.Mass(), star.Distance()
	extinction, age := star.Extinction(), star.Age()
	read := [singles]float64{mass.Low(), mass.Median(), mass.High(),
		star.Temperature(), star.Gravity(), star.Metals(), distance.Low(),
		distance.Median(), distance.High(), extinction.Low(),
		extinction.Median(), extinction.High(), low, age.Low(), age.Median(),
		age.High(), high}
	for at := range read {
		if !same(read[at], numbers[at]) {
			draw.Fatalf("number %d: %v, written %v", at, read[at], numbers[at])
		}
	}
	if gaia, known := star.GaiaID(); gaia != source || !known {
		draw.Fatalf("source %d %v, written %d", gaia, known, source)
	}
}
