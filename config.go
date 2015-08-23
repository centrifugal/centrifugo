package main

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

	"github.com/centrifugal/centrifugo/Godeps/_workspace/src/github.com/nu7hatch/gouuid"
	"github.com/centrifugal/centrifugo/Godeps/_workspace/src/github.com/spf13/viper" // newConfig creates new libcentrifugo.Config using viper.
	"github.com/centrifugal/centrifugo/libcentrifugo"
	"github.com/centrifugal/centrifugo/libcentrifugo/logger"
)

func newConfig() *libcentrifugo.Config {
	cfg := &libcentrifugo.Config{}
	cfg.Version = VERSION
	cfg.Name = getApplicationName()
	cfg.WebPassword = viper.GetString("web_password")
	cfg.WebSecret = viper.GetString("web_secret")
	cfg.ChannelPrefix = viper.GetString("channel_prefix")
	cfg.AdminChannel = libcentrifugo.ChannelID(cfg.ChannelPrefix + "." + "admin")
	cfg.ControlChannel = libcentrifugo.ChannelID(cfg.ChannelPrefix + "." + "control")
	cfg.MaxChannelLength = viper.GetInt("max_channel_length")
	cfg.NodePingInterval = int64(viper.GetInt("node_ping_interval"))
	cfg.NodeInfoCleanInterval = cfg.NodePingInterval * 3
	cfg.NodeInfoMaxDelay = cfg.NodePingInterval*2 + 1
	cfg.PresencePingInterval = int64(viper.GetInt("presence_ping_interval"))
	cfg.PresenceExpireInterval = int64(viper.GetInt("presence_expire_interval"))
	cfg.MessageSendTimeout = int64(viper.GetInt("message_send_timeout"))
	cfg.PrivateChannelPrefix = viper.GetString("private_channel_prefix")
	cfg.NamespaceChannelBoundary = viper.GetString("namespace_channel_boundary")
	cfg.UserChannelBoundary = viper.GetString("user_channel_boundary")
	cfg.UserChannelSeparator = viper.GetString("user_channel_separator")
	cfg.ClientChannelBoundary = viper.GetString("client_channel_boundary")
	cfg.ExpiredConnectionCloseDelay = int64(viper.GetInt("expired_connection_close_delay"))
	cfg.Insecure = viper.GetBool("insecure")
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
	s := structureFromConfig(v)
	return s.Validate()
}

func getGlobalProject(v *viper.Viper) (*libcentrifugo.Project, bool) {
	p := &libcentrifugo.Project{}

	// TODO: the same as for structureFromConfig function
	if v == nil {
		if !viper.IsSet("project_name") || viper.GetString("project_name") == "" {
			return nil, false
		}
		p.Name = libcentrifugo.ProjectKey(viper.GetString("project_name"))
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
		p.Name = libcentrifugo.ProjectKey(v.GetString("project_name"))
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

	var nl []libcentrifugo.Namespace
	if v == nil {
		viper.MarshalKey("project_namespaces", &nl)
	} else {
		v.MarshalKey("project_namespaces", &nl)
	}
	p.Namespaces = nl

	return p, true
}

func structureFromConfig(v *viper.Viper) *libcentrifugo.Structure {
	// TODO: as viper does not have exported global config instance
	// we need to use nil when application wants to use global viper
	// config - this must be improved using our own global viper instance

	var pl []libcentrifugo.Project

	if v == nil {
		viper.MarshalKey("projects", &pl)
	} else {
		v.MarshalKey("projects", &pl)
	}

	// top level project configuration
	p, exists := getGlobalProject(v)
	if exists {
		// add global project to project list
		pl = append([]libcentrifugo.Project{*p}, pl...)
	}

	s := libcentrifugo.NewStructure(pl)

	return s
}

func stringInSlice(a string, list []string) bool {
	for _, b := range list {
		if b == a {
			return true
		}
	}
	return false
}
