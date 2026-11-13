package astronn_test

import (
	"bytes"
	"math"
	"testing"

	"pgregory.net/rapid"

	"github.com/janearc/celestial-data/astronn"
	"github.com/janearc/celestial-data/internal/fitstest"
)

// drawnRow is a crafted row's numbers, drawn: the nine before its source
// and the six after, in the file's order.
type drawnRow struct {
	before [doublesBefore]float64
	after  [doublesAfter]float64
	source int64
}

// drawRow draws a row: any doubles, NaN among them, and any source but
// the crafted column's TNULL.
func drawRow(draw *rapid.T) drawnRow {
	var row drawnRow
	for at := range row.before {
		row.before[at] = rapid.Float64().Draw(draw, "before")
	}
	for at := range row.after {
		row.after[at] = rapid.Float64().Draw(draw, "after")
	}
	row.source = rapid.Int64().Filter(notNull).Draw(draw, "source")
	return row
}

// notNull is whether a source is not the crafted column's TNULL.
func notNull(source int64) bool { return source != nullSource }

// same is whether two doubles are the same number, NaN being NaN.
func same(one, other float64) bool {
	return one == other || math.IsNaN(one) && math.IsNaN(other)
}

// the property: a row of any numbers and any source reads back as it
// was written, each number in its own place, NaN as NaN.
func TestAnyRowReadsBackAsWritten(test *testing.T) {
	rapid.Check(test, readsBack)
}

// readsBack writes a drawn row, reads it, and holds each value to it.
func readsBack(draw *rapid.T) {
	drawn := drawRow(draw)
	row := fitstest.NewRow().Text(realID, idWidth).Text("lco25m", telescopeLen)
	for _, value := range drawn.before {
		row = row.Double(value)
	}
	row = row.Long(drawn.source)
	for _, value := range drawn.after {
		row = row.Double(value)
	}
	file := fitstest.File(layout(), row)
	catalogue, err := astronn.OpenCatalogue(bytes.NewReader(file))
	if err != nil {
		draw.Fatal(err)
	}
	star, err := catalogue.Star(onlyRow)
	if err != nil {
		draw.Fatal(err)
	}
	distance, distanceError := star.Distance()
	weighted, weightedError := star.WeightedDistance()
	read := [doublesBefore]float64{distance, distanceError, weighted,
		weightedError, star.AgeUncorrected(), star.AgeLinear(), star.Age(),
		star.AgeError(), star.AgeModelError()}
	orbit := [doublesAfter]float64{star.Eccentricity(),
		star.HighestAbovePlane(), star.Pericentre(), star.Apocentre(),
		star.AngularMomentum(), star.Energy()}
	for at := range read {
		if !same(read[at], drawn.before[at]) {
			draw.Fatalf("number %d: %v, written %v", at, read[at],
				drawn.before[at])
		}
	}
	for at := range orbit {
		if !same(orbit[at], drawn.after[at]) {
			draw.Fatalf("orbit %d: %v, written %v", at, orbit[at],
				drawn.after[at])
		}
	}
	if source, known := star.GaiaID(); source != drawn.source || !known {
		draw.Fatalf("source %d %v, written %d", source, known, drawn.source)
	}
}
