#!/bin/bash

set -e

echo "replacing tags of structs..."

gomodifytags -file api.pb.go -field User -struct ClientInfo -all -w -remove-options json=omitempty >/dev/null
gomodifytags -file api.pb.go -field Client -struct ClientInfo -all -w -remove-options json=omitempty >/dev/null
gomodifytags -file api.pb.go -field Presence -struct PresenceResult -all -w -remove-options json=omitempty >/dev/null
gomodifytags -file api.pb.go -field NumClients -struct PresenceStatsResult -all -w -remove-options json=omitempty >/dev/null
gomodifytags -file api.pb.go -field NumUsers -struct PresenceStatsResult -all -w -remove-options json=omitempty >/dev/null
gomodifytags -file api.pb.go -field Offset -struct HistoryResult -all -w -remove-options json=omitempty >/dev/null
gomodifytags -file api.pb.go -field Epoch -struct HistoryResult -all -w -remove-options json=omitempty >/dev/null
gomodifytags -file api.pb.go -field Publications -struct HistoryResult -all -w -remove-options json=omitempty >/dev/null
gomodifytags -file api.pb.go -field Uid -struct NodeResult -all -w -remove-options json=omitempty >/dev/null
gomodifytags -file api.pb.go -field Name -struct NodeResult -all -w -remove-options json=omitempty >/dev/null
gomodifytags -file api.pb.go -field Version -struct NodeResult -all -w -remove-options json=omitempty >/dev/null
gomodifytags -file api.pb.go -field NumClients -struct NodeResult -all -w -remove-options json=omitempty >/dev/null
gomodifytags -file api.pb.go -field NumUsers -struct NodeResult -all -w -remove-options json=omitempty >/dev/null
gomodifytags -file api.pb.go -field NumChannels -struct NodeResult -all -w -remove-options json=omitempty >/dev/null
gomodifytags -file api.pb.go -field NumSubs -struct NodeResult -all -w -remove-options json=omitempty >/dev/null
gomodifytags -file api.pb.go -field Channels -struct ChannelsResult -all -w -remove-options json=omitempty >/dev/null
gomodifytags -file api.pb.go -field Connections -struct ConnectionsResult -all -w -remove-options json=omitempty >/dev/null
gomodifytags -file api.pb.go -field Statuses -struct GetUserStatusResult -all -w -remove-options json=omitempty >/dev/null
gomodifytags -file api.pb.go -field Items -struct DeviceListResult -all -w -remove-options json=omitempty >/dev/null
gomodifytags -file api.pb.go -field Items -struct DeviceTopicListResult -all -w -remove-options json=omitempty >/dev/null
gomodifytags -file api.pb.go -field Items -struct UserTopicListResult -all -w -remove-options json=omitempty >/dev/null
gomodifytags -file api.pb.go -field Allowed -struct RateLimitResult -all -w -remove-options json=omitempty >/dev/null
gomodifytags -file api.pb.go -field AllowedInMs -struct RateLimitResult -all -w -remove-options json=omitempty >/dev/null
gomodifytags -file api.pb.go -field Allowed -struct RateLimitPolicyEvaluation -all -w -remove-options json=omitempty >/dev/null
gomodifytags -file api.pb.go -field TokensLeft -struct RateLimitPolicyEvaluation -all -w -remove-options json=omitempty >/dev/null
gomodifytags -file api.pb.go -field AllowedInMs -struct RateLimitPolicyEvaluation -all -w -remove-options json=omitempty >/dev/null
