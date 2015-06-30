Centrifuge + Go = Centrifugo – harder, better, faster, stronger.

Centrifugo is a real-time messaging server. This is a successor of 
[Centrifuge](https://github.com/centrifugal/centrifuge). Please note that it can be used in conjuction with your application backend written in any language - Python, Ruby, Perl, PHP, Javascript, Java, Objective-C etc.

To undestand what is it for and how it works – please, read 
[documentation](http://fzambia.gitbooks.io/centrifugal/content/) of 
Centrifugal organization.

Releases available as single executable files – just 
[download](https://github.com/centrifugal/centrifugo/releases) archive for your platform,  
unpack it and you are done. See also [Docker image](https://registry.hub.docker.com/u/fzambia/centrifugo/).

Try [demo instance](https://centrifugo.herokuapp.com/) on Heroku (password `demo`). Or deploy your own Centrifugo instance in one click:

[![Deploy](https://www.herokucdn.com/deploy/button.png)](https://heroku.com/deploy?template=https://github.com/centrifugal/centrifugo)

Centrifugo server distributed under MIT license.

Highlights:
* Fast server capable to serve lots of simultanious connections
* Sha-256 HMAC-based authorization
* HTTP API to communicate from your application backend (publish messages in channels etc.). API clients for Python, Ruby, PHP. Simple to implement new one.
* Javascript client to connect from web browser over SockJS or pure Websocket protocol
* Presence, history and join/leave events for channels
* Flexible configuration of channels via namespaces
* Different types of channels - private, user limited, client limited channels
* Administrative web interface
* Scale to several machines with Redis
* Ready to deploy (docker, rpm, Nginx configuration)
* Easily integrates with existing web site - no need to rewrite your code

Simplified scheme:

![scheme](https://raw.githubusercontent.com/centrifugal/documentation/master/assets/images/scheme.png)


[![Join the chat at https://gitter.im/centrifugal/centrifugo](https://badges.gitter.im/Join%20Chat.svg)](https://gitter.im/centrifugal/centrifugo?utm_source=badge&utm_medium=badge&utm_campaign=pr-badge&utm_content=badge)
