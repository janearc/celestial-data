# When a download does not come

Check which build you run: `fetch-set grissom` arrived in `de05f2c`,
followed by Chroma's current GitHub listing in `3c042c3`. Earlier
checkouts cannot fetch the complete set in one command; rebuild after
bringing those commits into your branch.

```sh
game build
bin/celestial-data fetch-set grissom --into /your/data/grissom
```

Run it again after a failure. Completed files stay, are reported as
kept, and are not downloaded again. A failed transfer's own `.part`
is removed. There is no cross-run resume yet: a failed large file starts
again from the beginning.

A surviving `.part` can belong to another running fetch, so check
before removing it.

## NASA's Earth images

On 2026-11-02, connections to `eoimages.gsfc.nasa.gov` timed out over
both IPv4 and IPv6, before TLS. Curl reproduced the failure outside
the process sandbox, on both HTTP and HTTPS. That does not establish
that the host is unreachable from every network.

NASA's pages checked on 2026-11-02 link downloads on `assets.science.nasa.gov`:

- [Blue Marble base maps](https://science.nasa.gov/earth/earth-observatory/blue-marble-next-generation/base-map/)
- [Black Marble flat maps](https://science.nasa.gov/earth/earth-observatory/earth-at-night/maps/)

The adapter now uses those links. July is under
`eo/images/bmng/bmng-base/july/`; Black Marble is under
`eo/images/imagerecords/144000/144898/`. Changing just the host and keeping
July's old image-record path returns 404.

The expected lengths did not change. Download addresses and hashes from
the 2026-11-02 fetch are recorded in `bluemarble/sources.yaml`; no image
is shipped. The downloader checks lengths, not those recorded hashes.

The hashes support a manual check with `shasum -a 256`. Earlier checksum
records are absent from this repository, so it cannot independently
establish that the fresh files match the earlier local copies.

## A server may work before it has anything to say

SkyView generates a whole-sky map before sending its response headers.
The IRIS 100 request at 1800 by 900 pixels took 74 seconds to send its
first byte, then completed at 79 seconds, on 2026-11-02. The old
30-second wait rejected that working request.

The picked wait in `cmd/celestial-data/numbers.yaml` is now 120 seconds.
It applies to connection, TLS handshake, and response headers. There is
still no deadline for the whole body: a slow, large file may take as
long as it needs after the headers arrive.

`timeout awaiting response headers` means this wait was exceeded;
it does not mean a partial FITS file was accepted. For a stubborn
failure, measure the exact URL with curl's `time_starttransfer` and
`time_total` before raising the wait again.

## Slow files and interrupted files

The two infrared mosaics are 504,051,840 bytes each. The MeerKAT
intensity mosaic is 852,356,160 bytes. The fetcher reports a completed
file, not progress during its body. A growing `.part` means the current
fetch is still receiving bytes; silence alone is not failure.

On the same network, one-MiB ranged MeerKAT transfers through curl
took 8.3 and 12.0 seconds. The server negotiated HTTP/1.1 even when
HTTP/2 was requested. Those bytes matched the beginning of the local
mosaic.

These probes establish slow access to the right file, not that a full
transfer will complete or that the archive has a fixed speed.

One full attempt ended with `unexpected EOF`. A retry is appropriate;
the error means the response ended before its promised length. The
fetcher refuses that file and removes its partial output. It does not
currently use the server's byte-range support to recover that work.

An interrupted transfer and a connection or TLS timeout are different
failures. Keep the exact error and URL. Increasing a header wait will
not repair a body that was cut short, and a successful range probe is
not a completed mosaic download.
