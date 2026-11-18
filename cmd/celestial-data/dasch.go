package main

import (
	"maps"

	"github.com/janearc/celestial-data/dasch"
	"github.com/janearc/celestial-data/web"
)

// hostPolicies are the hosts that have numbers of their own, by host. Any
// other host is asked at the web package's defaults. It is an error when
// the numbers of a host cannot be read, and the run does not start.
//
// The numbers are embedded in the packages that own them, so a read that
// fails means the embedded numbers are wrong, and a run asks no host with
// a policy it cannot read.
func hostPolicies() (map[string]web.Policy, error) {
	daschPolicy, err := dasch.Policy()
	if err != nil {
		return nil, err
	}
	policies := map[string]web.Policy{dasch.Host: daschPolicy}
	maps.Copy(policies, ntrsPolicies())
	return policies, nil
}

// gentleDoer is a doer asked through the tool's hosts: paced, stopped
// when Starglass asks, retried when it says nothing, its wait kept in
// the cache folder, and a stop naming --i-mean-it.
//
// A tool with no hosts, as in a test, asks the doer as it is.
func (tool command) gentleDoer(under dasch.Doer) dasch.Doer {
	if tool.hosts == nil {
		return under
	}
	// the doer and the hosts are both here, which is all it refuses
	through, _ := web.Through(under, tool.hosts)
	return web.RewritingDoer(through, nameFlag)
}
