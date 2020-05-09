# jwt

[![Build Status][build-img]][build-url]
[![GoDoc][doc-img]][doc-url]
[![Go Report Card][reportcard-img]][reportcard-url]
[![Coverage][coverage-img]][coverage-url]

JSON Web Tokens for Go

## Features

* Simple API.
* Optimized for speed.
* Dependency-free.

## Install

Go version 1.13

```
go get github.com/cristalhq/jwt
```

## Example

```go
key := []byte(`secret`)
signer, errSigner := jwt.NewHS256(key)
builder := jwt.NewBuilder(signer)

claims := &jwt.StandardClaims{
    Audience: []string{"admin"},
    ID:       "random-unique-string",
}
token, errBuild := builder.Build(claims)

raw := token.Raw() // JWT signed token

errVerify := signer.Verify(token.Payload(), token.Signature())
```

Also see examples: [build](https://github.com/cristalhq/jwt/blob/master/example_build_test.go), [parse](https://github.com/cristalhq/jwt/blob/master/example_parse_test.go).

## Documentation

See [these docs](https://godoc.org/github.com/cristalhq/jwt).

## License

[MIT License](LICENSE).

[build-img]: https://github.com/cristalhq/jwt/workflows/build/badge.svg
[build-url]: https://github.com/cristalhq/jwt/actions
[doc-img]: https://godoc.org/github.com/cristalhq/jwt?status.svg
[doc-url]: https://godoc.org/github.com/cristalhq/jwt
[reportcard-img]: https://goreportcard.com/badge/cristalhq/jwt
[reportcard-url]: https://goreportcard.com/report/cristalhq/jwt
[coverage-img]: https://codecov.io/gh/cristalhq/jwt/branch/master/graph/badge.svg
[coverage-url]: https://codecov.io/gh/cristalhq/jwt
