[![Join the chat at https://gitter.im/centrifugal/centrifugo](https://badges.gitter.im/Join%20Chat.svg)](https://gitter.im/centrifugal/centrifugo?utm_source=badge&utm_medium=badge&utm_campaign=pr-badge&utm_content=badge) [![Join the chat at https://t.me/joinchat/ABFVWBE0AhkyyhREoaboXQ](https://img.shields.io/badge/Telegram-Group-blue.svg)](https://t.me/joinchat/ABFVWBE0AhkyyhREoaboXQ)

Centrifugo is a real-time messaging server. It's language-agnostic and can be used in conjunction with application backend written in any programming language. Centrifugo runs as separate service and keeps persistent Websocket or SockJS connections from your application clients (from web browsers or other environments like iOS/Android apps). When you need to deliver event to your clients in real-time you publish it to Centrifugo API and Centrifugo then broadcasts event to all connected clients interested in this event (i.e. clients subscribed on event channel). In other words – this is PUB/SUB server.

See server [documentation](https://centrifugal.github.io/centrifugo/).

You can also find [this introduction post](https://medium.com/@fzambia/four-years-in-centrifuge-ce7a94e8b1a8) interesting – this is a story and motivation of Centrifugo.

![scheme](https://raw.githubusercontent.com/centrifugal/centrifugo/c2/docs/content/images/scheme_small_wide.png)

### How to install

Releases available as single executable files – just [download latest release](https://github.com/centrifugal/centrifugo/releases) for your platform, unpack and run.

If you are on MacOS:

```
brew tap centrifugal/centrifugo
brew install centrifugo
```

See official [Docker image](https://hub.docker.com/r/centrifugo/centrifugo/).

There are also [packages for 64-bit Debian, Centos and Ubuntu](https://packagecloud.io/FZambia/centrifugo).

### Demo

Try our [demo instance](https://centrifugo2.herokuapp.com/) on Heroku (password `password`). Or deploy your own Centrifugo instance in one click:

[![Deploy](https://www.herokucdn.com/deploy/button.png)](https://heroku.com/deploy?template=https://github.com/centrifugal/centrifugo)

### Highlights

* Fast server capable to serve thousands of simultaneous connections
* Simple to install and cross platform – works on Linux, MacOS and Windows
* Easily integrates with existing application – no need to rewrite your code base to introduce real-time events
* HTTP and GRPC API to communicate from your application backend (publish messages in channels etc)
* JSON and binary Protobuf Websocket protocol 
* SockJS polyfill for web browsers without Websocket support (JSON only)
* Scale with Redis PUB/SUB, Redis Sentinel for high availability, consistent sharding support
* JWT-based user authentication and private channel authorization
* Presence information for channels (show all active clients in channel)
* History information for channels (last messages published into channel)
* Join/leave events for channels (client goes online/offline)
* Automatically recover missed messages after network disconnect
* Built-in administrative web interface
* Ready to deploy (Docker image, RPM/DEB packages, Nginx configuration, automatic Let's Encrypt TLS certificates)
* MIT license

### Support project

If you like Centrifugo and want to thank Centrifugo author you can by him a coffee:

<a href="https://www.buymeacoffee.com/FZambia" target="_blank"><img src="https://www.buymeacoffee.com/assets/img/custom_images/orange_img.png" alt="Buy Me A Coffee" style="height: auto !important;width: auto !important;" ></a>
