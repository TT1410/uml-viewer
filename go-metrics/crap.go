package main

import (
	"bufio"
	"io"
	"strconv"
	"strings"
)

// segment is one block of a Go cover profile.
type segment struct {
	StartLine, EndLine int
	Statements, Count  int
}

// parseProfile reads a cover profile into segments keyed by the file's
// import-path-qualified name (`module/dir/file.go`).
func parseProfile(r io.Reader) map[string][]segment {
	out := map[string][]segment{}
	sc := bufio.NewScanner(r)
	for sc.Scan() {
		file, seg, ok := parseProfileLine(strings.TrimSpace(sc.Text()))
		if ok {
			out[file] = append(out[file], seg)
		}
	}
	return out
}

// parseProfileLine parses `file.go:12.3,15.2 3 1`.
func parseProfileLine(line string) (string, segment, bool) {
	colon := strings.LastIndex(line, ":")
	if colon < 0 || strings.HasPrefix(line, "mode:") {
		return "", segment{}, false
	}
	fields := strings.Fields(line[colon+1:])
	if len(fields) != 3 {
		return "", segment{}, false
	}
	start, end, ok := strings.Cut(fields[0], ",")
	if !ok {
		return "", segment{}, false
	}
	return line[:colon], segment{
		StartLine:  lineOf(start),
		EndLine:    lineOf(end),
		Statements: atoi(fields[1]),
		Count:      atoi(fields[2]),
	}, true
}

func lineOf(pos string) int {
	l, _, _ := strings.Cut(pos, ".")
	return atoi(l)
}

func atoi(s string) int {
	n, _ := strconv.Atoi(s)
	return n
}

// coverage is the percentage of statements covered between start and end
// lines; nil when the profile has nothing for the file (as crap4go).
func coverage(segs []segment, start, end int) *float64 {
	if len(segs) == 0 {
		return nil
	}
	total, covered := 0, 0
	for _, s := range segs {
		if s.EndLine < start || s.StartLine > end {
			continue
		}
		total += s.Statements
		if s.Count > 0 {
			covered += s.Statements
		}
	}
	pct := 0.0
	if total > 0 {
		pct = 100 * float64(covered) / float64(total)
	}
	return &pct
}

// crapScore is cc²·(1−cov)³ + cc with cov in percent.
func crapScore(cc int, pct float64) float64 {
	c := float64(cc)
	u := 1 - pct/100
	return c*c*u*u*u + c
}

type crapEntry struct {
	Name, Namespace string
	Complexity      int
	Coverage, Crap  float64
}

func crapEntries(fns []function, profile map[string][]segment, modPath string) []crapEntry {
	var out []crapEntry
	for _, fn := range fns {
		pct := coverage(profile[modPath+"/"+fn.File], fn.StartLine, fn.EndLine)
		if pct == nil {
			continue
		}
		out = append(out, crapEntry{
			Name: fn.Name, Namespace: fn.Namespace, Complexity: fn.Complexity,
			Coverage: *pct, Crap: crapScore(fn.Complexity, *pct),
		})
	}
	return out
}
