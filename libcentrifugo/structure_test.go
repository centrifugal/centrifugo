package libcentrifugo

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func getTestChannelOptions() ChannelOptions {
	return ChannelOptions{
		Watch:           true,
		Publish:         true,
		Presence:        true,
		HistorySize:     1,
		HistoryLifetime: 1,
	}
}

func getTestProject(name ProjectKey) Project {
	var ns []Namespace
	ns = append(ns, getTestNamespace("test"))
	return Project{
		Name:           name,
		Secret:         "secret",
		ChannelOptions: getTestChannelOptions(),
		Namespaces:     ns,
	}
}

func getTestNamespace(name NamespaceKey) Namespace {
	return Namespace{
		Name:           name,
		ChannelOptions: getTestChannelOptions(),
	}
}

func getTestStructure() *Structure {
	var pl []Project
	pl = append(pl, getTestProject("test1"))
	pl = append(pl, getTestProject("test2"))
	s := NewStructure(pl)
	return s
}

func TestStructureInitialize(t *testing.T) {
	s := getTestStructure()
	if len(s.projectMap) != 2 {
		t.Error("malformed project map")
	}
	if len(s.namespaceMap) != 2 {
		t.Error("malformed namespace map")
	}
}

func TestGetProjectByKey(t *testing.T) {
	s := getTestStructure()

	_, found := s.projectByKey("test3")
	assert.Equal(t, false, found, "found project that does not exist")

	_, found = s.projectByKey("test2")
	assert.Equal(t, true, found)
}

func TestGetChannelOptions(t *testing.T) {
	s := getTestStructure()

	_, err := s.channelOpts("wrong_project_key", "test")
	assert.Equal(t, ErrProjectNotFound, err)

	_, err = s.channelOpts("test1", "test")
	assert.Equal(t, nil, err)

	_, err = s.channelOpts("test1", "")
	assert.Equal(t, nil, err)

	_, err = s.channelOpts("test1", "wrongnamespacekey")
	assert.Equal(t, ErrNamespaceNotFound, err)
}
