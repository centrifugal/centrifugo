package app

import (
	"os"
	"strconv"
	"strings"

	"github.com/centrifugal/centrifugo/v6/internal/config"
	"github.com/centrifugal/centrifugo/v6/internal/logging"

	"github.com/centrifugal/centrifuge"
)

func centrifugeNodeConfig(version string, edition string, cfgContainer *config.Container, logHandler centrifuge.LogHandler) centrifuge.Config {
	appCfg := cfgContainer.Config()
	cfg := centrifuge.Config{}
	cfg.Version = version + " " + strings.ToUpper(edition)
	cfg.Metrics = centrifuge.MetricsConfig{
		MetricsNamespace: "centrifugo",
	}
	cfg.Name = nodeName(appCfg)
	cfg.ChannelMaxLength = appCfg.Channel.MaxLength
	cfg.ClientPresenceUpdateInterval = appCfg.Client.PresenceUpdateInterval.ToDuration()
	cfg.ClientExpiredCloseDelay = appCfg.Client.ExpiredCloseDelay.ToDuration()
	cfg.ClientExpiredSubCloseDelay = appCfg.Client.ExpiredSubCloseDelay.ToDuration()
	cfg.ClientStaleCloseDelay = appCfg.Client.StaleCloseDelay.ToDuration()
	cfg.ClientQueueMaxSize = appCfg.Client.QueueMaxSize
	cfg.ClientChannelLimit = appCfg.Client.ChannelLimit
	cfg.ClientChannelPositionCheckDelay = appCfg.Client.ChannelPositionCheckDelay.ToDuration()
	cfg.ClientChannelPositionMaxTimeLag = appCfg.Client.ChannelPositionMaxTimeLag.ToDuration()
	cfg.UserConnectionLimit = appCfg.Client.UserConnectionLimit
	cfg.NodeInfoMetricsAggregateInterval = appCfg.Node.InfoMetricsAggregateInterval.ToDuration()
	cfg.HistoryMaxPublicationLimit = appCfg.Client.HistoryMaxPublicationLimit
	cfg.RecoveryMaxPublicationLimit = appCfg.Client.RecoveryMaxPublicationLimit
	cfg.HistoryMetaTTL = appCfg.Channel.HistoryMetaTTL.ToDuration()
	cfg.ClientConnectIncludeServerTime = appCfg.Client.ConnectIncludeServerTime
	cfg.LogLevel = logging.CentrifugeLogLevel(strings.ToLower(appCfg.Log.Level))
	cfg.LogHandler = logHandler
	if appCfg.Client.ConnectCodeToUnidirectionalDisconnect.Enabled {
		uniCodeTransforms := make(map[uint32]centrifuge.Disconnect)
		for _, transform := range appCfg.Client.ConnectCodeToUnidirectionalDisconnect.Transforms {
			uniCodeTransforms[transform.Code] = centrifuge.Disconnect{Code: transform.To.Code, Reason: transform.To.Reason}
		}
		cfg.UnidirectionalCodeToDisconnect = uniCodeTransforms
	}
	return cfg
}

// nodeName returns a name for this Centrifugo node. If no name provided
// in configuration then it constructs node name based on hostname and port
func nodeName(cfg config.Config) string {
	name := cfg.Node.Name
	if name != "" {
		return name
	}
	port := strconv.Itoa(cfg.HTTP.Port)
	var hostname string
	hostname, err := os.Hostname()
	if err != nil {
		hostname = "?"
	}
	return hostname + "_" + port
}
