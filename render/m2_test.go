package render

import (
	"math"
	"testing"
)

func TestDecodeM2QuaternionIdentity(t *testing.T) {
	if value := decodeM2Quaternion(0); value != 0 {
		t.Fatalf("zero quaternion component=%f", value)
	}
	if value := decodeM2Quaternion(0x7fff); math.Abs(float64(value-1)) > 0.00001 {
		t.Fatalf("unit quaternion component=%f", value)
	}
	if value := decodeM2Quaternion(0x8000); value >= 0 {
		t.Fatalf("negative quaternion component=%f", value)
	}
}
