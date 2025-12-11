package config

import (
	"errors"

	"github.com/centrifugal/centrifugo/v6/internal/configtypes"
	"github.com/centrifugal/centrifugo/v6/internal/tools"
)

// ValidatePublicationData validates publication data according to the specified format.
// Returns an error if the data doesn't match the format requirements.
func ValidatePublicationData(data []byte, format string) error {
	switch format {
	case configtypes.PublicationDataFormatJSON:
		if !tools.IsValidJSON(data) {
			return errors.New("data is not valid JSON")
		}
	case configtypes.PublicationDataFormatBinary:
		// Binary format allows empty data, no validation needed
	default:
		// Default behavior: reject empty data
		if len(data) == 0 {
			return errors.New("data required")
		}
	}
	return nil
}
