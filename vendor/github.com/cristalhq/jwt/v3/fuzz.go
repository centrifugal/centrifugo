// +build gofuzz
// To run the fuzzer, run the following commands:
//		$ GO111MODULE=off go get -u github.com/dvyukov/go-fuzz/go-fuzz github.com/dvyukov/go-fuzz/go-fuzz-build
//		$ cd $GOPATH/src/github.com/cristalhq/jwt/
//		$ go-fuzz-build
//		$ go-fuzz
// Note: go-fuzz doesn't support go modules, so you must have your local
// installation of jwt under $GOPATH.

package jwt

func Fuzz(data []byte) int {
	if _, err := Parse(data); err != nil {
		return 0
	}
	return 1
}
