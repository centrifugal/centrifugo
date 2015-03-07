if (!window.location.origin) { // Some browsers (mainly IE) do not have this property, so we need to build it manually...
  window.location.origin = window.location.protocol + '//' + window.location.hostname + (window.location.port ? (':' + window.location.port) : '');
}

$(function() {
	var centrifuge = new Centrifuge({
		url: "http://localhost:8000/connection",
		//url: "ws://localhost:8000/connection/websocket",
        protocols_whitelist: [
            'websocket',
            'xdr-streaming',
            'xhr-streaming',
            'iframe-eventsource',
            'iframe-htmlfile',
            'xdr-polling',
            'xhr-polling',
            'iframe-xhr-polling',
            'jsonp-polling'
        ],		
		project: "test",
		debug: true,
		insecure: true
	})
	
	centrifuge.connect();
});

/*
//var sock = new SockJS(window.location.origin+'/connection');
var sock = new WebSocket("ws://localhost:8000/connection/websocket");

sock.onopen = function() {
	// console.log('connection open');
	document.getElementById("status").innerHTML = "connected";
	document.getElementById("send").disabled=false;
};

sock.onmessage = function(e) {
	document.getElementById("output").value += e.data +"\n";
};

sock.onclose = function() {
	// console.log('connection closed');
	document.getElementById("status").innerHTML = "disconnected";
	document.getElementById("send").disabled=true;
};
*/
