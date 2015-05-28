package libcentrifugo

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
	"text/template"

	"github.com/centrifugal/centrifugo/libcentrifugo/logger"
	"github.com/nu7hatch/gouuid"
	"github.com/spf13/viper"
)

type config struct {
	// name of this node - provided explicitly by configuration option
	// or constructed from hostname and port
	name string

	// admin web interface password
	webPassword string
	// secret key to generate auth token for admin web interface endpoints
	webSecret string

	// prefix before each channel
	channelPrefix string
	// channel name for admin messages
	adminChannel string
	// channel name for internal control messages between nodes
	controlChannel string
	// maximum length of channel name
	maxChannelLength int

	// in seconds, how often node must send ping control message
	nodePingInterval int64
	// in seconds, how often node must clean information about other running nodes
	nodeInfoCleanInterval int64
	// in seconds, how many seconds node info considered actual
	nodeInfoMaxDelay int64

	// in seconds, how often connected clients must update presence info
	presencePingInterval int64
	// in seconds, how long to consider presence info valid after receiving presence ping
	presenceExpireInterval int64

	// in seconds, an interval given to client to refresh its connection in the end of
	// connection lifetime
	expiredConnectionCloseDelay int64

	// prefix in channel name which indicates that channel is private
	privateChannelPrefix string
	// string separator which must be put after namespace part in channel name
	namespaceChannelBoundary string
	// string separator which must be set before allowed users part in channel name
	userChannelBoundary string
	// separates allowed users in user part of channel name
	userChannelSeparator string

	// insecure turns on insecure mode - when it's turned on then no authentication
	// required at all when connecting to Centrifugo, anonymous access and publish
	// allowed for all channels, no connection check performed. This can be suitable
	// for demonstration or personal usage
	insecure bool
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

func newConfig() *config {
	cfg := &config{}
	cfg.name = getApplicationName()
	cfg.webPassword = viper.GetString("web_password")
	cfg.webSecret = viper.GetString("web_secret")
	cfg.channelPrefix = viper.GetString("channel_prefix")
	cfg.adminChannel = cfg.channelPrefix + "." + "admin"
	cfg.controlChannel = cfg.channelPrefix + "." + "control"
	cfg.maxChannelLength = viper.GetInt("max_channel_length")
	cfg.nodePingInterval = int64(viper.GetInt("node_ping_interval"))
	cfg.nodeInfoCleanInterval = cfg.nodePingInterval * 3
	cfg.nodeInfoMaxDelay = cfg.nodePingInterval*2 + 1
	cfg.presencePingInterval = int64(viper.GetInt("presence_ping_interval"))
	cfg.presenceExpireInterval = int64(viper.GetInt("presence_expire_interval"))
	cfg.privateChannelPrefix = viper.GetString("private_channel_prefix")
	cfg.namespaceChannelBoundary = viper.GetString("namespace_channel_boundary")
	cfg.userChannelBoundary = viper.GetString("user_channel_boundary")
	cfg.userChannelSeparator = viper.GetString("user_channel_separator")
	cfg.expiredConnectionCloseDelay = int64(viper.GetInt("expired_connection_close_delay"))
	cfg.insecure = viper.GetBool("insecure")
	return cfg
}

// exists returns whether the given file or directory exists or not
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
  "project_name": "{{.Name}}",
  "project_secret": "{{.Secret}}"
}
`

var tomlConfigTemplate = `project_name = {{.Name}}
project_secret = {{.Secret}}
`

var yamlConfigTemplate = `project_name: {{.Name}}
project_secret: {{.Secret}}
`

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

	uid, err := uuid.NewV4()
	if err != nil {
		return err
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

	reader := bufio.NewReader(os.Stdin)
	fmt.Print("Enter your project name: ")
	name, _, err := reader.ReadLine()
	if err != nil {
		return err
	}

	var output bytes.Buffer
	t.Execute(&output, struct {
		Name   string
		Secret string
	}{
		strings.Trim(string(name), " "),
		uid.String(),
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

func validateConfig(f string) error {
	v := viper.New()
	v.SetConfigFile(f)
	err := v.ReadInConfig()
	if err != nil {
		return errors.New("unable to locate config file, use \"centrifugo genconfig -c " + f + "\" command to generate one")
	}
	structure := structureFromConfig(v)
	return structure.validate()
}

func getGlobalProject(v *viper.Viper) (*project, bool) {
	p := &project{}

	// TODO: the same as for structureFromConfig function
	if v == nil {
		if !viper.IsSet("project_name") || viper.GetString("project_name") == "" {
			return nil, false
		}
		p.Name = viper.GetString("project_name")
		p.Secret = viper.GetString("project_secret")
		p.ConnLifetime = int64(viper.GetInt("project_connection_lifetime"))
		p.Anonymous = viper.GetBool("project_anonymous")
		p.Watch = viper.GetBool("project_watch")
		p.Publish = viper.GetBool("project_publish")
		p.JoinLeave = viper.GetBool("project_join_leave")
		p.Presence = viper.GetBool("project_presence")
		p.HistorySize = int64(viper.GetInt("project_history_size"))
		p.HistoryLifetime = int64(viper.GetInt("project_history_lifetime"))
	} else {
		if !v.IsSet("project_name") || v.GetString("project_name") == "" {
			return nil, false
		}
		p.Name = v.GetString("project_name")
		p.Secret = v.GetString("project_secret")
		p.ConnLifetime = int64(v.GetInt("project_connection_lifetime"))
		p.Anonymous = v.GetBool("project_anonymous")
		p.Watch = v.GetBool("project_watch")
		p.Publish = v.GetBool("project_publish")
		p.JoinLeave = v.GetBool("project_join_leave")
		p.Presence = v.GetBool("project_presence")
		p.HistorySize = int64(v.GetInt("project_history_size"))
		p.HistoryLifetime = int64(v.GetInt("project_history_lifetime"))
	}

	var nl []namespace
	if v == nil {
		viper.MarshalKey("project_namespaces", &nl)
	} else {
		v.MarshalKey("project_namespaces", &nl)
	}
	p.Namespaces = nl

	return p, true
}

func structureFromConfig(v *viper.Viper) *structure {
	// TODO: as viper does not have exported global config instance
	// we need to use nil when application wants to use global viper
	// config - this must be improved using our own global viper instance

	var pl []project

	if v == nil {
		viper.MarshalKey("projects", &pl)
	} else {
		v.MarshalKey("projects", &pl)
	}

	// top level project configuration
	p, exists := getGlobalProject(v)
	if exists {
		// add global project to project list
		pl = append([]project{*p}, pl...)
	}

	s := &structure{
		ProjectList: pl,
	}

	s.initialize()
	return s
}
