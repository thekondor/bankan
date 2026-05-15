package bankan

import (
	"math/rand/v2"
	"strings"
)

const idAlphabet = "abcdefghijklmnopqrstuvwxyz0123456789"
const idLength = 5

// GenerateID returns a new random 5-character lowercase alphanumeric string.
func GenerateID() string {
	var b strings.Builder
	b.Grow(idLength)
	for range idLength {
		b.WriteByte(idAlphabet[rand.IntN(len(idAlphabet))])
	}
	return b.String()
}

// NewUniqueID generates an ID that is not present in the existing set.
func NewUniqueID(existing map[string]struct{}) string {
	for {
		id := GenerateID()
		if _, taken := existing[id]; !taken {
			return id
		}
	}
}
