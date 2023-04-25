package table

import (
	"fmt"
	"math/rand"
	"testing"

	"golang.org/x/exp/slices"
)

func TestSharedPrefixLen(t *testing.T) {
	t.Parallel()

	do := func(t *testing.T, keyLen int) {
		rando := rand.New(rand.NewSource(5))

		keys := sharedPrefixLenTestKeys(rando, keyLen)

		lengths := make(map[int]int) // length -> hits

		for i := 0; i < 100_000; i++ {
			a, b := randoTestKey(keys, rando), randoTestKey(keys, rando)
			want := sharedPrefixLenOld(a, b)
			got := sharedPrefixLen(a, b)
			if want != got {
				t.Fatal(want, got)
			}
			lengths[got]++
		}

		if keyLen < 10 {
			return
		}

		// make sure test hits various lengths and repeats

		if len(lengths) < 10 {
			t.Fatal(lengths)
		}

		var withRepeats int
		for _, hits := range lengths {
			if hits > 1 {
				withRepeats++
			}
		}
		if withRepeats < 10 {
			t.Fatal(lengths)
		}
	}

	for _, keyLen := range []int{1, 2, 3, 32, 128} {
		t.Run(fmt.Sprintf("keylen %v", keyLen), func(t *testing.T) { do(t, keyLen) })
	}
}

func BenchmarkSharedPrefixLen(b *testing.B) {
	do := func(b *testing.B, fn func(a, b []byte) int, keyLen int) {
		rando := rand.New(rand.NewSource(5))

		keys := sharedPrefixLenTestKeys(rando, keyLen)
		var pairs [][2][]byte
		for i := 0; i < 1000; i++ {
			k1 := randoTestKey(keys, rando)
			k2 := randoTestKey(keys, rando)
			pairs = append(pairs, [2][]byte{k1, k2})
		}

		b.ResetTimer()

		for i := 0; i < b.N; i++ {
			for _, p := range pairs {
				fn(p[0], p[1])
			}
		}
	}

	for _, keyLen := range []int{2, 16, 128, 256} {
		b.Run(fmt.Sprintf("old keylen %v", keyLen), func(b *testing.B) { do(b, sharedPrefixLenOld, keyLen) })
		b.Run(fmt.Sprintf("new keylen %v", keyLen), func(b *testing.B) { do(b, sharedPrefixLen, keyLen) })
	}
}

func sharedPrefixLenTestKeys(rando *rand.Rand, keyLenMax int) [][]byte {
	buf := make([]byte, keyLenMax)
	randKey := func() []byte {
		length := rando.Intn(keyLenMax)
		if length == 0 {
			length = 1
		}
		buf = buf[:length]
		rando.Read(buf)
		return slices.Clone(buf)
	}

	var keys [][]byte
	for i := 0; i < 100; i++ {
		keys = append(keys, randKey())
	}

	for i := 0; i < 1000; i++ {
		prefixKey := randoTestKey(keys, rando)
		prefixLen := rando.Intn(len(prefixKey))

		key := randKey()

		copy(key, prefixKey[:prefixLen]) // share the prefix

		keys = append(keys, key)
	}

	return keys
}

func randoTestKey(keys [][]byte, rando *rand.Rand) []byte {
	idx := rando.Intn(len(keys))
	return keys[idx]
}
