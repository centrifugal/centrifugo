package tools

// GetAcceptProtocolLabel returns the transport accept protocol label based on HTTP version.
func GetAcceptProtocolLabel(protoMajor int) string {
	switch protoMajor {
	case 3:
		return "h3"
	case 2:
		return "h2"
	case 1:
		return "h1"
	default:
		return "unknown"
	}
}
