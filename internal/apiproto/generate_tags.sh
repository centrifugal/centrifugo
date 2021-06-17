#!/bin/bash

set -e

echo "replacing tags of structs..."

gomodifytags -file api.pb.go -field User -struct ClientInfo -all -w -remove-options json=omitempty >/dev/null
gomodifytags -file api.pb.go -field Client -struct ClientInfo -all -w -remove-options json=omitempty >/dev/null
gomodifytags -file api.pb.go -field NumClients -struct PresenceStatsResult -all -w -remove-options json=omitempty >/dev/null
gomodifytags -file api.pb.go -field NumUsers -struct PresenceStatsResult -all -w -remove-options json=omitempty >/dev/null
gomodifytags -file api.pb.go -field Offset -struct HistoryResult -all -w -remove-options json=omitempty >/dev/null
gomodifytags -file api.pb.go -field Epoch -struct HistoryResult -all -w -remove-options json=omitempty >/dev/null
gomodifytags -file api.pb.go -field Uid -struct NodeResult -all -w -remove-options json=omitempty >/dev/null
gomodifytags -file api.pb.go -field Name -struct NodeResult -all -w -remove-options json=omitempty >/dev/null
gomodifytags -file api.pb.go -field Version -struct NodeResult -all -w -remove-options json=omitempty >/dev/null
gomodifytags -file api.pb.go -field NumClients -struct NodeResult -all -w -remove-options json=omitempty >/dev/null
gomodifytags -file api.pb.go -field NumUsers -struct NodeResult -all -w -remove-options json=omitempty >/dev/null
gomodifytags -file api.pb.go -field NumChannels -struct NodeResult -all -w -remove-options json=omitempty >/dev/null
