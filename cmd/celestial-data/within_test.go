package main

import (
	"testing"
	"time"

	celestialdata "github.com/janearc/celestial-data"
	"github.com/janearc/celestial-data/mocks"
	"github.com/janearc/celestial-data/web"
	"go.uber.org/mock/gomock"
)

// A verb that holds each whole request to a time asks through the web
// a test gives it, and with none, through the network: a client made
// from the numbers' wait. Without the numbers it has no web and says
// so.
func TestAVerbHeldToATimeAsksThroughTheWebItIsGiven(t *testing.T) {
	given := mocks.NewMockWeb(gomock.NewController(t))
	through, err := command{get: given}.within(documentsWhole)
	if err != nil || through != web.Web(given) {
		t.Fatalf("given a web, it asks through %v, %v", through, err)
	}
	picked := celestialdata.NewMeasured(numbersFile, numbersRaw)
	network, err := command{picked: picked}.within(crmWhole)
	if _, made := network.(web.Client); err != nil || !made {
		t.Fatalf("given none, it asks through %T, %v", network, err)
	}
	empty := celestialdata.NewMeasured("none.yaml", []byte("numbers: []\n"))
	if _, err := (command{picked: empty}).within(time.Second); err == nil {
		t.Fatal("with no wait among the numbers, it made a web")
	}
}
