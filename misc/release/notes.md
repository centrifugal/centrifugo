No backwards incompatible changes here.

Improvements:

* Massive JSON client protocol performance improvements in decoding multiple commands in a single frame. See [#215](https://github.com/centrifugal/centrifuge/pull/215) for details.
* General JSON client protocol performance improvements for unmarshalling messages (~8-10% according to [#215](https://github.com/centrifugal/centrifuge/pull/215)) 
* Subscribe proxy can now proxy custom `data` from a client passed in a subscribe command.

This release is built with Go 1.17.4.
