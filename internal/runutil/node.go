package runutil

import (
	"os"
	"strconv"
	"strings"

	"github.com/centrifugal/centrifugo/v5/internal/config"
	"github.com/centrifugal/centrifugo/v5/internal/logging"

	"github.com/centrifugal/centrifuge"
)

func centrifugeNodeConfig(version string, cfgContainer *config.Container) centrifuge.Config {
	appCfg := cfgContainer.Config()
	cfg := centrifuge.Config{}
	cfg.Version = version
	cfg.MetricsNamespace = "centrifugo"
	cfg.Name = nodeName(appCfg)
	cfg.ChannelMaxLength = appCfg.Channel.MaxLength
	cfg.ClientPresenceUpdateInterval = appCfg.Client.PresenceUpdateInterval
	cfg.ClientExpiredCloseDelay = appCfg.Client.ExpiredCloseDelay
	cfg.ClientExpiredSubCloseDelay = appCfg.Client.ExpiredSubCloseDelay
	cfg.ClientStaleCloseDelay = appCfg.Client.StaleCloseDelay
	cfg.ClientQueueMaxSize = appCfg.Client.QueueMaxSize
	cfg.ClientChannelLimit = appCfg.Client.ChannelLimit
	cfg.ClientChannelPositionCheckDelay = appCfg.Client.ChannelPositionCheckDelay
	cfg.ClientChannelPositionMaxTimeLag = appCfg.Client.ChannelPositionMaxTimeLag
	cfg.UserConnectionLimit = appCfg.Client.UserConnectionLimit
	cfg.NodeInfoMetricsAggregateInterval = appCfg.Node.InfoMetricsAggregateInterval
	cfg.HistoryMaxPublicationLimit = appCfg.Client.HistoryMaxPublicationLimit
	cfg.RecoveryMaxPublicationLimit = appCfg.Client.RecoveryMaxPublicationLimit
	cfg.HistoryMetaTTL = appCfg.Channel.HistoryMetaTTL // TODO: v6. GetDuration("global_history_meta_ttl", true)
	cfg.ClientConnectIncludeServerTime = appCfg.Client.ConnectIncludeServerTime
	cfg.LogLevel = logging.CentrifugeLogLevel(strings.ToLower(appCfg.LogLevel))
	cfg.LogHandler = logging.NewCentrifugeLogHandler().Handle
	return cfg
}

// nodeName returns a name for this Centrifugo node. If no name provided
// in configuration then it constructs node name based on hostname and port
func nodeName(cfg config.Config) string {
	name := cfg.Node.Name
	if name != "" {
		return name
	}
	port := strconv.Itoa(cfg.Port)
	var hostname string
	hostname, err := os.Hostname()
	if err != nil {
		hostname = "?"
	}
	return hostname + "_" + port
}
