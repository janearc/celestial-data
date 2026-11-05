package celestialdata

import (
	"fmt"
	"html"
	"io"
	"regexp"

	"github.com/janearc/celestial-data/web"
)

// href finds a link's address in a page. The address may be in double
// quotes, in single quotes or in none. Each way is its own group.
var href = regexp.MustCompile(`(?i)\bhref\s*=\s*(?:"([^"]*)"|'([^']*)'|` +
	`([^\s"'>]+))`)

// The counts the link reading works with.
const (
	firstGroup = 1  // defined by regexp: the groups follow the whole match
	every      = -1 // defined by regexp: no limit on the matches found
	empty      = 0  // defined: a group that matched nothing
)

// links are the addresses a page's links go to, in the order written. Their
// entities are read.
func links(page []byte) []string {
	var found []string
	for _, match := range href.FindAllSubmatch(page, every) {
		for _, group := range match[firstGroup:] {
			if len(group) > empty {
				link := html.UnescapeString(string(group))
				found = append(found, link)
			}
		}
	}
	return found
}

// listing is the links on the page at an address. Archives list a
// folder's files this way, through an adapter's Links.
func listing(get web.Web, address string) ([]string, error) {
	body, _, err := get.Get(address)
	if err != nil {
		return nil, err
	}
	defer body.Close()
	page, err := io.ReadAll(body)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", address, err)
	}
	return links(page), nil
}
