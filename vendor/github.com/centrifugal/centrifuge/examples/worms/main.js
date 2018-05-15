paper.install(window);

$(function(){
	// This demo depends on the canvas element
	if(!('getContext' in document.createElement('canvas'))){
		alert('Sorry, it looks like your browser does not support canvas!');
		return false;
    }
    var canvas = $('canvas')[0];
    paper.setup(canvas);

	var doc = $(document)
	var color = randomColor();
	var points = 5;
	var length = 15;
	var tool = new Tool();

	// Generate an unique ID
	var id = Math.round($.now()*Math.random());

	var clients = [];
	var worms = [];
    
    var centrifuge = new Centrifuge('ws://localhost:8000/connection/websocket');

    centrifuge.subscribe("moving", function(message) {
		data = message.data;
        var path = data.path[1];
		if(!(data.id in clients)){
			if (data.id != id) {
				// New user has joined – create new worm.
				worms[data.id] = createWorm(data.color);
			}
		}
		if (data.id !== id) {
        	worms[data.id].segments = path.segments;
			worms[data.id].strokeColor = data.color;
		}

		// Saving the current client state
		clients[data.id] = data;
		clients[data.id].updated = $.now();
    });

    centrifuge.connect();

	var initialized = false;
	var myPath;
	
	// Remove inactive clients after 10 seconds of inactivity
	setInterval(function(){
		for(var ident in clients){
			if($.now() - clients[ident].updated > 10000){
				// Last update was more than 10 seconds ago. 
				// This user has probably closed the page
				delete clients[ident];
				worms[ident].remove();
				delete worms[ident];
			}
		}
	}, 10000);

	function createWorm(color){
		var path = new paper.Path({
			strokeColor: color,
			strokeWidth: 20,
			strokeCap: 'round'
		});

		var start = new paper.Point(Math.random()*100,Math.random()*100);
		for (var i = 0; i < points; i++) {
			path.add(new paper.Point(i * length + start.x, 0 + start.y));
		}

		return path;
	}
	
	function randomColor() {
		colors = ['#5C4B51', '#8CBEB2', '#F3B562', '#F06060']
		return colors[Math.floor(Math.random()*colors.length)];
	}
	

    var lastEmit = $.now();

	paper.tool.onMouseMove = function(event) {
		if (!initialized) {
			// initialize worm on first mouse move.
			myPath = createWorm(color);
			initialized = true;
		}
		myPath.firstSegment.point = event.point;
		for (var i = 0; i < points - 1; i++) {
			var segment = myPath.segments[i];
			var nextSegment = segment.next;
			var vector = new paper.Point(segment.point.x - nextSegment.point.x,segment.point.y - nextSegment.point.y);
			vector.length = length;
			nextSegment.point = new paper.Point(segment.point.x - vector.x,segment.point.y - vector.y);
		}
		myPath.smooth();

		if ($.now() - lastEmit > 5) {
            var data = {
                'name': 'mousemove',
                'payload': {
                    'color'	: color,
                    'path'	: myPath,
                    'id': id
                }
            }
			centrifuge.send(data);
			lastEmit = $.now();
		}
    }

	paper.tool.onMouseUp = function(event) {
		var newColor = myPath.strokeColor;
        while(newColor === myPath.strokeColor){
            newColor = randomColor();
        }
        color = newColor;
        myPath.strokeColor = newColor;
    }
    
    function tick() {
        paper.view.draw();
        requestAnimationFrame(tick);
    }
    requestAnimationFrame(tick);
});
