# Quick start

Here we will build a very simple browser application with Centrifugo. It works in a way that users connect to Centrifugo over WebSocket, subscribe to channel and start receiving all messages published to that channel. In our case we will send a counter value to all channel subscribers to update it in all open browser tabs in real-time.
