package dasch

import "github.com/janearc/celestial-data"

// pagesName is the archive of DASCH's own pages, as the command line
// takes it.
const pagesName = "dasch-pages"

// dr7 is where DASCH keeps its DR7 pages.
const dr7 = "https://dasch.cfa.harvard.edu/dr7/"

// The pages the adapter's wire contract was read from on 2026-11-08: the
// web APIs (the two endpoints, their bodies and answers), the data
// products (what a light curve is), and the light curve's columns.
const (
	webAPIsName      = "dasch-dr7-web-apis.html"
	dataProductsName = "dasch-dr7-data-products.html"
	curveColumnsName = "dasch-dr7-lightcurve-columns.html"
	webAPIsAddress   = dr7 + "web-apis/"
	dataProductsAddr = dr7 + "data-products/"
	curveColumnsAddr = dr7 + "lightcurve-columns/"
	pagesAbout       = "a DASCH DR7 page the adapter's contract was read from"
)

// Pages is the archive of DASCH's own pages the adapter was built from,
// asked by name only. They are fetched to be kept as the record of what
// the adapter was read from, in cited-sources, which .game holds out of
// every release: they are Harvard's.
func Pages() (*celestialdata.Fixed, error) {
	return celestialdata.NewFixed(pagesName,
		celestialdata.NewFile("web apis", webAPIsName, webAPIsAddress,
			celestialdata.Unsized, pagesAbout),
		celestialdata.NewFile("data products", dataProductsName,
			dataProductsAddr, celestialdata.Unsized, pagesAbout),
		celestialdata.NewFile("light curve columns", curveColumnsName,
			curveColumnsAddr, celestialdata.Unsized, pagesAbout))
}
