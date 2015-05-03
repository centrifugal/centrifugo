package libcentrifugo

import (
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
// note that although Centrifuge can work with several projects
// but it's recommended to have separate Centrifuge installation
// for every project (maybe except copy of your project for development)
type project struct {
	// Name is unique project name, used as project key for client connections and API requests
	Name string `json:"name"`

	// Secret is a secret key for project, used to sign API requests and client connection tokens
	Secret string `json:"secret"`

	// ConnectionLifetime determines time until connection expire, 0 means no connection expire at all
	ConnectionLifetime int64 `mapstructure:"connection_lifetime" json:"connection_lifetime"`

	// Namespaces - list of namespaces for project for custom channel options
	Namespaces namespaceList `json:"namespaces"`

	// ChannelOptions - default project channel options
	ChannelOptions `mapstructure:",squash"`
}

// namespace allows to create channels with different channel options within the project
type namespace struct {
	// Name is a unique namespace name in project
	Name string `json:"name"`

	// ChannelOptions for namespace determine channel options for channels belonging to this namespace
	ChannelOptions `mapstructure:",squash"`
}

// namespaceList represents several namespaces within the project
type namespaceList []namespace

type projectList []project

// structure contains some helper structures and methods to work with projects in namespaces
// in a fast and comfortable way
type structure struct {
	sync.Mutex
	ProjectList  projectList
	ProjectMap   map[string]project
	NamespaceMap map[string]map[string]namespace
}

// initialize initializes structure fields based on project list
func (s *structure) initialize() {
	s.Lock()
	defer s.Unlock()
	projectMap := map[string]project{}
	namespaceMap := map[string]map[string]namespace{}
	for _, p := range s.ProjectList {
		projectMap[p.Name] = p
		namespaceMap[p.Name] = map[string]namespace{}
		for _, n := range p.Namespaces {
			namespaceMap[p.Name][n.Name] = n
		}
	}
	s.ProjectMap = projectMap
	s.NamespaceMap = namespaceMap
}

// getProjectByKey searches for a project with specified key in structure
func (s *structure) getProjectByKey(projectKey string) (*project, bool) {
	project, ok := s.ProjectMap[projectKey]
	if !ok {
		return nil, false
	}
	return &project, true
}

// getChannelOptions searches for channel options for specified project and namespace
func (s *structure) getChannelOptions(projectKey, namespaceName string) *ChannelOptions {
	project, exists := s.getProjectByKey(projectKey)
	if !exists {
		return nil
	}
	if namespaceName == "" {
		return &project.ChannelOptions
	} else {
		namespace, exists := s.NamespaceMap[projectKey][namespaceName]
		if !exists {
			return nil
		}
		return &namespace.ChannelOptions
	}
}
