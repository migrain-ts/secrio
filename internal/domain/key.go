package domain

import "fmt"

// Key is a 32-byte symmetric key. It never prints its bytes.
type Key [32]byte

const redactedKey = "Key{<redacted>}"

func (Key) String() string   { return redactedKey }
func (Key) GoString() string { return redactedKey }

// Format redacts the key for every fmt verb (including %x and %v).
func (Key) Format(f fmt.State, _ rune) { _, _ = f.Write([]byte(redactedKey)) }
