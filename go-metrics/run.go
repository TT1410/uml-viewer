package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

type options struct {
	module, src, profile, mutateLogs, out string
}

func run(opts options) error {
	modDir, err := filepath.Abs(opts.module)
	if err != nil {
		return err
	}
	modPath, err := modulePath(filepath.Join(modDir, "go.mod"))
	if err != nil {
		return err
	}
	src := opts.src
	if src == "" {
		src = modDir
	}
	if src, err = filepath.Abs(src); err != nil {
		return err
	}
	fns, err := moduleFunctions(modDir, modPath, src)
	if err != nil {
		return err
	}
	if err := writeCrap(opts, fns, modPath); err != nil {
		return err
	}
	return writeMutate(opts, fns)
}

var moduleLine = regexp.MustCompile(`(?m)^\s*module\s+"?([^\s"]+)"?`)

func modulePath(goMod string) (string, error) {
	text, err := os.ReadFile(goMod)
	if err != nil {
		return "", err
	}
	m := moduleLine.FindSubmatch(text)
	if m == nil {
		return "", errors.New("no module line in " + goMod)
	}
	return string(m[1]), nil
}

func writeCrap(opts options, fns []function, modPath string) error {
	if opts.profile == "" {
		return nil
	}
	f, err := os.Open(opts.profile)
	if err != nil {
		return err
	}
	defer f.Close()
	entries := crapEntries(fns, parseProfile(f), modPath)
	if err := os.MkdirAll(opts.out, 0o755); err != nil {
		return err
	}
	fmt.Printf("crap.edn: %d functions\n", len(entries))
	return os.WriteFile(filepath.Join(opts.out, "crap.edn"), []byte(crapEDN(entries)), 0o644)
}

func writeMutate(opts options, fns []function) error {
	if opts.mutateLogs == "" {
		return nil
	}
	runs, err := readRuns(opts.mutateLogs)
	if err != nil {
		return err
	}
	dir := filepath.Join(opts.out, "mutate")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	snaps := mutateSnapshots(runs, fns)
	fmt.Printf("mutate/: %d packages from %d runs\n", len(snaps), len(runs))
	for ns, forms := range snaps {
		if err := os.WriteFile(filepath.Join(dir, ns+".edn"), []byte(mutateEDN(ns, forms)), 0o644); err != nil {
			return err
		}
	}
	return nil
}

func readRuns(dir string) ([]mutateRun, error) {
	paths, err := filepath.Glob(filepath.Join(dir, "*.log"))
	if err != nil {
		return nil, err
	}
	sort.Strings(paths)
	var runs []mutateRun
	for _, p := range paths {
		f, err := os.Open(p)
		if err != nil {
			return nil, err
		}
		run := parseMutateLog(f)
		f.Close()
		if run.Source != "" {
			runs = append(runs, run)
		}
	}
	return runs, nil
}

func crapEDN(entries []crapEntry) string {
	var b strings.Builder
	b.WriteString("{:entries [")
	for i, e := range entries {
		if i > 0 {
			b.WriteString("\n  ")
		}
		fmt.Fprintf(&b, "{:name %s :namespace %s :complexity %d :coverage %s :crap %s}",
			strconv.Quote(e.Name), strconv.Quote(e.Namespace), e.Complexity, num(e.Coverage), num(e.Crap))
	}
	b.WriteString("]}\n")
	return b.String()
}

func mutateEDN(ns string, forms []mutateForm) string {
	var b strings.Builder
	fmt.Fprintf(&b, "{:version 1 :namespace %s :forms [", strconv.Quote(ns))
	for i, f := range forms {
		if i > 0 {
			b.WriteString("\n  ")
		}
		fmt.Fprintf(&b, "{:id %s :killed %d :survived %d :uncovered %d :sites %d}",
			strconv.Quote(defnID(f.Name)), f.Killed, f.Survived, f.Uncovered, f.Sites)
	}
	b.WriteString("]}\n")
	return b.String()
}

// defnID marks a lowercase (unexported) function private, as the viewer reads
// `defn-/` for Clojure's private functions.
func defnID(name string) string {
	last := name[strings.LastIndex(name, ".")+1:]
	if last != "" && last[0] >= 'a' && last[0] <= 'z' {
		return "defn-/" + name
	}
	return "defn/" + name
}

func num(f float64) string {
	return strconv.FormatFloat(f, 'f', -1, 64)
}
