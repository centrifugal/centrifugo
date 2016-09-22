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

	"github.com/FZambia/go-logger"
	"github.com/FZambia/viper-lite"
	"github.com/centrifugal/centrifugo/libcentrifugo/config"
	"github.com/satori/go.uuid"
)

// newConfig creates new libcentrifugo.Config using viper.
func newConfig(v *viper.Viper) *config.Config {
	cfg := &config.Config{}
	cfg.Version = VERSION

	cfg.Name = getApplicationName(v)
	cfg.Debug = v.GetBool("debug")
	cfg.Admin = v.GetBool("admin")
	cfg.AdminPassword = v.GetString("admin_password")
	cfg.AdminSecret = v.GetString("admin_secret")
	cfg.Web = v.GetBool("web")
	cfg.WebPath = v.GetString("web_path")
	cfg.HTTPAddress = v.GetString("address")
	cfg.HTTPPort = v.GetString("port")
	cfg.HTTPAdminPort = v.GetString("admin_port")
	cfg.HTTPAPIPort = v.GetString("api_port")
	cfg.HTTPPrefix = v.GetString("http_prefix")
	cfg.SockjsURL = v.GetString("sockjs_url")
	cfg.SSL = v.GetBool("ssl")
	cfg.SSLCert = v.GetString("ssl_cert")
	cfg.SSLKey = v.GetString("ssl_cert")
	cfg.ChannelPrefix = v.GetString("channel_prefix")
	cfg.MaxChannelLength = v.GetInt("max_channel_length")
	cfg.PingInterval = time.Duration(v.GetInt("ping_interval")) * time.Second
	cfg.NodePingInterval = time.Duration(v.GetInt("node_ping_interval")) * time.Second
	cfg.NodeInfoCleanInterval = cfg.NodePingInterval * 3
	cfg.NodeInfoMaxDelay = cfg.NodePingInterval*2 + 1*time.Second
	cfg.NodeMetricsInterval = time.Duration(v.GetInt("node_metrics_interval")) * time.Second
	cfg.PresencePingInterval = time.Duration(v.GetInt("presence_ping_interval")) * time.Second
	cfg.PresenceExpireInterval = time.Duration(v.GetInt("presence_expire_interval")) * time.Second
	cfg.MessageSendTimeout = time.Duration(v.GetInt("message_send_timeout")) * time.Second
	cfg.PrivateChannelPrefix = v.GetString("private_channel_prefix")
	cfg.NamespaceChannelBoundary = v.GetString("namespace_channel_boundary")
	cfg.UserChannelBoundary = v.GetString("user_channel_boundary")
	cfg.UserChannelSeparator = v.GetString("user_channel_separator")
	cfg.ClientChannelBoundary = v.GetString("client_channel_boundary")
	cfg.ExpiredConnectionCloseDelay = time.Duration(v.GetInt("expired_connection_close_delay")) * time.Second
	cfg.StaleConnectionCloseDelay = time.Duration(v.GetInt("stale_connection_close_delay")) * time.Second
	cfg.ClientRequestMaxSize = v.GetInt("client_request_max_size")
	cfg.ClientQueueMaxSize = v.GetInt("client_queue_max_size")
	cfg.ClientQueueInitialCapacity = v.GetInt("client_queue_initial_capacity")
	cfg.ClientChannelLimit = v.GetInt("client_channel_limit")
	cfg.Insecure = v.GetBool("insecure")
	cfg.InsecureAPI = v.GetBool("insecure_api")
	cfg.InsecureAdmin = v.GetBool("insecure_admin") || viper.GetBool("insecure_web")
	cfg.Secret = v.GetString("secret")
	cfg.ConnLifetime = int64(v.GetInt("connection_lifetime"))
	cfg.Watch = v.GetBool("watch")
	cfg.Publish = v.GetBool("publish")
	cfg.Anonymous = v.GetBool("anonymous")
	cfg.Presence = v.GetBool("presence")
	cfg.JoinLeave = v.GetBool("join_leave")
	cfg.HistorySize = v.GetInt("history_size")
	cfg.HistoryLifetime = v.GetInt("history_lifetime")
	cfg.HistoryDropInactive = v.GetBool("history_drop_inactive")
	cfg.Recover = v.GetBool("recover")
	cfg.Namespaces = namespacesFromConfig(v)
	return cfg
}

// getApplicationName returns a name for this node. If no name provided
// in configuration then it constructs node name based on hostname and port
func getApplicationName(v *viper.Viper) string {
	name := v.GetString("name")
	if name != "" {
		return name
	}
	port := v.GetString("port")
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
	c := newConfig(v)
	return c.Validate()
}

func namespacesFromConfig(v *viper.Viper) []config.Namespace {
	ns := []config.Namespace{}
	if !viper.IsSet("namespaces") {
		return ns
	}
	v.UnmarshalKey("namespaces", &ns)
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
