# ads/testdata

These answers are crafted for the tests, in the shape ADS's developer
documentation gives for its search endpoint. None was recorded from ADS:
there was no token to ask with. The bibcodes, titles and authors are
made up.

- two-papers.json: two papers, the first with three authors and three
  identifiers, one of them an arXiv id written as ADS writes it, after
  an arXiv-shaped bibcode that is not one; the second with none.
- bare-paper.json: one paper with no title and no author.
- none-found.json: an answer that found nothing.
- no-response.json: valid JSON without the response ADS gives.
- not-json.txt: not JSON at all.
