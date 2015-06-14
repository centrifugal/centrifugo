v0.2.0
======

* add optional `client` field to publish API requests. `client` will be added on top level of 
	published message. This means that there is now a way to include `client` connection ID to 
	publish API request to Centrifugo (to get client connection ID call `centrifuge.getClientId()` in 
	javascript). This client will be included in a message as I said above and you can compare
	current client ID in javascript with `client` ID in message and if both values equal then in 
	some situations you will wish to drop this message as it was published by this client and 
	probably already processed (via optimistic optimization or after successful AJAX call to web 
	application backend initiated this message).
* client limited channels - like user limited channels but for client. Only client with ID used in
	channel name can subscribe on such channel. Default client channel boundary is `&`. If you have
	channels with `&` in its name - then you must adapt your channel names to not use `&` or run Centrifugo with another client channel boundary using `client_channel_boundary` configuration
	file option.
* fix for presence and history client calls - check subscription on channel before returning result 
	or return Permission Denied error if not subscribed.
* handle interrupts - unsubscribe clients from all channels. Many thanks again to Mr Klaus Post.	
* code refactoring, detach libcentrifugo real-time core from Centrifugo service.

v0.1.1
======

Lots of internal refactoring, no API changes. Thanks to Mr Klaus Post (@klauspost) and Mr Dmitry Chestnykh (@dchest)

v0.1.0
======

First release. New [documentation](http://fzambia.gitbooks.io/centrifugal/content/).