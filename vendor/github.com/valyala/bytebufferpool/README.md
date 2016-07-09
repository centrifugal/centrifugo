[![Build Status](https://travis-ci.org/valyala/bytebufferpool.svg)](https://travis-ci.org/valyala/bytebufferpool)
[![GoDoc](https://godoc.org/github.com/valyala/bytebufferpool?status.svg)](http://godoc.org/github.com/valyala/bytebufferpool)
[![Go Report](http://goreportcard.com/badge/valyala/bytebufferpool)](http://goreportcard.com/report/valyala/bytebufferpool)

# bytebufferpool

An implementation of a pool of byte buffers with anti-memory-waste protection.

The pool may waste limited amount of memory due to fragmentation.
This amount equals to the maximum total size of the byte buffers
in concurrent use.

# bytebufferpool users

* [fasthttp](https://github.com/valyala/fasthttp)
* [quicktemplate](https://github.com/valyala/quicktemplate)
