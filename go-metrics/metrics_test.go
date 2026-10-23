package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const orderGo = `package order

type Service struct{}

func New() *Service { return &Service{} }

func (s *Service) Place(n int) int {
	if n > 0 && n < 10 {
		return n
	}
	return 0
}

func helper() {}
`

func writeModule(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	must(t, os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module example.com/shop\n\ngo 1.23\n"), 0o644))
	must(t, os.MkdirAll(filepath.Join(dir, "internal", "order"), 0o755))
	must(t, os.WriteFile(filepath.Join(dir, "internal", "order", "order.go"), []byte(orderGo), 0o644))
	must(t, os.WriteFile(filepath.Join(dir, "internal", "order", "order_test.go"), []byte("package order\nfunc TestX() {}\n"), 0o644))
	return dir
}

func must(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}

func TestFunctionsAreNamedAndCountedAsCrap4go(t *testing.T) {
	dir := writeModule(t)
	fns, err := moduleFunctions(dir, "example.com/shop", dir)
	must(t, err)
	got := map[string]function{}
	for _, f := range fns {
		got[f.Name] = f
	}
	if len(fns) != 3 {
		t.Fatalf("want 3 functions (tests skipped), got %d: %+v", len(fns), fns)
	}
	place := got["Service.Place"]
	if place.Complexity != 3 || place.Namespace != "example.com.shop.internal.order" || place.File != "internal/order/order.go" {
		t.Fatalf("Service.Place: %+v", place)
	}
	if got["New"].Complexity != 1 {
		t.Fatalf("New: %+v", got["New"])
	}
}

func TestCoverageAndCrapFollowTheProfile(t *testing.T) {
	profile := parseProfile(strings.NewReader(
		"mode: set\n" +
			"example.com/shop/internal/order/order.go:7.36,8.22 1 1\n" +
			"example.com/shop/internal/order/order.go:8.22,10.3 1 0\n" +
			"example.com/shop/internal/order/order.go:11.2,11.10 2 1\n"))
	segs := profile["example.com/shop/internal/order/order.go"]
	pct := coverage(segs, 7, 12)
	if pct == nil || *pct != 75 {
		t.Fatalf("coverage: %v", pct)
	}
	if got := crapScore(3, 75); got != 3*3*0.25*0.25*0.25+3 {
		t.Fatalf("crap: %v", got)
	}
	if coverage(nil, 1, 2) != nil {
		t.Fatal("a file absent from the profile has no coverage")
	}
	if z := coverage(segs, 13, 13); z == nil || *z != 0 {
		t.Fatalf("a function without statements counts as 0%% (as crap4go): %v", z)
	}
}

func TestMutateLogCountsResultsAndUncoveredButNotTheSurvivorRecap(t *testing.T) {
	run := parseMutateLog(strings.NewReader(`Mutation run: internal/order/order.go
Total mutation sites: 5
Uncovered mutations:
  line 14 remove call func/helper
[1/3] worker-1 killed line 8 replace > with >=: func/Service.Place
[2/3] worker-2 survived line 8 replace && with ||: func/Service.Place
[3/3] timeout line 5 return nil: func/New

Mutation Report
===============
Killed: 2
Survived: 1
Uncovered: 1

Survivors:
  line 8 replace && with || func/Service.Place
`))
	if run.Source != "internal/order/order.go" {
		t.Fatalf("source: %q", run.Source)
	}
	place := run.Counts["Service.Place"]
	if place.Killed != 1 || place.Survived != 1 || place.Uncovered != 0 {
		t.Fatalf("Service.Place: %+v", place)
	}
	if run.Counts["New"].Killed != 1 || run.Counts["helper"].Uncovered != 1 {
		t.Fatalf("counts: %+v %+v", run.Counts["New"], run.Counts["helper"])
	}
}

func TestRunWritesSnapshotsKeyedByImportPath(t *testing.T) {
	dir := writeModule(t)
	profile := filepath.Join(dir, "cover.out")
	must(t, os.WriteFile(profile, []byte("mode: set\nexample.com/shop/internal/order/order.go:5.27,5.46 1 1\n"), 0o644))
	logs := filepath.Join(dir, "logs")
	must(t, os.MkdirAll(logs, 0o755))
	must(t, os.WriteFile(filepath.Join(logs, "order.log"), []byte(
		"Mutation run: internal/order/order.go\n[1/1] killed line 5 x: func/New\n"), 0o644))
	out := filepath.Join(dir, ".metrics")
	must(t, run(options{module: dir, profile: profile, mutateLogs: logs, out: out}))

	crap, err := os.ReadFile(filepath.Join(out, "crap.edn"))
	must(t, err)
	if !strings.Contains(string(crap), `{:name "New" :namespace "example.com.shop.internal.order" :complexity 1 :coverage 100 :crap 1}`) {
		t.Fatalf("crap.edn:\n%s", crap)
	}
	snap, err := os.ReadFile(filepath.Join(out, "mutate", "example.com.shop.internal.order.edn"))
	must(t, err)
	for _, want := range []string{
		`:namespace "example.com.shop.internal.order"`,
		`{:id "defn/New" :killed 1 :survived 0 :uncovered 0 :sites 1}`,
		`{:id "defn/Service.Place" :killed 0 :survived 0 :uncovered 0 :sites 0}`,
		`{:id "defn-/helper"`,
	} {
		if !strings.Contains(string(snap), want) {
			t.Fatalf("snapshot lacks %s:\n%s", want, snap)
		}
	}
}

func TestReceiversAreNamedByTheirType(t *testing.T) {
	dir := t.TempDir()
	must(t, os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module m\n"), 0o644))
	must(t, os.WriteFile(filepath.Join(dir, "g.go"), []byte(`package g
type List[T any] struct{}
type Pair[K, V any] struct{}
type Plain struct{}
func (l *List[T]) Push() {}
func (p Pair[K, V]) Key() {}
func (Plain) Value() {}
`), 0o644))
	fns, err := moduleFunctions(dir, "m", dir)
	must(t, err)
	var names []string
	for _, f := range fns {
		names = append(names, f.Name)
	}
	if strings.Join(names, " ") != "List.Push Pair.Key Plain.Value" {
		t.Fatalf("names: %v", names)
	}
	if fns[0].Namespace != "m" {
		t.Fatalf("root package namespace: %q", fns[0].Namespace)
	}
}
