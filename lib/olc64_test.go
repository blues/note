package notelib

import (
	"math"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestOLCFromINT64BytesRejectsNonPositive(t *testing.T) {
	codes := []int64{0, -1, -8, math.MinInt64, OLC32ToOLC64(0x80000000), OLC32ToOLC64(0xFFFFFFFF)}
	for byteCount := 4; byteCount <= 7; byteCount++ {
		for _, code := range codes {
			assert.NotPanics(t, func() {
				assert.Empty(t, OLCFromINT64Bytes(code, byteCount), "code %d, %d bytes", code, byteCount)
			})
		}
	}
}

func FuzzOLCFromINT64Bytes(f *testing.F) {
	f.Add(int64(0), 4)
	f.Add(int64(-1), 5)
	f.Add(int64(math.MinInt64), 6)
	f.Add(int64(math.MaxInt64), 7)
	f.Add(OLC32ToOLC64(0xFFFFFFFF), 4)
	f.Fuzz(func(t *testing.T, code int64, byteCount int) {
		olc := OLCFromINT64Bytes(code, byteCount)
		if code <= 0 && olc != "" {
			t.Fatalf("code %d: expected empty OLC, got %q", code, olc)
		}
	})
}
