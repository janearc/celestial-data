// Package caltech retains named public documents from Caltech hosts.
package caltech

import (
	"fmt"
	"net/url"
	"strings"

	"github.com/janearc/celestial-data/primarydocs"
	"github.com/janearc/celestial-data/web"
)

// Check refuses entries outside this adapter's scope.
func Check(document primarydocs.Document) error {
	address, err := url.Parse(document.URL)
	if err != nil || address.Scheme != "https" || address.User != nil ||
		address.Fragment != "" {
		return fmt.Errorf("caltech: document needs an HTTPS address without credentials or fragment")
	}
	host := strings.ToLower(strings.TrimSuffix(address.Hostname(), "."))
	if host != "caltech.edu" && !strings.HasSuffix(host, ".caltech.edu") {
		return fmt.Errorf("caltech: document host %q is not caltech.edu", host)
	}
	if address.Port() != "" && address.Port() != "443" {
		return fmt.Errorf("caltech: document needs the HTTPS port")
	}
	_, err = document.Product()
	return err
}

// Fetch retains an accepted document through the caller's web.
func Fetch(get web.Web, document primarydocs.Document, into string) (string, error) {
	if err := Check(document); err != nil {
		return "", err
	}
	return primarydocs.Fetch(get, document, into)
}
