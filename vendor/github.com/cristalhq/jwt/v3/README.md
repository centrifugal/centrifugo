# jwt

[![Build Status][build-img]][build-url]
[![GoDoc][pkg-img]][pkg-url]
[![Go Report Card][reportcard-img]][reportcard-url]
[![Coverage][coverage-img]][coverage-url]

JSON Web Token for Go [RFC 7519](https://tools.ietf.org/html/rfc7519), also see [jwt.io](https://jwt.io) for more.

The latest version is `v3`.

## Features

* Simple API.
* Clean and tested code.
* Optimized for speed.
* Dependency-free.
* All sign methods supported
  * HMAC (HS)
  * RSA (RS)
  * RSA-PSS (PS)
  * ECDSA (ES)
  * EdDSA (EdDSA)
  * or your own!

## Install

Go version 1.13+

```
go get github.com/cristalhq/jwt/v3
```

## Example

```go
// 1. create a signer & a verifier
key := []byte(`secret`)
signer, err := jwt.NewSignerHS(jwt.HS256, key)
checkErr(err)
verifier, err := jwt.NewVerifierHS(jwt.HS256, key)
checkErr(err)

// 2. create q standard claims
// (you can create your own, see: Example_BuildUserClaims)
claims := &jwt.StandardClaims{
    Audience: []string{"admin"},
    ID:       "random-unique-string",
}

// 3. create a builder
builder := jwt.NewBuilder(signer)

// 4. and build a token
token, err := builder.Build(claims)

// 5. here is your token  :)
var _ []byte = token.Raw() // or just token.String() for string

// 6. parse a token (by example received from a request)
tokenStr := token.String()
newToken, errParse := jwt.ParseString(tokenStr)
checkErr(errParse)

// 7. and verify it's signature
errVerify := verifier.Verify(newToken.Payload(), newToken.Signature())
checkErr(errVerify)

// 8. also you can parse and verify in 1 operation
newToken, err = jwt.ParseAndVerifyString(tokenStr, verifier)
checkErr(err)

// 9. get standard claims
var newClaims jwt.StandardClaims
errClaims := json.Unmarshal(newToken.RawClaims(), &newClaims)
checkErr(errClaims)

// 10. verify claims
var _ bool = newClaims.IsForAudience("admin")
var _ bool = newClaims.IsValidAt(time.Now())
```

Also see examples: [this above](https://github.com/cristalhq/jwt/blob/master/example_test.go), [build](https://github.com/cristalhq/jwt/blob/master/example_build_test.go), [parse](https://github.com/cristalhq/jwt/blob/master/example_parse_test.go).

## Documentation

See [these docs][doc-url].

## License

[MIT License](LICENSE).

[build-img]: https://github.com/cristalhq/jwt/workflows/build/badge.svg
[build-url]: https://github.com/cristalhq/jwt/actions
[pkg-img]: https://pkg.go.dev/badge/cristalhq/jwt/v3
[pkg-url]: https://pkg.go.dev/github.com/cristalhq/jwt/v3
[reportcard-img]: https://goreportcard.com/badge/cristalhq/jwt
[reportcard-url]: https://goreportcard.com/report/cristalhq/jwt
[coverage-img]: https://codecov.io/gh/cristalhq/jwt/branch/master/graph/badge.svg
[coverage-url]: https://codecov.io/gh/cristalhq/jwt
