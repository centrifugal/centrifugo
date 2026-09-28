#!/usr/bin/env python3
"""Generate api.swagger.proto from ../api.proto.

api.proto is the single source of truth for the server API schema. The swagger
proto is api.proto plus OpenAPI annotations: the swagger header options below
and a google.api.http + openapiv2_operation option on every rpc. Messages and
enums are copied from api.proto as is.

When adding a new rpc to api.proto, add an entry to OPERATIONS below – the
script fails if some rpc has no entry (or an entry has no rpc).

Run from this directory (generate_swagger.sh does this):

    python3 generate_swagger_proto.py
"""

import re
import sys

SOURCE = "../api.proto"
TARGET = "api.swagger.proto"

# Tags in the order they appear in swagger UI.
TAGS = [
    "publication",
    "connection management",
    "history",
    "presence",
    "stats",
    "user status",
    "user block",
    "token",
    "push notification",
    "batch",
    "rpc",
    "map",
    "shared poll",
]

# rpc name -> (tag, summary). HTTP path is /<snake_case rpc name>, see http_path.
OPERATIONS = {
    "Batch": ("batch", "Batch request (send many commands in one request)"),
    "Publish": ("publication", "Publish data into channel"),
    "Broadcast": ("publication", "Broadcast allows publishing same data into many channels"),
    "Subscribe": ("connection management", "Subscribe connection(s) to a channel"),
    "Unsubscribe": ("connection management", "Unsubscribe connection(s) from channel"),
    "Disconnect": ("connection management", "Disconnect client(s) from server"),
    "Presence": ("presence", "Presence information for a channel"),
    "PresenceStats": ("presence", "Presence stats information for a channel"),
    "History": ("history", "History for a channel"),
    "HistoryRemove": ("history", "Remove history for a channel"),
    "Info": ("stats", "Info shows details about server nodes"),
    "RPC": ("rpc", "Execute RPC"),
    "Refresh": ("connection management", "Refresh connection(s) by server-side call"),
    "Channels": ("stats", "Channels request"),
    "Connections": ("stats", "Connections request"),
    "UpdateUserStatus": ("user status", "Updated user status request"),
    "GetUserStatus": ("user status", "Get user status request"),
    "DeleteUserStatus": ("user status", "Delete user status request"),
    "BlockUser": ("user block", "Block user request"),
    "UnblockUser": ("user block", "Unblock user request"),
    "RevokeToken": ("token", "Revoke token request"),
    "InvalidateUserTokens": ("token", "Invalidate user tokens request"),
    "DeviceRegister": ("push notification", "Device register request"),
    "DeviceUpdate": ("push notification", "Device update request"),
    "DeviceRemove": ("push notification", "Device remove request"),
    "DeviceList": ("push notification", "List devices"),
    "DeviceTopicList": ("push notification", "List device topics"),
    "DeviceTopicUpdate": ("push notification", "Update device topic model"),
    "UserTopicList": ("push notification", "List user topics"),
    "UserTopicUpdate": ("push notification", "Update user topic model"),
    "SendPushNotification": ("push notification", "Send push notification"),
    "UpdatePushStatus": ("push notification", "Update push notification status"),
    "CancelPush": ("push notification", "Cancel delayed push notification"),
    "MapPublish": ("map", "Publish a key into a map channel"),
    "MapRemove": ("map", "Remove a key from a map channel"),
    "MapReadState": ("map", "Read current state of a map channel"),
    "MapReadStream": ("map", "Read the stream of a map channel"),
    "MapStats": ("map", "Stats for a map channel"),
    "MapClear": ("map", "Clear a map channel"),
    "SharedPollPublish": ("shared poll", "Publish into a shared poll channel"),
}

