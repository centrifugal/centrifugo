package jwktypes

const (
	// EC represents elliptic curve keys used with NIST curves
	EC = "EC"

	// RSA represents RSA keys
	RSA = "RSA"

	// OKP represents octet key pairs (essentially a pair of raw byte arrays)
	// used with safe elliptic curve algorithm such as Ed25519 and X25519
	OKP = "OKP"

	// OctetKey is essentially a raw byte array often used with symmetric
	// algorithms such as HMAC, ChaPoly or different AES modes.
	OctetKey = "oct"
)
