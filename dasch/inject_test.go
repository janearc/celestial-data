//ninja:vow dojo the asker's refusals before it sends, through newAsker
package dasch

import (
	"bytes"
	"errors"
	"io"
	"net/http"
	"os"
	"strings"
	"testing"

	"go.uber.org/mock/gomock"

	"github.com/janearc/celestial-data"
	"github.com/janearc/celestial-data/mocks"
)

// These tests reach the refusals no answer can bring about, since the
// adapter's own constants are sound: each is made to fire by giving
// the code what it would refuse. None asks the network.

// anyAgent and anyMost are any agent and any count will do.
const (
	anyAgent = "celestial-data-tests"
	anyMost  = 1
)

// A name that is not one word is refused, as Named refuses it.
func TestAnArchiveOfAnotherNameIsRefusedIfItIsNotOneWord(t *testing.T) {
	doer := mocks.NewMockDoer(gomock.NewController(t))
	found, err := build("two words", doer, anyAgent, anyMost)
	if found != nil || !errors.Is(err, celestialdata.Refused()) {
		t.Errorf("%v, %v", found, err)
	}
}

// A body that cannot be written as JSON is an error, and nothing is sent.
func TestABodyThatIsNotJSONIsNotSent(t *testing.T) {
	doer := mocks.NewMockDoer(gomock.NewController(t))
	asking := newAsker(doer, anyAgent)
	listed, err := asking.post(queryPath, make(chan int))
	if listed != nil || err == nil {
		t.Errorf("%v, %v", listed, err)
	}
}

// A root that is no address makes a request that cannot be made, and
// nothing is sent.
func TestARootThatIsNoAddressSendsNothing(t *testing.T) {
	doer := mocks.NewMockDoer(gomock.NewController(t))
	asking := newAsker(doer, anyAgent)
	asking.root = "://no"
	listed, err := asking.post(queryPath, curve{})
	if listed != nil || err == nil {
		t.Errorf("%v, %v", listed, err)
	}
}

// A catalogue whose name is no plain file name makes a product that is
// refused, and Find says so with the archive's name.
func TestACatalogueThatNamesNoPlainFileIsRefused(t *testing.T) {
	raw, err := os.ReadFile("testdata/querycat.json")
	if err != nil {
		t.Fatal(err)
	}
	doer := mocks.NewMockDoer(gomock.NewController(t))
	doer.EXPECT().Do(gomock.Any()).Return(&http.Response{
		StatusCode: http.StatusOK, Header: http.Header{},
		Body: io.NopCloser(bytes.NewReader(raw))}, nil)
	archive, err := New(doer, anyAgent, anyMost)
	if err != nil {
		t.Fatal(err)
	}
	archive.refcat = "../apass"
	centre, err := celestialdata.NewPosition(287.50086, -22.23481)
	if err != nil {
		t.Fatal(err)
	}
	near, err := celestialdata.NewNear(centre, 0.0014)
	if err != nil {
		t.Fatal(err)
	}
	products, err := archive.Find(near)
	if products != nil || !errors.Is(err, celestialdata.Refused()) ||
		!strings.HasPrefix(err.Error(), "dasch: ") ||
		!strings.Contains(err.Error(), "not one plain file name") {
		t.Errorf("%v, %v", products, err)
	}
}
