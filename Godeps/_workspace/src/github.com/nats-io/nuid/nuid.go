// Copyright 2016 Apcera Inc. All rights reserved.

// A unique identifier generator that is high performance, very fast, and entropy pool friendly.
package nuid

import (
	"crypto/rand"
	"fmt"
	"math"
	"math/big"
	"sync"
	"time"

	prand "math/rand"
)

// NUID needs to be very fast to generate and truly unique, all while being entropy pool friendly.
// We will use 12 bytes of crypto generated data (entropy draining), and 10 bytes of sequential data
// that is started at a pseudo random number and increments with a pseudo-random increment.
// Total is 22 bytes of base 36 ascii text :)

const (
	digits   = "0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZ"
	base     = 36
	preLen   = 12
	seqLen   = 10
	maxPre   = int64(4738381338321616896) // base^preLen == 36^12
	maxSeq   = int64(3656158440062976)    // base^seqLen == 36^10
	minInc   = int64(33)
	maxInc   = int64(333)
	totalLen = preLen + seqLen
)

type NUID struct {
	pre []byte
	seq int64
	inc int64
}

type lockedNUID struct {
	sync.Mutex
	*NUID
}

// Global NUID
var globalNUID *lockedNUID

// Seed sequential random with crypto or math/random and current time
// and generate crypto prefix.
func init() {
	r, err := rand.Int(rand.Reader, big.NewInt(math.MaxInt64))
	if err != nil {
		prand.Seed(time.Now().UnixNano())
	} else {
		prand.Seed(r.Int64())
	}
	globalNUID = &lockedNUID{NUID: New()}
	globalNUID.RandomizePrefix()
}

// New will generate a new NUID and properly initialize the prefix, sequential start, and sequential increment.
func New() *NUID {
	n := &NUID{
		seq: prand.Int63n(maxSeq),
		inc: minInc + prand.Int63n(maxInc-minInc),
		pre: make([]byte, preLen),
	}
	n.RandomizePrefix()
	return n
}

// Generate the next NUID string from the global locked NUID instance.
func Next() string {
	globalNUID.Lock()
	nuid := globalNUID.Next()
	globalNUID.Unlock()
	return nuid
}

// Generate the next NUID string.
func (n *NUID) Next() string {
	// Increment and capture.
	n.seq += n.inc
	if n.seq >= maxSeq {
		n.RandomizePrefix()
		n.resetSequential()
	}
	seq := n.seq

	// Copy prefix
	var b [totalLen]byte
	bs := b[:preLen]
	copy(bs, n.pre)

	// copy in the seq in base36.
	for i, l := len(b), seq; i > preLen; l /= base {
		i -= 1
		b[i] = digits[l%base]
	}
	return string(b[:])
}

// Resets the sequential portion of the NUID.
func (n *NUID) resetSequential() {
	n.seq = prand.Int63n(maxSeq)
	n.inc = minInc + prand.Int63n(maxInc-minInc)
}

// Generate a new prefix from crypto/rand.
// This will drain entropy and will be called automatically when we exhaust the sequential
// Will panic if it gets an error from rand.Int()
func (n *NUID) RandomizePrefix() {
	r, err := rand.Int(rand.Reader, big.NewInt(maxPre))
	if err != nil {
		panic(fmt.Sprintf("nuid: failed generating crypto random number: %v\n", err))
	}
	i := len(n.pre)
	for l := r.Int64(); i > 0; l /= base {
		i -= 1
		n.pre[i] = digits[l%base]
	}
}
