package telemetry

import (
	"github.com/centrifugal/centrifugo/v6/internal/configtypes"

	"go.opentelemetry.io/otel/sdk/resource"
)

// resourceDetectors returns extra resource detectors to attach to the
// telemetry resource based on the OpenTelemetry configuration.
func resourceDetectors(cfg configtypes.OpenTelemetry) ([]resource.Detector, error) {
	return nil, nil
}
