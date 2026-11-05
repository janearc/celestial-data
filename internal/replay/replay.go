// Package replay answers an adapter's questions with what an archive
// really said. The replies are recorded in the adapter's testdata, so its
// tests ask nothing of the network.
package replay

import (
	"bytes"
	"compress/gzip"
	_ "embed"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/janearc/celestial-data"
	"github.com/janearc/celestial-data/mocks"
	"github.com/janearc/celestial-data/web"
	"go.uber.org/mock/gomock"
)

// askedFile is the numbers file that says where the replies asked about.
const askedFile = "asked.yaml"

//go:embed asked.yaml
var askedRaw []byte

// asked is the numbers file beside replay, read.
func asked() *celestialdata.Measured {
	return celestialdata.NewMeasured(askedFile, askedRaw)
}

// Asked is the circle every recorded reply asked about.
//
//ninja:vow practice test support: the adapters' tests are its callers
func Asked(test testing.TB) *celestialdata.Near {
	test.Helper()
	circle, err := asked().Circle("asked-ra", "asked-dec", "asked-radius")
	stopOn(test, err)
	return circle
}

// stopOn stops the test when there is an error, and says it.
func stopOn(test testing.TB, err error) {
	test.Helper()
	if err != nil {
		test.Fatalf("replay: %v", err)
	}
}

// Web is a mock web that answers a Get of each address once. The answer
// is the recorded file named for it under testdata, with its length. Any
// other question fails the test, and so does a file that is not there.
//
//ninja:vow practice test support: the adapters' tests are its callers
func Web(test testing.TB, replies map[string]string) *mocks.MockWeb {
	test.Helper()
	get := mocks.NewMockWeb(gomock.NewController(test))
	for address, file := range replies {
		recorded, err := os.ReadFile(filepath.Join("testdata", file))
		if err != nil {
			stopOn(test, err)
			return nil
		}
		body := io.NopCloser(bytes.NewReader(recorded))
		length := int64(len(recorded))
		get.EXPECT().Get(address).Return(body, length, nil)
	}
	return get
}

// agent is what a live check says it is, to the archives it asks.
const agent = "celestial-data-live-check"

// Live is a client for the live checks, which ask a real archive. Its
// waits are picked in the numbers file beside the asked circle.
//
//ninja:vow practice test support: the adapters' live checks are its callers
func Live(test testing.TB) *web.Client {
	test.Helper()
	wait, waitErr := asked().Get("live-wait")
	whole, wholeErr := asked().Get("live-whole")
	stopOn(test, errors.Join(waitErr, wholeErr))
	client := web.New(web.Seconds(wait), web.Seconds(whole), agent)
	return &client
}

// photons is what a fetched test file holds.
const photons = "photons"

// Fetched is what Fetch makes of a product whose address answers with a
// small file, gzip when its name ends in .gz. Each adapter's test sees the
// size the adapter gave its products this way. Only Fetch reads it.
//
//ninja:vow practice test support: the adapters' tests are its callers
func Fetched(test testing.TB, product *celestialdata.Product) error {
	test.Helper()
	var content bytes.Buffer
	if strings.HasSuffix(product.Name(), ".gz") {
		packed := gzip.NewWriter(&content)
		io.WriteString(packed, photons)
		packed.Close()
	} else {
		content.WriteString(photons)
	}
	length := int64(content.Len())
	get := mocks.NewMockWeb(gomock.NewController(test))
	body := io.NopCloser(&content)
	get.EXPECT().Get(product.URL()).Return(body, length, nil)
	_, err := celestialdata.Fetch(get, product, test.TempDir())
	return err
}
