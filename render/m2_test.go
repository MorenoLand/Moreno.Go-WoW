package render

import (
	"math"
	"testing"
)

func TestDecodeM2QuaternionIdentity(t *testing.T) {
	if value := decodeM2Quaternion(0x7fff); value != 0 {
		t.Fatalf("zero quaternion component=%f", value)
	}
	if value := decodeM2Quaternion(0); math.Abs(float64(value+1)) > 0.00001 {
		t.Fatalf("negative quaternion component=%f", value)
	}
	if value := decodeM2Quaternion(0xffff); math.Abs(float64(value-1)) > 0.00001 {
		t.Fatalf("unit quaternion component=%f", value)
	}
}

func TestPoseM2VertexUsesBoneLookupTable(t *testing.T) {
	model := parsedM2{bones: []m2Bone{{scale: [3]float32{1, 1, 1}}, {translation: [3]float32{3, 0, 0}, scale: [3]float32{1, 1, 1}}}, boneCombos: []uint16{1}}
	skin := parsedSkin{}
	vertex := m2Vertex{weights: [4]uint8{255, 0, 0, 0}, position: [3]float32{0, 0, 0}, normal: [3]float32{0, 1, 0}}
	posed := poseM2Vertex(model, skin, 0, vertex, 0)
	if posed.position[0] != 3 {
		t.Fatalf("bone lookup translation=%v", posed.position)
	}
}
