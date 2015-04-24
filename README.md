An attempt to move Centrifuge from Python to Go

Why:

* performance
* no need to launch several processes to utilize several cores
* static binary

What's left:

- [ ] implement presence ping
- [ ] memory engine presence and history
- [ ] redis engine
- [ ] web interface HTTP and websocket handlers
- [ ] metrics
- [ ] tests
- [ ] deploy (Dockerfile, rpm) 
- [ ] documentation
- [ ] compare with python version (cpu, memory, performance etc)