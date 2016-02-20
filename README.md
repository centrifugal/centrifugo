Centrifugo is a real-time messaging server. This is a successor of 
[Centrifuge](https://github.com/centrifugal/centrifuge). Please note that it can be used in conjunction with your application backend written in any language - Python, Ruby, Perl, PHP, Javascript, Java, Objective-C etc.

In a nutshell this is a server running near your application and keeping lots of persistent Websocket or SockJS connections from your application clients (from web browsers or other environments). When some event happens you can broadcast it to all interested clients using Centrifugo API.

To get more information what is it for and how it works – please, read 
[documentation](http://fzambia.gitbooks.io/centrifugal/content/) of 
Centrifugal organization.

Releases available as single executable files – just 
[download](https://github.com/centrifugal/centrifugo/releases) archive for your platform,  
unpack it and you are done. See also [Docker image](https://hub.docker.com/r/centrifugo/centrifugo/) and [packages for 64-bit Debian, Centos and Ubuntu](https://packagecloud.io/FZambia/centrifugo).

Try [demo instance](https://centrifugo.herokuapp.com/) on Heroku (password `demo`). Or deploy your own Centrifugo instance in one click:

[![Deploy](https://www.herokucdn.com/deploy/button.png)](https://heroku.com/deploy?template=https://github.com/centrifugal/centrifugo)

Highlights:
* Fast server capable to serve thousands of simultanious connections
* HTTP API to communicate from your application backend (publish messages in channels etc.). API clients for Python, Ruby, PHP, Go, NodeJS. Simple to implement new one
* Javascript client to connect from web browser over SockJS or pure Websocket protocol
* Scale to several machines with Redis
* SHA-256 HMAC-based authorization
* Different types of channels – private, user limited, client limited channels
* Flexible configuration of channels via namespaces
* Presence information for channels (show all clients in channel)
* History information for channels (last messages sent into channels)
* Join/leave events for channels (client goes online/offline)
* Recover missed messages after network disconnect
* Built-in administrative web interface
* Ready to deploy (docker image, rpm, Nginx configuration)
* Easily integrates with existing application - no need to rewrite your backend code
* MIT license

Simplified scheme:

![scheme](https://raw.githubusercontent.com/centrifugal/documentation/master/assets/images/scheme.png)


[![Join the chat at https://gitter.im/centrifugal/centrifugo](https://badges.gitter.im/Join%20Chat.svg)](https://gitter.im/centrifugal/centrifugo?utm_source=badge&utm_medium=badge&utm_campaign=pr-badge&utm_content=badge)
