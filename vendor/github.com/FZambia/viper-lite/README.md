Viper-lite
==========

Viper-lite is a lightweight opinionated fork of Steve Francia's [viper](https://github.com/spf13/viper) library for application configuration.

I made it because of many external dependencies that original `viper` currently requires. This results in large list of vendored libs in the end product. As I don't need most of those features I decided to remove some functionality and made this fork. 

So what's out of the box:

* remote providers
* fsnotify
* HCL and Java properties config file formats
* afero filesystem support

Another features are here and have the same API. If you are OK with lack of features above than all you need is change viper import path in your code.

Original license information is kept in this repo unmodified.

Documentation
-------------

- [API Reference](http://godoc.org/github.com/FZambia/viper-lite)

Installation
------------

Install using the "go get" command:

```
go get github.com/FZambia/viper-lite
```

License
-------

MIT license.
