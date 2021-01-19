package jwk

// This package contains some default security settings that determine encryption strength
const (
	SHASizeStr = "256"
	AESSizeStr = "128"
	// SHA-1 is probably safe with OAEP, even if collisions become very easy to compute,
	// but let's throw it away to be on the safe side.
	// Using bloated RSA for encryption is a waste anyway.
	RSAPadding        = "-OAEP-256"
	UseKeyWrapForECDH = true

	// Default Signature Algorithms:
	DefaultECSignAlg   = "ES" + SHASizeStr
	DefaultRSASignAlg  = "RS" + SHASizeStr
	DefaultHMACSignAlg = "HS" + SHASizeStr

	// Default Content Encryption Algorithms:
	DefaultContentEncryptionAlgorithm = "A" + AESSizeStr + "GCM"

	// Default Key Management Algorithms:
	DefaultAESKeyAlg           = "A" + AESSizeStr + "KW"
	DefaultRSAKeyAlg           = "RSA" + RSAPadding
	DefaultECKeyAlg            = "ECDH-ES"
	DefaultECKeyAlgWithKeyWrap = "ECDH-ES+" + DefaultAESKeyAlg
)
