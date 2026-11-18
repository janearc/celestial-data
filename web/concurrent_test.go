package web_test

import (
	"bytes"
	"io"
	"sync"
	"testing"

	"github.com/janearc/celestial-data/mocks"
	"github.com/janearc/celestial-data/web"
	"go.uber.org/mock/gomock"
)

// askers is how many goroutines ask together in these tests. It's
// defined: enough to meet at a gate, few enough to run in a blink.
const askers = 8

// together runs a function in many goroutines and waits for all of them.
func together(count int, run func(which int)) {
	var done sync.WaitGroup
	for which := range count {
		done.Go(func() { run(which) })
	}
	done.Wait()
}

// Many goroutines asking one gentle go one at a time, all are answered,
// each is counted, and a Release, an Insist and a read of Asked in the
// middle of them are safe. The test is run under the race detector.
func TestManyGoroutinesAskingOneGentleAreAnsweredOneAtATime(t *testing.T) {
	path := recordIn(t)
	wrapped, inner, gentle, keeping := within(t, example(t))
	if err := gentle.Hold(path); err != nil {
		t.Fatal(err)
	}
	inner.EXPECT().Get(address).DoAndReturn(func(string) (io.ReadCloser,
		int64, error) {
		if web.Holding(gentle) {
			return answer(), 1, nil
		}
		t.Error("a request is in the air and the gentle is not held")
		return answer(), 1, nil
	}).Times(askers)
	together(askers+3, func(which int) {
		switch {
		case which == askers:
			gentle.Insist(&bytes.Buffer{})
		case which == askers+1:
			gentle.Release()
		case which == askers+2:
			gentle.Asked()
		default:
			body, _, err := wrapped.Get(address)
			if err != nil {
				t.Error(err)
				return
			}
			body.Close()
		}
	})
	if gentle.Asked() != askers || web.Holding(gentle) {
		t.Errorf("asked %d, held %v", gentle.Asked(), web.Holding(gentle))
	}
	// each request after the first waited out the pace
	if len(keeping.slept) != askers-1 {
		t.Errorf("%v", keeping.slept)
	}
}

// A Leave with nothing held, or a second one, does nothing, and does not
// wait for ever.
func TestALeaveWithNothingHeldDoesNothing(t *testing.T) {
	gentle, _ := exampleAt(t)
	gentle.Leave()
	if err := gentle.Enter(t.Context()); err != nil {
		t.Fatal(err)
	}
	gentle.Leave()
	gentle.Leave()
	if web.Holding(gentle) {
		t.Error("held")
	}
	askOnce(t, gentle)
}

// Many goroutines asking many hosts of one Hosts get one gentle a host,
// and Insist and Release in the middle of them are safe.
func TestManyGoroutinesAskingOneHostsMakeOneGentleAHost(t *testing.T) {
	folder := t.TempDir()
	clock, _ := clockAt(t, start)
	hosts := web.NewHosts(folder, clock, randomOf(t, half), nil)
	inner := mocks.NewMockWeb(gomock.NewController(t))
	wrapped, err := web.Within(t.Context(), inner, hosts)
	if err != nil {
		t.Fatal(err)
	}
	names := []string{"https://one.example.net/a", "https://two.example.net/a",
		"https://ONE.example.net./b"}
	inner.EXPECT().Head(gomock.Any()).Return(int64(1), nil).Times(askers)
	together(askers+2, func(which int) {
		switch {
		case which == askers:
			hosts.Insist(&bytes.Buffer{})
		case which == askers+1:
			hosts.Release()
		default:
			if _, err := wrapped.Head(names[which%len(names)]); err != nil {
				t.Error(err)
			}
		}
	})
	one, err := hosts.For(names[0])
	if err != nil {
		t.Fatal(err)
	}
	if again, _ := hosts.For(names[2]); again != one {
		t.Error("one host has two gentles")
	}
}
