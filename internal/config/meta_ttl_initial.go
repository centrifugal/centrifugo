package config

import (
	"fmt"
	"time"

	"github.com/centrifugal/centrifugo/v6/internal/configtypes"
)

// defaultChannelPositionCheckDelay is the delay Centrifuge uses when
// client.channel_position_check_delay is 0.
const defaultChannelPositionCheckDelay = 40 * time.Second

// channelPositionCheckDelay is the position check delay Centrifuge uses.
func channelPositionCheckDelay(cfg Config) time.Duration {
	if d := cfg.Client.ChannelPositionCheckDelay.ToDuration(); d != 0 {
		return d
	}
	return defaultChannelPositionCheckDelay
}

// minInitialMetaTTL is the shortest initial meta TTL: live subscribers keep such
// a meta with position checks, so it must be at least 3 of their delays. Whole
// seconds, as meta TTLs are.
func minInitialMetaTTL(cfg Config) time.Duration {
	return (3*channelPositionCheckDelay(cfg) + time.Second - 1).Truncate(time.Second)
}

// raiseInitialMetaTTLs raises the initial meta TTLs below minInitialMetaTTL to
// it, with a warning for each: so that the default (5m) works with any
// client.channel_position_check_delay, and a value set too short doesn't stop
// Centrifugo from starting. Zero (the global one) and negative (off) are kept.
func raiseInitialMetaTTLs(cfg Config) (Config, []string) {
	minTTL := minInitialMetaTTL(cfg)
	var warnings []string
	raise := func(name string, d *configtypes.Duration) {
		if ttl := d.ToDuration(); ttl > 0 && ttl < minTTL {
			warnings = append(warnings, fmt.Sprintf("%s (%s) is below 3 × client.channel_position_check_delay (%s), using %s", name, ttl, channelPositionCheckDelay(cfg), minTTL))
			*d = configtypes.Duration(minTTL)
		}
	}
	raise("channel.history_meta_ttl_initial", &cfg.Channel.HistoryMetaTTLInitial)
	raise("channel.without_namespace.history_meta_ttl_initial", &cfg.Channel.WithoutNamespace.HistoryMetaTTLInitial)
	raise("channel.without_namespace.map.meta_ttl_initial", &cfg.Channel.WithoutNamespace.Map.MetaTTLInitial)
	namespaces := make([]configtypes.ChannelNamespace, len(cfg.Channel.Namespaces))
	copy(namespaces, cfg.Channel.Namespaces)
	for i := range namespaces {
		ns := &namespaces[i]
		raise(fmt.Sprintf("channel.namespaces[%s].history_meta_ttl_initial", ns.Name), &ns.HistoryMetaTTLInitial)
		raise(fmt.Sprintf("channel.namespaces[%s].map.meta_ttl_initial", ns.Name), &ns.Map.MetaTTLInitial)
	}
	if cfg.Channel.Namespaces != nil {
		cfg.Channel.Namespaces = namespaces
	}
	return cfg, warnings
}
