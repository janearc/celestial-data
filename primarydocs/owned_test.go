package primarydocs

import (
	"strings"
	"testing"
)

// A document at a host that has an adapter of its own is refused, with
// the adapter's verb named, before anything is asked: arXiv's hosts and
// ADS's, whatever their case, and any host under them. A host that only
// ends in the same letters is not one of them.
func TestADocumentAtAnAdaptersOwnHostIsRefused(t *testing.T) {
	asked := &fakeWeb{body: "%PDF-1.7\n%%EOF", length: 14}
	for address, through := range map[string]string{
		"https://arxiv.org/pdf/2408.14425v1":             "papers",
		"https://export.arxiv.org/abs/2408.14425":        "papers",
		"https://ARXIV.org/pdf/2408.14425":               "papers",
		"https://ui.adsabs.harvard.edu/abs/2021AJ/x.pdf": "ads",
		"https://api.adsabs.harvard.edu/v1/search/query": "ads",
	} {
		d := Document{ID: "one", Name: "one.pdf", URL: address, Title: "a paper", Kind: "pdf"}
		_, err := Fetch(asked, d, t.TempDir())
		if err == nil || !strings.Contains(err.Error(), "an adapter of its own") ||
			!strings.HasSuffix(err.Error(), "ask it through "+through) {
			t.Errorf("%s: want a refusal naming %s, got %v", address, through, err)
		}
	}
	if asked.calls != 0 {
		t.Fatalf("a refused document was asked for %d times", asked.calls)
	}
	for _, address := range []string{
		"https://notarxiv.org/a.pdf",
		"https://docs.unidata.ucar.edu/tds/current/userguide/a.pdf",
		"https://harvard.edu/a.pdf",
	} {
		d := Document{ID: "one", Name: "one.pdf", URL: address, Title: "a page", Kind: "pdf"}
		if _, err := d.Product(); err != nil {
			t.Errorf("%s: refused: %v", address, err)
		}
	}
}
