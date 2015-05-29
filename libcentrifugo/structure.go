package libcentrifugo

import (
	"errors"
	"regexp"
	"sync"
)

// ChannelOptions represent channel specific configuration for namespace or project in a whole
type ChannelOptions struct {
	// Watch determines if message published into channel will be sent into admin channel
	Watch bool `json:"watch"`

	// Publish determines if client can publish messages into channel directly
	Publish bool `json:"publish"`

	// Anonymous determines is anonymous access (with empty user ID) allowed or not
	Anonymous bool `json:"anonymous"`

	// Presence turns on(off) presence information for channels
	Presence bool `json:"presence"`

	// HistorySize determines max amount of history messages for channel, 0 means no history for channel
	HistorySize int64 `mapstructure:"history_size" json:"history_size"`

	// HistoryLifetime determines time in seconds until expiration for history messages
	HistoryLifetime int64 `mapstructure:"history_lifetime" json:"history_lifetime"`

	// JoinLeave turns on(off) join/leave messages for channels
	JoinLeave bool `mapstructure:"join_leave" json:"join_leave"`
}

// project represents single project
// note that although Centrifugo can work with several projects
// but it's recommended to have separate Centrifugo installation
// for every project (maybe except copy of your project for development)
type project struct {
	// Name is unique project name, used as project key for client connections and API requests
	Name ProjectKey `json:"name"`

	// Secret is a secret key for project, used to sign API requests and client connection tokens
	Secret string `json:"secret"`

	// ConnLifetime determines time until connection expire, 0 means no connection expire at all
	ConnLifetime int64 `mapstructure:"connection_lifetime" json:"connection_lifetime"`

	// Namespaces - list of namespaces for project for custom channel options
	Namespaces []namespace `json:"namespaces"`

	// ChannelOptions - default project channel options
	ChannelOptions `mapstructure:",squash"`
}

// namespace allows to create channels with different channel options within the project
type namespace struct {
	// Name is a unique namespace name in project
	Name NamespaceKey `json:"name"`

	// ChannelOptions for namespace determine channel options for channels belonging to this namespace
	ChannelOptions `mapstructure:",squash"`
}

type (
	NamespaceKey string // Namespace ID
	ProjectKey   string // Project ID
	Channel      string // Channel Name
	ChannelID    string // Channel ID (including prefixes)
	UserID       string // User ID
	ConnID       string // Connection ID
)

// structure contains some helper structures and methods to work with projects in namespaces
// in a fast and comfortable way
type structure struct {
	sync.RWMutex
	ProjectList  []project
	ProjectMap   map[ProjectKey]project
	NamespaceMap map[ProjectKey]map[NamespaceKey]namespace
}

// initialize initializes structure fields based on project list
func (s *structure) initialize() {
	s.Lock()
	defer s.Unlock()
	projectMap := map[ProjectKey]project{}
	namespaceMap := map[ProjectKey]map[NamespaceKey]namespace{}
	for _, p := range s.ProjectList {
		projectMap[p.Name] = p
		namespaceMap[p.Name] = map[NamespaceKey]namespace{}
		for _, n := range p.Namespaces {
			namespaceMap[p.Name][n.Name] = n
		}
	}
	s.ProjectMap = projectMap
	s.NamespaceMap = namespaceMap
}

func stringInSlice(a string, list []string) bool {
	for _, b := range list {
		if b == a {
			return true
		}
	}
	return false
}

// validate validates structure and return error if problems found
func (s *structure) validate() error {
	s.Lock()
	defer s.Unlock()
	var projectNames []string
	errPrefix := "config error: "
	pattern := "^[-a-zA-Z0-9_]{2,}$"
	for _, p := range s.ProjectList {
		name := string(p.Name)
		match, _ := regexp.MatchString(pattern, name)
		if !match {
			return errors.New(errPrefix + "wrong project name – " + name)
		}
		if p.Secret == "" {
			return errors.New(errPrefix + "secret required for project – " + name)
		}
		if stringInSlice(name, projectNames) {
			return errors.New(errPrefix + "project name must be unique – " + name)
		}
		projectNames = append(projectNames, name)

		if p.Namespaces == nil {
			continue
		}
		var nss []string
		for _, n := range p.Namespaces {
			name := string(n.Name)
			match, _ := regexp.MatchString(pattern, name)
			if !match {
				return errors.New(errPrefix + "wrong namespace name – " + name)
			}
			if stringInSlice(name, nss) {
				return errors.New(errPrefix + "namespace name must be unique for project – " + name)
			}
			nss = append(nss, name)
		}

	}
	return nil
}

// projectByKey searches for a project with specified key in structure
func (s *structure) projectByKey(pk ProjectKey) (*project, bool) {
	s.RLock()
	defer s.RUnlock()
	project, ok := s.ProjectMap[pk]
	if !ok {
		return nil, false
	}
	return &project, true
}

// channelOpts searches for channel options for specified project and namespace
func (s *structure) channelOpts(pk ProjectKey, ns NamespaceKey) *ChannelOptions {
	s.RLock()
	defer s.RUnlock()
	project, exists := s.projectByKey(pk)
	if !exists {
		return nil
	}
	if ns == "" {
		return &project.ChannelOptions
	} else {
		namespace, exists := s.NamespaceMap[pk][ns]
		if !exists {
			return nil
		}
		return &namespace.ChannelOptions
	}
}
