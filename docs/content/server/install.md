# Install and quick start

Go language gives developers an opportunity to build single binary executable file with application and cross-compile application for all common operating systems. This means that all you need to get Centrifugo – [download latest release](https://github.com/centrifugal/centrifugo/releases) for you operating system, unpack it and you are done!

Now you can see help information for Centrifugo:

```
./centrifugo -h
```

Centrifugo server node requires configuration file with secret key. If you are new to Centrifugo then there is `genconfig` command which generates minimal required configuration file:

```bash
./centrifugo genconfig
```

It generates secret key automatically and creates configuration file `config.json` in current directory (by default) so you can finally run Centrifugo instance:

```bash
./centrifugo --config=config.json
```

We will talk about configuration in detail in next sections.

You can also put or symlink `centrifugo` into your `bin` OS directory and run it from anywhere:

```bash
centrifugo --config=config.json
```

In production you will need to daemonize Centrifugo. We have prebuilt `rpm` and `deb` packages for
most popular Linux distributions and Docker image. See `Deploy` section for more information.
