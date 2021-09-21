package jwtverify

import (
	"encoding/json"
)

var claimsDecoder = &Decoder{}

type Decoder struct{}

func (d *Decoder) DecodeConnectClaims(data []byte) (*ConnectTokenClaims, error) {
	var claims ConnectTokenClaims
	if err := json.Unmarshal(data, &claims); err != nil {
		return nil, err
	}
	return &claims, nil
}

func (d *Decoder) DecodeSubscribeClaims(data []byte) (*SubscribeTokenClaims, error) {
	var claims SubscribeTokenClaims
	if err := json.Unmarshal(data, &claims); err != nil {
		return nil, err
	}
	return &claims, nil
}
