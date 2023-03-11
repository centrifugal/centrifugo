package proxyproto

import "github.com/centrifugal/centrifuge"

func DisconnectFromProto(s *Disconnect) *centrifuge.Disconnect {
	return &centrifuge.Disconnect{
		Code:   s.Code,
		Reason: s.Reason,
	}
}

func ErrorFromProto(s *Error) *centrifuge.Error {
	return &centrifuge.Error{
		Code:      s.Code,
		Message:   s.Message,
		Temporary: s.Temporary,
	}
}
