package main

import (
	"bytes"
	"errors"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
	"text/template"
	"time"

	"github.com/centrifugal/centrifugo/Godeps/_workspace/src/github.com/FZambia/go-logger"
	"github.com/centrifugal/centrifugo/Godeps/_workspace/src/github.com/satori/go.uuid"
	"github.com/centrifugal/centrifugo/Godeps/_workspace/src/github.com/spf13/viper"
	"github.com/centrifugal/centrifugo/libcentrifugo"
)

// newConfig creates new libcentrifugo.Config using viper.
func newConfig() *libcentrifugo.Config {
	cfg := &libcentrifugo.Config{}
	cfg.Version = VERSION
	cfg.Name = getApplicationName()
	cfg.Debug = viper.GetBool("debug")
	cfg.Admin = viper.GetBool("admin")
	cfg.Web = viper.GetBool("web")
	cfg.AdminPassword = viper.GetString("admin_password")
	cfg.AdminSecret = viper.GetString("admin_secret")
	cfg.ChannelPrefix = viper.GetString("channel_prefix")
	cfg.AdminChannel = libcentrifugo.ChannelID(cfg.ChannelPrefix + "." + "admin")
	cfg.ControlChannel = libcentrifugo.ChannelID(cfg.ChannelPrefix + "." + "control")
	cfg.MaxChannelLength = viper.GetInt("max_channel_length")
	cfg.PingInterval = time.Duration(viper.GetInt("ping_interval")) * time.Second
	cfg.NodePingInterval = time.Duration(viper.GetInt("node_ping_interval")) * time.Second
	cfg.NodeInfoCleanInterval = cfg.NodePingInterval * 3
	cfg.NodeInfoMaxDelay = cfg.NodePingInterval*2 + 1*time.Second
	cfg.NodeMetricsInterval = time.Duration(viper.GetInt("node_metrics_interval")) * time.Second
	cfg.PresencePingInterval = time.Duration(viper.GetInt("presence_ping_interval")) * time.Second
	cfg.PresenceExpireInterval = time.Duration(viper.GetInt("presence_expire_interval")) * time.Second
	cfg.MessageSendTimeout = time.Duration(viper.GetInt("message_send_timeout")) * time.Second
	cfg.PrivateChannelPrefix = viper.GetString("private_channel_prefix")
	cfg.NamespaceChannelBoundary = viper.GetString("namespace_channel_boundary")
	cfg.UserChannelBoundary = viper.GetString("user_channel_boundary")
	cfg.UserChannelSeparator = viper.GetString("user_channel_separator")
	cfg.ClientChannelBoundary = viper.GetString("client_channel_boundary")
	cfg.ExpiredConnectionCloseDelay = time.Duration(viper.GetInt("expired_connection_close_delay")) * time.Second
	cfg.StaleConnectionCloseDelay = time.Duration(viper.GetInt("stale_connection_close_delay")) * time.Second
	cfg.ClientRequestMaxSize = viper.GetInt("client_request_max_size")
	cfg.ClientQueueMaxSize = viper.GetInt("client_queue_max_size")
	cfg.ClientQueueInitialCapacity = viper.GetInt("client_queue_initial_capacity")
	cfg.ClientChannelLimit = viper.GetInt("client_channel_limit")
	cfg.Insecure = viper.GetBool("insecure")
	cfg.InsecureAPI = viper.GetBool("insecure_api")
	cfg.InsecureAdmin = viper.GetBool("insecure_admin") || viper.GetBool("insecure_web")

	cfg.Secret = viper.GetString("secret")
	cfg.ConnLifetime = int64(viper.GetInt("connection_lifetime"))

	cfg.Watch = viper.GetBool("watch")
	cfg.Publish = viper.GetBool("publish")
	cfg.Anonymous = viper.GetBool("anonymous")
	cfg.Presence = viper.GetBool("presence")
	cfg.JoinLeave = viper.GetBool("join_leave")
	cfg.HistorySize = viper.GetInt("history_size")
	cfg.HistoryLifetime = viper.GetInt("history_lifetime")
	cfg.HistoryDropInactive = viper.GetBool("history_drop_inactive")
	cfg.Recover = viper.GetBool("recover")
	cfg.Namespaces = namespacesFromConfig(nil)

	return cfg
}

// getApplicationName returns a name for this node. If no name provided
// in configuration then it constructs node name based on hostname and port
func getApplicationName() string {
	name := viper.GetString("name")
	if name != "" {
		return name
	}
	port := viper.GetString("port")
	var hostname string
	hostname, err := os.Hostname()
	if err != nil {
		logger.ERROR.Println(err)
		hostname = "?"
	}
	return hostname + "_" + port
}

// pathExists returns whether the given file or directory exists or not
func pathExists(path string) (bool, error) {
	_, err := os.Stat(path)
	if err == nil {
		return true, nil
	}
	if os.IsNotExist(err) {
		return false, nil
	}
	return false, err
}

var jsonConfigTemplate = `{
  "secret": "{{.Secret}}"
}
`

var tomlConfigTemplate = `secret = {{.Secret}}
`

var yamlConfigTemplate = `secret: {{.Secret}}
`

// generateConfig generates configuration file at provided path.
func generateConfig(f string) error {
	exists, err := pathExists(f)
	if err != nil {
		return err
	}
	if exists {
		return errors.New("output config file already exists: " + f)
	}
	ext := filepath.Ext(f)

	if len(ext) > 1 {
		ext = ext[1:]
	}

	supportedExts := []string{"json", "toml", "yaml", "yml"}

	if !stringInSlice(ext, supportedExts) {
		return errors.New("output config file must have one of supported extensions: " + strings.Join(supportedExts, ", "))
	}

	var t *template.Template

	switch ext {
	case "json":
		t, err = template.New("config").Parse(jsonConfigTemplate)
	case "toml":
		t, err = template.New("config").Parse(tomlConfigTemplate)
	case "yaml", "yml":
		t, err = template.New("config").Parse(yamlConfigTemplate)
	}
	if err != nil {
		return err
	}

	var output bytes.Buffer
	t.Execute(&output, struct {
		Secret string
	}{
		uuid.NewV4().String(),
	})

	err = ioutil.WriteFile(f, output.Bytes(), 0644)
	if err != nil {
		return err
	}

	err = validateConfig(f)
	if err != nil {
		_ = os.Remove(f)
		return err
	}

	return nil
}

// validateConfig validates config file located at provided path.
func validateConfig(f string) error {
	v := viper.New()
	v.SetConfigFile(f)
	err := v.ReadInConfig()
	if err != nil {
		switch err.(type) {
		case viper.ConfigParseError:
			return err
		default:
			return errors.New("Unable to locate config file, use \"centrifugo genconfig -c " + f + "\" command to generate one")
		}
	}
	c := newConfig()
	return c.Validate()
}

func namespacesFromConfig(v *viper.Viper) []libcentrifugo.Namespace {
	// TODO: as viper does not have exported global config instance
	// we need to use nil when application wants to use global viper
	// config - this must be improved using our own global viper instance
	ns := []libcentrifugo.Namespace{}
	if !viper.IsSet("namespaces") {
		return ns
	}
	if v == nil {
		viper.MarshalKey("namespaces", &ns)
	} else {
		v.MarshalKey("namespaces", &ns)
	}
	return ns
}

func stringInSlice(a string, list []string) bool {
	for _, b := range list {
		if b == a {
			return true
		}
	}
	return false
}
