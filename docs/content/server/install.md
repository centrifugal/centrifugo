# Server overview and installation

!!!danger

    This is a documentation for Centrifugo v2. The latest Centrifugo version is v3. Go to the [centrifugal.dev](https://centrifugal.dev) for v3 docs.

Centrifugo server written in Go language. It's an open-source software, the source code is available [on Github](https://github.com/centrifugal/centrifugo).

Centrifugo is built around [Centrifuge](https://github.com/centrifugal/centrifuge) library for Go language. That library defines custom protocol and message types which must be sent over various transports (Websocket, SockJS). Server clients use that protocol internally and provide simple API to features - making persistent connection, subscribing on channels, calling RPC commands and more.

Server documentation covers a lot of server concepts in detail. Here we start with ways to install Centrifugo on your system. 

## Install from binary release

Binary releases available on Github. [Download latest release](https://github.com/centrifugal/centrifugo/releases) for your operating system, unpack it and you are done. Centrifugo is pre-built for:

* Linux 64-bit (linux_amd64)
* Linux 32-bit (linux_386)
* MacOS (darwin_amd64)
* Windows (windows_amd64)
* FreeBSD (freebsd_amd64)
* ARM v6 (linux_armv6)

Archives contain a single statically compiled binary `centrifugo` file which is ready to run: 

```
./centrifugo -h
```

See version of Centrifugo:

```
./centrifugo version
```

Centrifugo server node requires configuration file with some secret keys. If you are new to Centrifugo then there is `genconfig` command which generates minimal required configuration file:

```bash
./centrifugo genconfig
```

It generates secret keys automatically and creates configuration file `config.json` in a current directory (by default) so you can finally run Centrifugo instance:

```bash
./centrifugo --config=config.json
```

We will talk about a configuration in detail in next sections.

You can also put or symlink `centrifugo` into your `bin` OS directory and run it from anywhere:

```bash
centrifugo --config=config.json
```

## Docker image

Centrifugo server has docker image [available on Docker Hub](https://hub.docker.com/r/centrifugo/centrifugo/).

```
docker pull centrifugo/centrifugo
```

Run:

```bash
docker run --ulimit nofile=65536:65536 -v /host/dir/with/config/file:/centrifugo -p 8000:8000 centrifugo/centrifugo centrifugo -c config.json
```

Note that docker allows setting `nofile` limits in command-line arguments which is pretty important to handle lots of simultaneous persistent connections and not run out of open file limit (each connection requires one file descriptor). See also [OS tuning chapter](../deploy/tuning.md).

## Docker-compose example

Create configuration file `config.json`:

```json
{
  "v3_use_offset": true,
  "token_hmac_secret_key": "my_secret",
  "api_key": "my_api_key",
  "admin_password": "password",
  "admin_secret": "secret",
  "admin": true
}
```

Create `docker-compose.yml`:

```yml
centrifugo:
  container_name: centrifugo
  image: centrifugo/centrifugo:latest
  volumes:
    - ./config.json:/centrifugo/config.json
  command: centrifugo -c config.json
  ports:
    - 8000:8000
  ulimits:
    nofile:
      soft: 65535
      hard: 65535
```

Run with:

```
docker-compose up
```

## Kubernetes Helm chart

Official Kubernetes Helm chart available and [located on Github](https://github.com/centrifugal/helm-charts). Follow instructions in repository README to bootstrap Centrifugo inside your Kubernetes cluster.

## RPM and DEB packages for Linux

Every time we make new Centrifugo release we upload rpm and deb packages for popular linux distributions on [packagecloud.io](https://packagecloud.io/FZambia/centrifugo).

At moment, we support versions of the following distributions:

* 64-bit Debian 8 Jessie
* 64-bit Debian 9 Stretch
* 64-bit Debian 10 Buster
* 64-bit Ubuntu 16.04 Xenial
* 64-bit Ubuntu 18.04 Bionic
* 64-bit Ubuntu 20.04 Focal Fossa
* 64-bit Centos 7
* 64-bit Centos 8

See [full list of available packages](https://packagecloud.io/FZambia/centrifugo) and [installation instructions](https://packagecloud.io/FZambia/centrifugo/install).

Centrifugo also works on 32-bit architecture, but we don't support packaging for it since 64-bit is more convenient for servers today.

## With brew on MacOS

If you are developing on MacOS then you can install Centrifugo over `brew`:

```
brew tap centrifugal/centrifugo
brew install centrifugo
```

## Build from source

You need Go language installed:

```
git clone https://github.com/centrifugal/centrifugo.git
cd centrifugo
go build
./centrifugo
```
