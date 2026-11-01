// Command snapshot runs a Centrifugo binary through scripted scenarios and
// writes what clients, backends and operators can observe as normalized
// text files, so that two builds can be compared with a plain diff.
//
//	snapshot -binary <path> -config <file> [-config <file>...] -scenarios <dir> [-scenarios <dir>...] -out <dir>
//	snapshot diff <dirA> <dirB>
//
// See README.md.
package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strings"
	"sync"
	"time"
)

type multiFlag []string

func (m *multiFlag) String() string     { return strings.Join(*m, ",") }
func (m *multiFlag) Set(v string) error { *m = append(*m, v); return nil }

func main() {
	if len(os.Args) > 1 && os.Args[1] == "diff" {
		os.Exit(diffMain(os.Args[2:]))
	}
	os.Exit(runMain(os.Args[1:]))
}

func diffMain(args []string) int {
	if len(args) != 2 {
		fmt.Fprintln(os.Stderr, "usage: snapshot diff <dirA> <dirB>")
		return 2
	}
	d, err := DiffDirs(args[0], args[1])
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		return 2
	}
	if d == "" {
		fmt.Println("snapshots are identical")
		return 0
	}
	fmt.Print(d)
	return 1
}

func runMain(args []string) int {
	fs := flag.NewFlagSet("snapshot", flag.ExitOnError)
	var configs, scenarioDirs multiFlag
	binary := fs.String("binary", "", "path to the Centrifugo binary")
	fs.Var(&configs, "config", "base config template (JSON); repeat to merge more (later files are JSON merge patches)")
	fs.Var(&scenarioDirs, "scenarios", "scenario directory; repeat for more (a later file with the same name replaces an earlier one)")
	out := fs.String("out", "", "output directory (must not exist or be empty)")
	with := fs.String("with", "", "comma-separated integration services available: redis,postgres,kafka,nats")
	only := fs.String("only", "", "regexp: run only scenarios whose name matches")
	timeout := fs.Duration("timeout", 10*time.Second, "timeout of each waiting step")
	parallel := fs.Int("parallel", max(runtime.NumCPU()/2, 1), "scenarios run concurrently")
	redisAddr := fs.String("redis", "redis://127.0.0.1:6379", "Redis address for {{.RedisAddr}}")
	pgDSN := fs.String("postgres", "postgres://test:test@127.0.0.1:5432/test", "Postgres DSN for {{.PostgresDSN}}")
	kafkaAddr := fs.String("kafka", "127.0.0.1:29092", "Kafka broker for {{.KafkaAddr}}")
	natsURL := fs.String("nats", "nats://127.0.0.1:4222", "NATS URL for {{.NatsURL}}")
	_ = fs.Parse(args)

	if *binary == "" || len(configs) == 0 || len(scenarioDirs) == 0 || *out == "" {
		fs.Usage()
		return 2
	}
	bin, err := filepath.Abs(*binary)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 2
	}
	opts := Options{
		Binary:      bin,
		Out:         *out,
		With:        map[string]bool{},
		Timeout:     *timeout,
		RedisAddr:   *redisAddr,
		PostgresDSN: *pgDSN,
		KafkaAddr:   *kafkaAddr,
		NatsURL:     *natsURL,
	}
	for _, c := range configs {
		a, err := filepath.Abs(c)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 2
		}
		opts.Configs = append(opts.Configs, a)
	}
	for _, w := range strings.Split(*with, ",") {
		if w = strings.TrimSpace(w); w != "" {
			opts.With[w] = true
		}
	}
	if *only != "" {
		opts.Only = regexp.MustCompile(*only)
	}
	if entries, err := os.ReadDir(*out); err == nil && len(entries) > 0 {
		fmt.Fprintf(os.Stderr, "output directory %s is not empty\n", *out)
		return 2
	}
	if err := os.MkdirAll(*out, 0o755); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 2
	}
	scenarios, raw, err := loadScenarios(scenarioDirs)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 2
	}

	type job struct {
		name, variant string
		v             Variant
	}
	var jobs []job
	for _, s := range scenarios {
		if opts.Only != nil && !opts.Only.MatchString(s.Name) {
			continue
		}
		if len(s.Variants) == 0 {
			jobs = append(jobs, job{name: s.Name})
			continue
		}
		names := make([]string, 0, len(s.Variants))
		for n := range s.Variants {
			names = append(names, n)
		}
		sort.Strings(names)
		for _, n := range names {
			jobs = append(jobs, job{name: s.Name, variant: n, v: s.Variants[n]})
		}
	}

	start := time.Now()
	var (
		wg       sync.WaitGroup
		mu       sync.Mutex
		failed   bool
		sem      = make(chan struct{}, max(*parallel, 1))
		ran      int
		skipped  []string
		requires = func(s *Scenario, v Variant) []string {
			var missing []string
			for _, r := range append(append([]string{}, s.Requires...), v.Requires...) {
				if !opts.With[r] {
					missing = append(missing, r)
				}
			}
			return missing
		}
	)
	byName := map[string]*Scenario{}
	for _, s := range scenarios {
		byName[s.Name] = s
	}
	for _, j := range jobs {
		id := j.name
		if j.variant != "" {
			id += "@" + j.variant
		}
		if missing := requires(byName[j.name], j.v); len(missing) > 0 {
			skipped = append(skipped, fmt.Sprintf("%s (needs %s)", id, strings.Join(missing, ",")))
			continue
		}
		ran++
		wg.Add(1)
		sem <- struct{}{}
		go func(j job, id string) {
			defer wg.Done()
			defer func() { <-sem }()
			t := time.Now()
			err := runScenario(opts, raw[j.name], j.name, j.variant, j.v)
			mu.Lock()
			defer mu.Unlock()
			if err != nil {
				failed = true
				fmt.Fprintf(os.Stderr, "FAIL %s: %v\n", id, err)
				return
			}
			fmt.Fprintf(os.Stderr, "ok   %s (%s)\n", id, time.Since(t).Round(time.Millisecond))
		}(j, id)
	}
	wg.Wait()
	for _, s := range skipped {
		fmt.Fprintf(os.Stderr, "skip %s\n", s)
	}
	fmt.Fprintf(os.Stderr, "%d scenario runs in %s, %d skipped\n", ran, time.Since(start).Round(time.Millisecond), len(skipped))
	if failed {
		return 1
	}
	return 0
}
