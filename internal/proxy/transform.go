package proxy

import (
	"github.com/centrifugal/centrifugo/v6/internal/configtypes"
	"github.com/centrifugal/centrifugo/v6/internal/proxyproto"
)

func transformConnectResponse(err error, statusToCodeTransforms configtypes.HttpStatusToCodeTransforms) (*proxyproto.ConnectResponse, error) {
	protocolError, protocolDisconnect := transformHTTPStatusError(err, statusToCodeTransforms)
	if protocolError != nil || protocolDisconnect != nil {
		return &proxyproto.ConnectResponse{
			Error:      protocolError,
			Disconnect: protocolDisconnect,
		}, nil
	}
	return nil, err
}

func transformPublishResponse(err error, statusToCodeTransforms configtypes.HttpStatusToCodeTransforms) (*proxyproto.PublishResponse, error) {
	protocolError, protocolDisconnect := transformHTTPStatusError(err, statusToCodeTransforms)
	if protocolError != nil || protocolDisconnect != nil {
		return &proxyproto.PublishResponse{
			Error:      protocolError,
			Disconnect: protocolDisconnect,
		}, nil
	}
	return nil, err
}

func transformSubscribeResponse(err error, statusToCodeTransforms configtypes.HttpStatusToCodeTransforms) (*proxyproto.SubscribeResponse, error) {
	protocolError, protocolDisconnect := transformHTTPStatusError(err, statusToCodeTransforms)
	if protocolError != nil || protocolDisconnect != nil {
		return &proxyproto.SubscribeResponse{
			Error:      protocolError,
			Disconnect: protocolDisconnect,
		}, nil
	}
	return nil, err
}

func transformRPCResponse(err error, statusToCodeTransforms configtypes.HttpStatusToCodeTransforms) (*proxyproto.RPCResponse, error) {
	protocolError, protocolDisconnect := transformHTTPStatusError(err, statusToCodeTransforms)
	if protocolError != nil || protocolDisconnect != nil {
		return &proxyproto.RPCResponse{
			Error:      protocolError,
			Disconnect: protocolDisconnect,
		}, nil
	}
	return nil, err
}

func transformSubRefreshResponse(err error, statusToCodeTransforms configtypes.HttpStatusToCodeTransforms) (*proxyproto.SubRefreshResponse, error) {
	protocolError, protocolDisconnect := transformHTTPStatusError(err, statusToCodeTransforms)
	if protocolError != nil || protocolDisconnect != nil {
		return &proxyproto.SubRefreshResponse{
			Error:      protocolError,
			Disconnect: protocolDisconnect,
		}, nil
	}
	return nil, err
}

func transformRefreshResponse(err error, statusToCodeTransforms configtypes.HttpStatusToCodeTransforms) (*proxyproto.RefreshResponse, error) {
	protocolError, protocolDisconnect := transformHTTPStatusError(err, statusToCodeTransforms)
	if protocolError != nil || protocolDisconnect != nil {
		return &proxyproto.RefreshResponse{
			Error:      protocolError,
			Disconnect: protocolDisconnect,
		}, nil
	}
	return nil, err
}
