package main

// ChannelOptions represent channel specific configuration for namespace or project in a whole
type ChannelOptions struct {
	Watch           bool  `json:"watch"`
	Publish         bool  `json:"publish"`
	Anonymous       bool  `json:"anonymous"`
	Presence        bool  `json:"presence"`
	History         bool  `json:"history"`
	HistorySize     int64 `mapstructure:"history_size" json:"history_size"`
	HistoryLifetime int64 `mapstructure:"history_lifetime" json:"history_lifetime"`
	JoinLeave       bool  `mapstructure:"join_leave" json:"join_leave"`
}

// project represents single project
// note that although Centrifuge can work with several projects
// but it's recommended to have separate Centrifuge installation
// for every project (maybe except copy of your project for development)
type project struct {
	Name               string        `json:"name"`
	Secret             string        `json:"secret"`
	ConnectionCheck    bool          `mapstructure:"connection_check" json:"connection_check"`
	ConnectionLifetime int64         `mapstructure:"connection_lifetime" json:"connection_lifetime"`
	Namespaces         namespaceList `json:"namespaces"`
	ChannelOptions     `mapstructure:",squash"`
}

// namespace allows to create channels with different channel options within the project
type namespace struct {
	Name           string `json:"name"`
	ChannelOptions `mapstructure:",squash"`
}

// namespaceList represents several namespaces within the project
type namespaceList []namespace

// structure contains some helper structures and methods to work with projects in namespaces
// in a fast and comfortable way
type structure struct {
	ProjectList  []project
	ProjectMap   map[string]project
	NamespaceMap map[string]map[string]namespace
}

func (s *structure) getProjectByKey(projectKey string) (project, bool) {

}
