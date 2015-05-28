package libcentrifugo

import (
	"testing"
)

func getTestChannelOptions() ChannelOptions {
	return ChannelOptions{
		Watch:   true,
		Publish: true,
	}
}

func getTestProject(name string) project {
	var ns []namespace
	ns = append(ns, getTestNamespace("test"))
	return project{
		Name:           name,
		Secret:         "secret",
		ChannelOptions: getTestChannelOptions(),
		Namespaces:     ns,
	}
}

func getTestNamespace(name string) namespace {
	return namespace{
		Name:           name,
		ChannelOptions: getTestChannelOptions(),
	}
}

func getTestStructure() *structure {
	var pl []project
	pl = append(pl, getTestProject("test1"))
	pl = append(pl, getTestProject("test2"))
	s := &structure{
		ProjectList: pl,
	}
	return s
}

func TestStructureInitialize(t *testing.T) {
	s := getTestStructure()
	s.initialize()
	if len(s.ProjectMap) != 2 {
		t.Error("malformed project map")
	}
	if len(s.NamespaceMap) != 2 {
		t.Error("malformed namespace map")
	}
}

func TestGetProjectByKey(t *testing.T) {
	s := getTestStructure()
	s.initialize()
	_, found := s.projByKey("test3")
	if found {
		t.Error("found project that does not exist")
	}
	_, found = s.projByKey("test2")
	if !found {
		t.Error("project not found")
	}
}

func TestGetChannelOptions(t *testing.T) {
	s := getTestStructure()
	s.initialize()
	options := s.channelOpts("test1", "test")
	if options == nil {
		t.Error("namespace channel options not found")
	}
	options = s.channelOpts("test1", "")
	if options == nil {
		t.Error("project channel options not found")
	}
	options = s.channelOpts("test1", "notexist")
	if options != nil {
		t.Error("found channel options for namespace that does not exist")
	}
}
