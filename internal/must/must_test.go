package must_test

import (
	"errors"
	"testing"

	"github.com/janearc/celestial-data/internal/must"
)

// made is what the tests make.
const made = "made"

// errWrong is the error of a value not made.
var errWrong = errors.New("declared wrong")

// the test: a value made is given back.
func TestAValueMadeIsGivenBack(t *testing.T) {
	if got := must.Be(made, nil); got != made {
		t.Errorf("%q", got)
	}
}

// the anti-test: a value not made panics with its error, and is not
// given back.
func TestAValueNotMadePanicsWithItsError(t *testing.T) {
	defer panickedWithWrong(t)
	must.Be(made, errWrong)
	t.Error("a value not made was given back")
}

// panickedWithWrong fails the test unless it is ending in a panic with
// errWrong.
func panickedWithWrong(t *testing.T) {
	got, isError := recover().(error)
	if !isError || !errors.Is(got, errWrong) {
		t.Errorf("panicked with %v", got)
	}
}
