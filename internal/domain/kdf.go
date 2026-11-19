package domain

// KDFParams are Argon2id cost parameters.
type KDFParams struct {
	Time      uint32
	MemoryKiB uint32
	Threads   uint8
}
