package main

import (
	"time"
)

type channelOptions struct {
	watch           bool
	publish         bool
	anonymousAccess bool
	presence        bool
	history         bool
	historySize     int64
	historyLifetime time.Duration
	joinLeave       bool
}

type project struct {
	name               string
	displayName        string
	connectionCheck    bool
	connectionLifetime time.Duration
	namespaces         namespaceList
	channelOptions
}

type namespace struct {
	name string
	channelOptions
}

type projectList []project

type namespaceList []namespace

type structure struct {
	projects projectList
}
