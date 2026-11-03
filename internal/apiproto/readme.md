Centrifugo server API Protobuf definitions.

`api.proto` is the source of truth for the server API: the Go code here, the
swagger definitions in `swagger/` and the server SDKs are generated from it.
Its comments are the documentation of every method, message and field, and
SDK generators read three phrases in them:

- `Required.` - a field a request must set; `Required unless ...` for a
  conditional one.
- `Centrifugo PRO only` - a method or message only Centrifugo PRO has.
- `One of: add, remove, set.` - the values of a string field with a fixed
  set, written as one sentence, which SDKs turn into constants. The field
  stays a string: Centrifugo may accept a value an older SDK does not name.

After changing `api.proto` run `generate.sh` here and `generate_swagger.sh`
in `swagger/`.
