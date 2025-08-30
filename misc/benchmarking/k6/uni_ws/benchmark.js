import ws from 'k6/ws';
import { check } from 'k6';

export let options = {
  stages: [
    // Ramp up to 5000 connections over 60 seconds
    { duration: '60s', target: 5000 },
    // Hold 5000 connections for 4 minutes
    { duration: '4m', target: 5000 },
    // Ramp down to 0 connections
    { duration: '10s', target: 0 },
  ],
};

export default function () {
  const url = 'ws://localhost:8000/connection/uni_websocket?cf_connect={}';
  
  const response = ws.connect(url, {}, function (socket) {
    socket.on('open', function open() {
      console.log('WebSocket connection established');
    });

    socket.on('message', function message(data) {
      console.log('Received message:', data);
    });

    socket.on('close', function close() {
      console.log('WebSocket connection closed');
    });

    socket.on('error', function error(e) {
      console.log('WebSocket error:', e.error());
    });

    // Keep the connection alive for the duration of the test
    // The connection will be closed when the VU iteration ends
    socket.setTimeout(function () {
      socket.close();
    }, 300000); // 5 minutes timeout as safety measure
    
    // Send a ping every 30 seconds to keep connection active
    socket.setInterval(function () {
      socket.ping();
    }, 30000);
  });

  check(response, { 'status is 101': (r) => r && r.status === 101 });
}
