package graphite

import (
	"context"
	"fmt"
	"io"
	"net"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/FZambia/eagle"
	"github.com/prometheus/client_golang/prometheus"
)

var re = regexp.MustCompile("[[:^ascii:]]")

// PreparePathComponent cleans string to be used as Graphite metric path.
func PreparePathComponent(s string) string {
	s = re.ReplaceAllLiteralString(s, "_")
	return strings.Replace(s, ".", "_", -1)
}

// Exporter to Graphite.
type Exporter struct {
	prefix    string
	address   string
	timeout   time.Duration
	tags      bool
	closeOnce sync.Once
	closeCh   chan struct{}
	sink      chan eagle.Metrics
	eagle     *eagle.Eagle
}

// Config for Graphite Exporter.
type Config struct {
	Address  string
	Gatherer prometheus.Gatherer
	Interval time.Duration
	Prefix   string
	Tags     bool
}

// New creates new Graphite Exporter.
func New(c Config) *Exporter {
	exporter := &Exporter{
		prefix:  c.Prefix,
		address: c.Address,
		timeout: time.Second,
		tags:    c.Tags,
		closeCh: make(chan struct{}),
		sink:    make(chan eagle.Metrics),
	}
	exporter.eagle = eagle.New(eagle.Config{
		Gatherer: c.Gatherer,
		Interval: c.Interval,
		Sink:     exporter.sink,
	})
	return exporter
}

func (e *Exporter) Run(ctx context.Context) error {
	defer func() { _ = e.close() }()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case metrics := <-e.sink:
			_ = e.exportOnce(metrics)
		}
	}
}

// Close stops exporter.
func (e *Exporter) close() error {
	e.closeOnce.Do(func() {
		close(e.closeCh)
		_ = e.eagle.Close()
	})
	return nil
}

func (e *Exporter) exportOnce(metrics eagle.Metrics) error {
	conn, err := net.DialTimeout("tcp", e.address, e.timeout)
	if err != nil {
		return err
	}
	defer func() { _ = conn.Close() }()
	_ = e.write(conn, metrics)
	return nil
}

func makeTags(labels []string) string {
	if len(labels) == 0 {
		return ""
	}
	var tagParts []string
	for i := 0; i < len(labels); i += 2 {
		tagParts = append(tagParts, fmt.Sprintf("%s=%s", labels[i], labels[i+1]))
	}
	return ";" + strings.Join(tagParts, ";")
}

func (e *Exporter) write(w io.Writer, metrics eagle.Metrics) error {
	now := time.Now().Unix()
	for _, item := range metrics.Items {
		for _, metricValue := range item.Values {
			parts := []string{e.prefix}
			if item.Namespace != "" {
				parts = append(parts, item.Namespace)
			}
			if item.Subsystem != "" {
				parts = append(parts, item.Subsystem)
			}
			if item.Name != "" {
				parts = append(parts, item.Name)
			}
			if metricValue.Name != "" {
				parts = append(parts, metricValue.Name)
			}
			if !e.tags {
				parts = append(parts, metricValue.Labels...)
			}
			key := strings.Join(parts, ".")

			if e.tags {
				key += makeTags(metricValue.Labels)
			}

			var err error

			if item.Type == eagle.MetricTypeCounter {
				_, err = fmt.Fprintf(w, "%s %d %d\n", key, int64(metricValue.Value), now)
			} else {
				_, err = fmt.Fprintf(w, "%s %f %d\n", key, metricValue.Value, now)
			}
			if err != nil {
				return err
			}
		}
	}
	return nil
}
