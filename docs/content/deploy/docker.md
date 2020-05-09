# Docker image

Centrifugo server has docker image [available on Docker Hub](https://hub.docker.com/r/centrifugo/centrifugo/).

```
docker pull centrifugo/centrifugo
```

Run:

```bash
docker run --ulimit nofile=65536:65536 -v /host/dir/with/config/file:/centrifugo -p 8000:8000 centrifugo/centrifugo centrifugo -c config.json
```

Note that docker allows setting `nofile` limits in command-line arguments which is pretty important to handle lots of simultaneous persistent connections and not run out of open file limit (each connection requires one file descriptor).
