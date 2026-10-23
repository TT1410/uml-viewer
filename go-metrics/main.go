// Command go-metrics writes the uml-viewer `.metrics` snapshots for a Go module.
//
// crap4go and mutate4go print reports keyed by the short package name, which
// is ambiguous in a large module (many packages are called `app` or `domain`).
// The viewer joins snapshots by namespace, so this command computes the same
// numbers keyed by the full import path, dotted:
//
//   - .metrics/crap.edn: cyclomatic complexity (counted as crap4go counts it),
//     coverage from a Go cover profile, and CRAP = cc²·(1−cov)³ + cc;
//   - .metrics/mutate/<namespace>.edn: killed, survived and uncovered mutants per
//     function, read from saved mutate4go output (one log per source file).
//
// Usage:
//
//	go-metrics -module backend -src backend/internal -profile backend/target/coverage/coverage.out \
//	  -mutate-logs .metrics/go-mutate -out .metrics
package main

import (
	"flag"
	"fmt"
	"os"
)

func main() {
	opts := options{}
	flag.StringVar(&opts.module, "module", ".", "directory with go.mod")
	flag.StringVar(&opts.src, "src", "", "only packages under this directory (default: the module)")
	flag.StringVar(&opts.profile, "profile", "", "Go cover profile (go test -coverprofile)")
	flag.StringVar(&opts.mutateLogs, "mutate-logs", "", "directory of saved mutate4go output, one .log per source file")
	flag.StringVar(&opts.out, "out", ".metrics", "directory that receives crap.edn and mutate/")
	flag.Parse()
	if err := run(opts); err != nil {
		fmt.Fprintln(os.Stderr, "go-metrics:", err)
		os.Exit(1)
	}
}
