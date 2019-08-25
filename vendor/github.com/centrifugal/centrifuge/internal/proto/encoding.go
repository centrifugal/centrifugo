package proto

// EncodingType determines connection payload encoding type.
type EncodingType string

const (
	// EncodingTypeJSON means JSON protocol.
	EncodingTypeJSON EncodingType = "json"
	// EncodingTypeBinary means binary payload.
	EncodingTypeBinary EncodingType = "binary"
)
