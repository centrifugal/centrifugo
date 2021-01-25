# jwt

[![build-img]][build-url]
[![pkg-img]][pkg-url]
[![reportcard-img]][reportcard-url]
[![coverage-img]][coverage-url]

JSON Web Token for Go [RFC 7519](https://tools.ietf.org/html/rfc7519), also see [jwt.io](https://jwt.io) for more.

The latest version is `v3`.

## Rationale

There are many JWT libraries, but many of them are hard to use (unclear or fixed API), not optimal (unneeded allocations + strange API). This library addresses all these issues. It's simple to read, to use, memory and CPU conservative.

## Features

* Simple API.
* Clean and tested code.
* Optimized for speed.
* Dependency-free.
* All well-known algorithms are supported
  * HMAC (HS)
  * RSA (RS)
  * RSA-PSS (PS)
  * ECDSA (ES)
  * EdDSA (EdDSA)
  * or your own!

## Install

Go version 1.13+

```
GO111MODULE=on go get github.com/cristalhq/jwt/v3
```

## Example

Build new token:

```go
// create a Signer (HMAC in this example)
key := []byte(`secret`)
signer, err := jwt.NewSignerHS(jwt.HS256, key)
checkErr(err)

// create claims (you can create your own, see: Example_BuildUserClaims)
claims := &jwt.RegisteredClaims{
    Audience: []string{"admin"},
    ID:       "random-unique-string",
}

// create a Builder
builder := jwt.NewBuilder(signer)

// and build a Token
token, err := builder.Build(claims)
checkErr(err)

// here is token as byte slice
var _ []byte = token.Bytes() // or just token.String() for string
```

Parse and verify token:
```go
// create a Verifier (HMAC in this example)
key := []byte(`secret`)
verifier, err := jwt.NewVerifierHS(jwt.HS256, key)
checkErr(err)

// parse a Token (by example received from a request)
tokenStr := `<header.payload.signature>`
token, err := jwt.ParseString(tokenStr, verifier)
checkErr(err)

// and verify it's signature
err = verifier.Verify(token)
checkErr(err)

// also you can parse and verify together
newToken, err = jwt.ParseAndVerifyString(tokenStr, verifier)
checkErr(err)

// get standard claims
var newClaims jwt.StandardClaims
errClaims := json.Unmarshal(newToken.RawClaims(), &newClaims)
checkErr(errClaims)

// verify claims as you 
var _ bool = newClaims.IsForAudience("admin")
var _ bool = newClaims.IsValidAt(time.Now())
```

Also see examples: [example_test.go](https://github.com/cristalhq/jwt/blob/master/example_test.go).

## Documentation

See [these docs][pkg-url].

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
