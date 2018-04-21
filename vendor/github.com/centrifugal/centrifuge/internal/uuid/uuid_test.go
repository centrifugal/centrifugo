// Copyright (C) 2013-2018 by Maxim Bublis <b@codemonkey.ru>
//
// Permission is hereby granted, free of charge, to any person obtaining
// a copy of this software and associated documentation files (the
// "Software"), to deal in the Software without restriction, including
// without limitation the rights to use, copy, modify, merge, publish,
// distribute, sublicense, and/or sell copies of the Software, and to
// permit persons to whom the Software is furnished to do so, subject to
// the following conditions:
//
// The above copyright notice and this permission notice shall be
// included in all copies or substantial portions of the Software.
//
// THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND,
// EXPRESS OR IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF
// MERCHANTABILITY, FITNESS FOR A PARTICULAR PURPOSE AND
// NONINFRINGEMENT. IN NO EVENT SHALL THE AUTHORS OR COPYRIGHT HOLDERS BE
// LIABLE FOR ANY CLAIM, DAMAGES OR OTHER LIABILITY, WHETHER IN AN ACTION
// OF CONTRACT, TORT OR OTHERWISE, ARISING FROM, OUT OF OR IN CONNECTION
// WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE SOFTWARE.

package uuid

import (
	"bytes"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestBytes(t *testing.T) {
	u := UUID{0x6b, 0xa7, 0xb8, 0x10, 0x9d, 0xad, 0x11, 0xd1, 0x80, 0xb4, 0x00, 0xc0, 0x4f, 0xd4, 0x30, 0xc8}

	bytes1 := []byte{0x6b, 0xa7, 0xb8, 0x10, 0x9d, 0xad, 0x11, 0xd1, 0x80, 0xb4, 0x00, 0xc0, 0x4f, 0xd4, 0x30, 0xc8}

	assert.True(t, bytes.Equal(u.Bytes(), bytes1))
}

func TestString(t *testing.T) {
	assert.Equal(t, NamespaceDNS.String(), "6ba7b810-9dad-11d1-80b4-00c04fd430c8")
}

func TestEqual(t *testing.T) {
	assert.True(t, Equal(NamespaceDNS, NamespaceDNS))
	assert.False(t, Equal(NamespaceDNS, NamespaceURL))
}

func TestSetVersion(t *testing.T) {
	u := UUID{}
	u.SetVersion(4)
	assert.Equal(t, u.Version(), V4)
}

func TestVariant(t *testing.T) {
	u1 := UUID{0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00}
	assert.Equal(t, u1.Variant(), VariantNCS)

	u2 := UUID{0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x80, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00}
	assert.Equal(t, u2.Variant(), VariantRFC4122)

	u3 := UUID{0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0xc0, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00}
	assert.Equal(t, u3.Variant(), VariantMicrosoft)

	u4 := UUID{0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0xe0, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00}
	assert.Equal(t, u4.Variant(), VariantFuture)
}

func TestSetVariant(t *testing.T) {
	u := UUID{}
	u.SetVariant(VariantNCS)
	assert.Equal(t, u.Variant(), VariantNCS)
	u.SetVariant(VariantRFC4122)
	assert.Equal(t, u.Variant(), VariantRFC4122)
	u.SetVariant(VariantMicrosoft)
	assert.Equal(t, u.Variant(), VariantMicrosoft)
	u.SetVariant(VariantFuture)
	assert.Equal(t, u.Variant(), VariantFuture)
}

func TestMust(t *testing.T) {
	defer func() {
		assert.NotNil(t, recover())
	}()
	Must(func() (UUID, error) {
		return Nil, fmt.Errorf("uuid: expected error")
	}())
}
