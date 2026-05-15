package bankan

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGenerateID_Length(t *testing.T) {
	id := GenerateID()
	assert.Len(t, id, idLength)
}

func TestGenerateID_Charset(t *testing.T) {
	for range 200 {
		id := GenerateID()
		for _, ch := range id {
			assert.Contains(t, idAlphabet, string(ch), "unexpected character %q in id %q", ch, id)
		}
	}
}

func TestGenerateID_Randomness(t *testing.T) {
	seen := make(map[string]struct{})
	const n = 500
	for range n {
		seen[GenerateID()] = struct{}{}
	}
	// With 36^5 = 60_466_176 possibilities, 500 draws must be nearly all unique.
	assert.Greater(t, len(seen), n*9/10, "too many collisions: only %d unique IDs out of %d", len(seen), n)
}

func TestNewUniqueID_AvoidsExisting(t *testing.T) {
	// Pre-fill all but a handful of 2-char IDs by brute force is impractical;
	// instead verify the function returns an ID not in the provided set.
	existing := map[string]struct{}{
		"aaaaa": {},
		"bbbbb": {},
	}
	id := NewUniqueID(existing)
	require.NotEmpty(t, id)
	_, collision := existing[id]
	assert.False(t, collision, "NewUniqueID returned an already-existing id %q", id)
}
