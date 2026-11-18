package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/janearc/celestial-data/web"
	"github.com/janearc/puzzlemouth/pzm"
	"github.com/janearc/puzzlemouth/verb"
)

// insistFlag is the name of the switch that asks a host again before its
// stop has passed. Every verb that fetches from a server declares it
// through insisting.
const insistFlag = "i-mean-it"

// insisting is the flags of a verb that fetches from a server: the one
// switch that overrides a stop. It is declared here once, and added to
// each such verb as searching is.
//
// The verbs that fetch from a server through the web are find, fetch,
// fetch-set, papers and ads. find and fetch ask Starglass through the
// same hosts, and ads asks through the gentle of its own package.
func insisting() []pzm.Flag {
	return []pzm.Flag{
		pzm.Switch(insistFlag, verb.NoShort, "asks a host again "+
			"before its stop has passed; says what it asked first"),
	}
}

// cacheFolder is celestial-data's folder in the user's cache folder, or
// nothing when the user has none.
func cacheFolder() string {
	cache, err := os.UserCacheDir()
	if err != nil {
		return ""
	}
	return filepath.Join(cache, agent)
}

// hostsOf is the gentles of the hosts a run asks, each keeping its record
// in the folder, or none kept when there is no folder. A host with numbers
// of its own is asked by them: see hostPolicies. It is an error when one
// of those numbers cannot be read.
func hostsOf(folder string) (*web.Hosts, error) {
	return hostsOn(folder, web.Wall(), web.Unix())
}

// hostsOn is the gentles of the hosts a run asks, on a clock and a source
// of jitter a test may give. A host that has a policy of its own is asked
// by it; every other is asked by the default.
func hostsOn(folder string, clock web.Clock,
	random web.Random) (*web.Hosts, error) {
	policies, err := hostPolicies()
	if err != nil {
		return nil, err
	}
	return web.NewHosts(folder, clock, random, policies), nil
}

// insist has every host a run asks insist, when the call gave the
// switch. What each host asked for is said on the tool's stderr. It is
// an error when the tool has no stderr to say it on.
func (tool command) insist(call pzm.Call) error {
	if !call.On(insistFlag) || tool.hosts == nil {
		return nil
	}
	return tool.hosts.Insist(tool.errs)
}

// insistedWeb is the web a call asks through, after the call's switch has
// been given to the hosts. It refuses when the switch cannot be given.
func (tool command) insistedWeb(call pzm.Call) (web.Web, error) {
	if err := tool.insist(call); err != nil {
		return nil, err
	}
	return tool.web()
}

// saysNoCache says on the tool's stderr that there is no cache folder, so
// no waits are kept between runs. A tool with no stderr says nothing.
func (tool command) saysNoCache() {
	if !tool.noCache || tool.errs == nil {
		return
	}
	fmt.Fprintln(tool.errs, "celestial-data: no cache folder was found, "+
		"so waits will not be kept between runs")
}

// announcing is a fetching verb that says first that there is no cache
// folder, when the run has none.
func (tool command) announcing(
	body func(pzm.Call) error) func(pzm.Call) error {
	return func(call pzm.Call) error {
		tool.saysNoCache()
		return body(call)
	}
}

// fetchingWithOverride gives a retention verb the override it declares,
// and says when no cache folder was found, before the verb asks a host.
func (tool command) fetchingWithOverride(
	body func(pzm.Call) error) func(pzm.Call) error {
	return tool.announcing(func(call pzm.Call) error {
		if err := tool.insist(call); err != nil {
			return err
		}
		return body(call)
	})
}

// release puts away what the hosts asked kept in their records, so the
// next run keeps their paces and waits. A tool with no hosts has none.
func (tool command) release() error {
	if tool.hosts == nil {
		return nil
	}
	return tool.hosts.Release()
}

// finish is the code a run ends with, after its hosts are released: the
// verb's own, or a failure when the verb went well and the records did
// not write.
func (tool command) finish(code int, errs io.Writer) int {
	err := tool.release()
	if err == nil {
		return code
	}
	fmt.Fprintf(errs, "celestial-data: %v\n", err)
	return max(code, exitFailed)
}

// switchNamed is an error that ends by naming the switch that overrides
// it.
type switchNamed struct {
	stop error
}

// Error is the stop, and how to ask anyway.
func (naming switchNamed) Error() string {
	return naming.stop.Error() + "; to ask anyway, add --" + insistFlag
}

// Unwrap is the stop, so a caller finds the *web.Stopped in it.
func (naming switchNamed) Unwrap() error { return naming.stop }

// nameFlag is an error that ends by naming --i-mean-it when it holds a
// stop the switch may override. A stop that was overridden already, or
// whose switch is set, is given as it is: the switch is not the way on.
// An error that already names the switch is given as it is too.
func nameFlag(err error) error {
	var named switchNamed
	if errors.As(err, &named) {
		return err
	}
	var stopped *web.Stopped
	if !errors.As(err, &stopped) || stopped.Tried || stopped.Insisting {
		return err
	}
	return switchNamed{stop: err}
}

// gentle is the web, asked through the gentles of its hosts when the tool
// has them: each host one request at a time, at a pace, left alone when
// it asks, its record kept in the cache folder. Without hosts, as in a
// test, it is the web as it is. Its stops end by naming the switch.
func (tool command) gentle(bare web.Web) web.Web {
	if tool.hosts == nil {
		return bare
	}
	// the context, web and hosts are all here, which is all it refuses
	within, _ := web.Within(context.Background(), bare, tool.hosts)
	return web.Rewriting(within, nameFlag)
}
