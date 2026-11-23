module github.com/janearc/celestial-data

go 1.27.3

require (
	github.com/Epistemic-Technology/arxiv v1.0.1
	github.com/janearc/celestial v0.0.10
	github.com/janearc/puzzlemouth v0.1.3
	// scigolib/netcdf at upstream's commit 5be112aa81c3 of 20 November
	// 2026. Upstream has no tags; a tag, when it cuts one, replaces this.
	github.com/scigolib/netcdf v0.0.0-20261120113335-5be112aa81c3
	go.uber.org/mock v0.6.0
	pgregory.net/rapid v1.3.0
)

require (
	github.com/inconshreveable/mousetrap v1.1.0 // indirect
	github.com/spf13/cobra v1.10.2 // indirect
	github.com/spf13/pflag v1.0.9 // indirect
	golang.org/x/mod v0.27.0 // indirect
	golang.org/x/sync v0.16.0 // indirect
	golang.org/x/sys v0.49.0 // indirect
	golang.org/x/term v0.47.0 // indirect
	golang.org/x/time v0.6.0 // indirect
	golang.org/x/tools v0.36.0 // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
)

tool go.uber.org/mock/mockgen
