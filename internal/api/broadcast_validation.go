package api

import (
	"sync"

	"github.com/centrifugal/centrifugo/v6/internal/config"
	"github.com/centrifugal/centrifugo/v6/internal/configtypes"
)

// validatePublicationData is config.ValidatePublicationData, a variable so
// that tests can count the validations a broadcast makes.
var validatePublicationData = config.ValidatePublicationData

// broadcastDataValidation validates the data of one broadcast. All its
// channels share the data, so the JSON formats, which scan the whole payload,
// are checked once per broadcast rather than once per channel.
type broadcastDataValidation struct {
	data []byte

	jsonOnce       sync.Once
	jsonErr        error
	jsonObjectOnce sync.Once
	jsonObjectErr  error
}

func (v *broadcastDataValidation) validate(format string) error {
	switch format {
	case configtypes.PublicationDataFormatJSON:
		v.jsonOnce.Do(func() { v.jsonErr = validatePublicationData(v.data, format) })
		return v.jsonErr
	case configtypes.PublicationDataFormatJSONObject:
		v.jsonObjectOnce.Do(func() { v.jsonObjectErr = validatePublicationData(v.data, format) })
		return v.jsonObjectErr
	default:
		// The other formats check the data length at most.
		return validatePublicationData(v.data, format)
	}
}
