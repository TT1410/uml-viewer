package main

import (
	"bufio"
	"io"
	"regexp"
	"strings"
)

// mutateRun is what one mutate4go run says about one source file.
type mutateRun struct {
	Source string                     // as passed to mutate4go, module-relative
	Counts map[string]*mutationCounts // by function name
}

type mutationCounts struct {
	Killed, Survived, Uncovered int
}

var (
	runLine       = regexp.MustCompile(`^Mutation run: (\S+)`)
	resultLine    = regexp.MustCompile(`^\[\d+/\d+\] (?:worker-\d+ )?(killed|survived|timeout) line \d+ .*: func/(\S+)\s*$`)
	uncoveredLine = regexp.MustCompile(`^\s+line \d+ .* func/(\S+)\s*$`)
)

// parseMutateLog reads mutate4go output. Results come from the per-mutant
// progress lines; uncovered sites from the "Uncovered mutations:" list.
// The "Survivors:" list repeats progress lines and is ignored.
func parseMutateLog(r io.Reader) mutateRun {
	run := mutateRun{Counts: map[string]*mutationCounts{}}
	section := ""
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 1024*1024), 1024*1024)
	for sc.Scan() {
		line := sc.Text()
		section = nextSection(section, line)
		run.take(section, line)
	}
	return run
}

func nextSection(current, line string) string {
	switch {
	case strings.HasPrefix(line, "Uncovered mutations:"):
		return "uncovered"
	case strings.HasPrefix(line, "Survivors:"):
		return "survivors"
	case line != "" && !strings.HasPrefix(line, " "):
		return ""
	}
	return current
}

func (run *mutateRun) take(section, line string) {
	if m := runLine.FindStringSubmatch(line); m != nil {
		run.Source = strings.TrimPrefix(m[1], "./")
		return
	}
	if m := resultLine.FindStringSubmatch(line); m != nil {
		c := run.counts(m[2])
		if m[1] == "survived" {
			c.Survived++
		} else {
			c.Killed++
		}
		return
	}
	if section == "uncovered" {
		if m := uncoveredLine.FindStringSubmatch(line); m != nil {
			run.counts(m[1]).Uncovered++
		}
	}
}

func (run *mutateRun) counts(name string) *mutationCounts {
	c := run.Counts[name]
	if c == nil {
		c = &mutationCounts{}
		run.Counts[name] = c
	}
	return c
}

// mutateForm is one function of a snapshot; sites 0 means no mutation operators.
type mutateForm struct {
	Name                               string
	Killed, Survived, Uncovered, Sites int
}

// mutateSnapshots groups runs by package namespace. Every function of a file
// that has a run is listed, with zeros when mutate4go found no site in it.
func mutateSnapshots(runs []mutateRun, fns []function) map[string][]mutateForm {
	byFile := map[string][]function{}
	for _, fn := range fns {
		byFile[fn.File] = append(byFile[fn.File], fn)
	}
	out := map[string][]mutateForm{}
	for _, run := range runs {
		for _, fn := range byFile[sourceFile(run.Source, byFile)] {
			out[fn.Namespace] = append(out[fn.Namespace], formOf(fn.Name, run.Counts[fn.Name]))
		}
	}
	return out
}

// sourceFile matches the run's path to a scanned file by suffix, so a run made
// from the module directory or from a parent both match.
func sourceFile(source string, byFile map[string][]function) string {
	for file := range byFile {
		if source == file || strings.HasSuffix(source, "/"+file) || strings.HasSuffix(file, "/"+source) {
			return file
		}
	}
	return ""
}

func formOf(name string, c *mutationCounts) mutateForm {
	if c == nil {
		return mutateForm{Name: name}
	}
	return mutateForm{
		Name: name, Killed: c.Killed, Survived: c.Survived, Uncovered: c.Uncovered,
		Sites: c.Killed + c.Survived + c.Uncovered,
	}
}
