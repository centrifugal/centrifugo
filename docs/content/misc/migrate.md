# Migration notes from Centrifugo v1

In version 2 of Centrifugo many things changed in backwards incompatible way comparing to version 1. This document aims to help Centrifugo v1 users to migrate their projects to version 2 (if they want to).

### New client protocol and client libraries

In Centrifugo v2 internal client-server protocol changed meaning that old client library version won't work with new server. So first step in migrating - update client libraries to new version with Centrifugo v2 support.

While refactoring client's API changed a bit so you have to adapt your code to those changes.

### Migrate communication with API

Centrifugo v2 simplified communication with API - requests should not be signed with secret key anymore thus you can simply integrate your backend with Centrifugo without using any of our helper libraries - just send JSON API command as POST request to api endpoint. Don't forget to use api key and protect API endpoint with TLS (more information in server API description document).

Centrifugo v1 could process messages published in Redis queue. In v2 this possibility was removed because this technique is not good in terms of error handling and non-deterministic delay before message will be processed by Centrifugo node worker. Migrate to using HTTP or GRPC API.

### Use JWT instead of hand-crafted connection token

In Centrifugo v2 you must use JWT instead of hand-crafted tokens of v1. This means that you need to download JWT library for your language (there are plenty of them – see jwt.io) and build connection token with it.

See dedicated docs chapter to see how token can be built. 

All connection information will be passed inside this single token string. This means you only need to pass one string to your frontend. No need to pass `user`, `timestamp`, `info` anymore. This also means that you will have less problems with escaping features of template engines - because JWT is safe base64 string. 

### Use JWT instead of hand-crafted signature for private subscriptions

Read chapter about private subscriptions to find how you should now use JWT for private channel subscriptions.

