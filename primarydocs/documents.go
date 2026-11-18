// Package primarydocs retains explicitly catalogued primary research documents.
// It does no search and accepts no arbitrary URL from the command line.
package primarydocs

import (
	"bytes"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	data "github.com/janearc/celestial-data"
	"github.com/janearc/celestial-data/web"
)

// Document names one public document at its primary host, chosen by a reader.
type Document struct {
	ID    string `yaml:"ID" json:"ID"`
	Name  string `yaml:"Name" json:"Name"`
	URL   string `yaml:"URL" json:"URL"`
	Title string `yaml:"Title" json:"Title"`
	Kind  string `yaml:"Kind" json:"Kind"`
}

// Product refuses untrusted schemes, credential-bearing URLs, non-plain file
// names and unsupported formats before it asks a network for anything.
func (d Document) Product() (*data.Product, error) {
	u, err := url.Parse(d.URL)
	if err != nil || u.Scheme != "https" || u.Hostname() == "" ||
		u.User != nil || u.Fragment != "" {
		return nil, fmt.Errorf("document %q needs a public HTTPS URL "+
			"without credentials or fragment", d.ID)
	}
	if own := adapterOf(u.Hostname()); own != "" {
		return nil, fmt.Errorf("document %q is at %s, which has an "+
			"adapter "+
			"of its own: ask it through %s", d.ID,
			u.Hostname(), own)
	}
	if d.ID == "" || d.Title == "" {
		return nil, fmt.Errorf("document needs an ID and title")
	}
	if d.Kind != "pdf" && d.Kind != "html" &&
		d.Kind != "text" {
		return nil, fmt.Errorf("unsupported document kind %q", d.Kind)
	}
	if filepath.Ext(d.Name) != "."+d.Kind &&
		!(d.Kind == "text" && filepath.Ext(d.Name) == ".txt") {
		return nil, fmt.Errorf("filename and kind disagree")
	}
	a, _ := data.Named("primarydocs")
	p, err := a.Product(d.ID, d.Name, d.URL, data.Unsized, d.Title)
	return p.Checked(documentCheck(d.Kind)), err
}

// owned maps hosts celestial-data asks through an adapter of its own to
// that adapter's verb. Each adapter keeps rules for how its host is asked;
// an address catalogue would walk past those rules, so it is refused here.
// primarydocs does not overlap with arxiv or any other adapter carried.
var owned = map[string]string{
	"arxiv.org":          "papers",
	"adsabs.harvard.edu": "ads",
}

// adapterOf is the verb of the adapter that owns a host, or none: the
// host itself or any host under it, whatever its case.
func adapterOf(host string) string {
	host = strings.ToLower(strings.TrimSuffix(host, "."))
	for name, verb := range owned {
		if host == name || strings.HasSuffix(host, "."+name) {
			return verb
		}
	}
	return ""
}

type documentCheck string

// Check rejects challenge/error HTML masquerading as a PDF, truncated PDFs,
// and obvious error/challenge pages. This is format integrity, not a claim
// that a server's document is academically correct or licensed for reuse.
func (kind documentCheck) Check(path string) error {
	b, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	if len(b) == 0 {
		return fmt.Errorf("empty document")
	}
	switch kind {
	case "pdf":
		if !bytes.HasPrefix(b, []byte("%PDF-")) ||
			!bytes.Contains(b[max(0, len(b)-4096):],
				[]byte("%%EOF")) {
			return fmt.Errorf("not a complete PDF")
		}
	case "html":
		lower := strings.ToLower(string(b))
		if !strings.Contains(lower, "<html") &&
			!strings.Contains(lower, "<!doctype html") {
			return fmt.Errorf("not HTML")
		}
		for _, marker := range []string{"<title>just a moment",
			"<title>access denied", "<title>404",
			"<title>page not found"} {
			if strings.Contains(lower, marker) {
				return fmt.Errorf(
					"server supplied a challenge/" +
						"error page")
			}
		}
	case "text":
		if bytes.Contains(b, []byte{0}) {
			return fmt.Errorf("text contains NUL")
		}
	}
	return nil
}

// Fetch uses celestial-data's common atomic, length-checked fetch. Existing
// files are refused; no authenticated session is imported or retained.
func Fetch(get web.Web, d Document, into string) (string, error) {
	p, err := d.Product()
	if err != nil {
		return "", err
	}
	return data.Fetch(get, p, into)
}
