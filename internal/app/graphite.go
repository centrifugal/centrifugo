package app

import (
	"net"
	"strconv"
	"strings"

	"github.com/centrifugal/centrifugo/v6/internal/config"
	"github.com/centrifugal/centrifugo/v6/internal/metrics/graphite"

	"github.com/centrifugal/centrifuge"
	"github.com/prometheus/client_golang/prometheus"
)

func graphiteExporter(cfg config.Config, nodeCfg centrifuge.Config) *graphite.Exporter {
	return graphite.New(graphite.Config{
		Address:  net.JoinHostPort(cfg.Graphite.Host, strconv.Itoa(cfg.Graphite.Port)),
		Gatherer: prometheus.DefaultGatherer,
		Prefix:   strings.TrimSuffix(cfg.Graphite.Prefix, ".") + "." + graphite.PreparePathComponent(nodeCfg.Name),
		Interval: cfg.Graphite.Interval.ToDuration(),
		Tags:     cfg.Graphite.Tags,
	})
}