HEADER = """import "google/api/annotations.proto";
import "protoc-gen-openapiv2/options/annotations.proto";

option (grpc.gateway.protoc_gen_openapiv2.options.openapiv2_swagger) = {
  info: {
    title: "Centrifugo server HTTP API";
    version: "6.0";
    description: "";
  };
  external_docs: {
    url: "https://centrifugal.dev/docs/server/server_api";
    description: "More about Centrifugo HTTP API";
  }
  consumes: "application/json";
  produces: "application/json";
  base_path: "/api";
  tags: [
%(tags)s
  ];
  responses: {
    key: "400";
    value: {
      description: "Returned in case of invalid request."
    }
  }
  responses: {
    key: "401";
    value: {
      description: "Returned in case of missing auth."
    }
  }
  responses: {
    key: "500";
    value: {
      description: "Returned in case of internal server error."
    }
  }
  security_definitions: {
    security: {
      key: "ApiKeyAuth";
      value: {
        type: TYPE_API_KEY;
        in: IN_HEADER;
        name: "X-API-Key";
      }
    }
  }
  security: {
    security_requirement: {
      key: "ApiKeyAuth";
      value: {};
    }
  }
};
"""

RPC = """  rpc %(name)s (%(request)s) returns (%(response)s) {
    option (google.api.http) = {
      post: "%(path)s",
      body: "*"
    };
    option (grpc.gateway.protoc_gen_openapiv2.options.openapiv2_operation) = {
      summary: "%(summary)s";
      tags: ["%(tag)s"];
    };
  }"""

SERVICE_RE = re.compile(r"^service CentrifugoApi \{\n(.*?)^\}\n", re.M | re.S)
RPC_RE = re.compile(r"^\s*rpc\s+(\w+)\s*\((\w+)\)\s*returns\s*\((\w+)\)\s*\{\s*\}\s*$")
GO_PACKAGE_RE = re.compile(r'^option go_package = .*;\n', re.M)


def fail(msg):
    sys.exit("generate_swagger_proto.py: " + msg)


def http_path(rpc_name):
    # PresenceStats -> /presence_stats, RPC -> /rpc.
    return "/" + re.sub(r"(?<=[a-z0-9])(?=[A-Z])", "_", rpc_name).lower()


def render_tags():
    return ",\n".join('    {\n      name: "%s"\n    }' % tag for tag in TAGS)


def render_service(body):
    rpcs = []
    for line in body.splitlines():
        stripped = line.strip()
        if not stripped:
            continue
        if stripped.startswith("//"):
            rpcs.append("  " + stripped)
            continue
        m = RPC_RE.match(line)
        if not m:
            fail("unexpected line in CentrifugoApi service: %r" % line)
        name, request, response = m.groups()
        if name not in OPERATIONS:
            fail("no OPERATIONS entry for rpc %s, add one" % name)
        tag, summary = OPERATIONS[name]
        rpcs.append(RPC % dict(
            name=name, request=request, response=response,
            path=http_path(name), summary=summary, tag=tag,
        ))
    return "service CentrifugoApi {\n%s\n}\n" % "\n".join(rpcs)


def main():
    for name, (tag, _) in OPERATIONS.items():
        if tag not in TAGS:
            fail("rpc %s uses tag %r which is not in TAGS" % (name, tag))

    with open(SOURCE) as f:
        source = f.read()

    service = SERVICE_RE.search(source)
    if not service:
        fail("service CentrifugoApi not found in " + SOURCE)
    rpc_names = set(RPC_RE.match(l).group(1) for l in service.group(1).splitlines() if RPC_RE.match(l))
    stale = sorted(set(OPERATIONS) - rpc_names)
    if stale:
        fail("OPERATIONS has entries for rpcs missing in %s: %s" % (SOURCE, ", ".join(stale)))

    out = source[:service.start()] + render_service(service.group(1)) + source[service.end():]

    go_package = GO_PACKAGE_RE.search(out)
    if not go_package:
        fail("option go_package not found in " + SOURCE)
    header = "\n" + HEADER % dict(tags=render_tags())
    out = out[:go_package.end()] + header + out[go_package.end():]

    with open(TARGET, "w") as f:
        f.write(out)


if __name__ == "__main__":
    main()
