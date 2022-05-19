package build

// Version of server. Set to tag in CI during release.
var Version = "0.0.0"

// UsageStatsEndpoint is where we send stats. Set in CI during release.
var UsageStatsEndpoint string

// UsageStatsToken used for usage stats request auth. Set in CI during release.
var UsageStatsToken string
