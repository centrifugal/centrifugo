package centrifuge

import (
	"bytes"
	"fmt"
	"math"
	"sync"

	"github.com/dgrijalva/jwt-go"
)

const (
	maxSeq uint32 = math.MaxUint32 // maximum uint32 value
	maxGen uint32 = math.MaxUint32 // maximum uint32 value
)

func nextSeqGen(currentSeq, currentGen uint32) (uint32, uint32) {
	var nextSeq uint32
	nextGen := currentGen
	if currentSeq == maxSeq {
		nextSeq = 0
		nextGen++
	} else {
		nextSeq = currentSeq + 1
	}
	return nextSeq, nextGen
}

func uint64Sequence(currentSeq, currentGen uint32) uint64 {
	return uint64(currentGen)*uint64(math.MaxUint32) + uint64(currentSeq)
}

func unpackUint64(val uint64) (uint32, uint32) {
	return uint32(val), uint32(val >> 32)
}

func stringInSlice(a string, list []string) bool {
	for _, b := range list {
		if b == a {
			return true
		}
	}
	return false
}

var bufferPool = sync.Pool{
	// New is called when a new instance is needed
	New: func() interface{} {
		return new(bytes.Buffer)
	},
}

func getBuffer() *bytes.Buffer {
	return bufferPool.Get().(*bytes.Buffer)
}

func putBuffer(buf *bytes.Buffer) {
	buf.Reset()
	bufferPool.Put(buf)
}

func jwtKeyFunc(config Config) func(token *jwt.Token) (interface{}, error) {
	return func(token *jwt.Token) (interface{}, error) {
		switch token.Method.(type) {
		case *jwt.SigningMethodHMAC:
			if config.TokenHMACSecretKey == "" && config.Secret == "" {
				return nil, fmt.Errorf("token HMAC secret key not set")
			}
			if config.Secret != "" {
				return []byte(config.Secret), nil
			}
			return []byte(config.TokenHMACSecretKey), nil
		case *jwt.SigningMethodRSA:
			if config.TokenRSAPublicKey == nil {
				return nil, fmt.Errorf("token RSA public key not set")
			}
			return config.TokenRSAPublicKey, nil
		default:
			return nil, fmt.Errorf("unsupported signing method: %v", token.Header["alg"])
		}
	}
}
