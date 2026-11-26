# caltech documents

this adapter retains named public documents from Caltech HTTPS hosts. the
caller supplies a catalogue entry with an ID, filename, title, URL and
kind. it refuses other hosts, credentials, fragments, unsupported kinds and
unsafe names before making a request. the documents are kept as received;
page content and transient tokens are not normalized.

`celestial-data caltech` uses the same catalogue, provenance record,
per-host request gate and stop handling as `documents`. it does not search,
authenticate, bypass a refusal or promise a stable hash for a live page.
the command refuses redirects; select a permitted destination explicitly
instead of requesting a different host without its gate.
the fetched bytes and their hash identify one observation, not a permanent
publisher version. check the publisher's reuse terms before redistribution.
