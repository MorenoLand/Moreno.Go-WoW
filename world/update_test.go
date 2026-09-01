package world

import (
	"bytes"
	"compress/zlib"
	"encoding/binary"
	"math"
	"testing"
)

func TestParseUpdateObjectCreate(t *testing.T) {
	body := make([]byte, 4)
	binary.LittleEndian.PutUint32(body, 1)
	body = append(body, byte(UpdateCreate2), 0x03, 0x34, 0x12, byte(ObjectTypeUnit))
	var flags [2]byte
	binary.LittleEndian.PutUint16(flags[:], UpdateFlagStationaryPosition)
	body = append(body, flags[:]...)
	for _, value := range []float32{1, 2, 3, 0.5} {
		var raw [4]byte
		binary.LittleEndian.PutUint32(raw[:], mathFloat32bits(value))
		body = append(body, raw[:]...)
	}
	body = append(body, 3)
	for _, value := range []uint32{16, 0, 8, 1, 0x12345678} {
		var raw [4]byte
		binary.LittleEndian.PutUint32(raw[:], value)
		body = append(body, raw[:]...)
	}
	blocks, err := ParseUpdateObject(body)
	if err != nil {
		t.Fatal(err)
	}
	if len(blocks) != 1 || blocks[0].GUID != 0x1234 || blocks[0].ObjectType != ObjectTypeUnit || !blocks[0].Spawned {
		t.Fatalf("blocks=%+v", blocks)
	}
	if blocks[0].Fields[4] != 1 || blocks[0].Fields[67] != 0x12345678 || !blocks[0].Movement.HasPosition || blocks[0].Movement.Position.X != 1 {
		t.Fatalf("block=%+v", blocks[0])
	}
}

func TestParseCompressedUpdateObject(t *testing.T) {
	plain := make([]byte, 4)
	var compressed bytes.Buffer
	writer := zlib.NewWriter(&compressed)
	_, _ = writer.Write(plain)
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	body := make([]byte, 4)
	binary.LittleEndian.PutUint32(body, uint32(len(plain)))
	body = append(body, compressed.Bytes()...)
	blocks, err := ParseCompressedUpdateObject(body)
	if err != nil {
		t.Fatal(err)
	}
	if len(blocks) != 0 {
		t.Fatalf("blocks=%+v", blocks)
	}
}

func TestParseDestroyObject(t *testing.T) {
	body := make([]byte, 9)
	binary.LittleEndian.PutUint64(body, 0x1020304050607080)
	guid, err := ParseDestroyObject(body)
	if err != nil || guid != 0x1020304050607080 {
		t.Fatalf("guid=%x err=%v", guid, err)
	}
}

func TestParseMonsterMove(t *testing.T) {
	body := []byte{0x03, 0x34, 0x12, 0}
	for _, value := range []float32{1, 2, 3} {
		var raw [4]byte
		binary.LittleEndian.PutUint32(raw[:], mathFloat32bits(value))
		body = append(body, raw[:]...)
	}
	body = append(body, 0, 0, 0, 0, 0)
	body = append(body, 0, 0, 0, 0)
	var raw [4]byte
	binary.LittleEndian.PutUint32(raw[:], 1000)
	body = append(body, raw[:]...)
	binary.LittleEndian.PutUint32(raw[:], 1)
	body = append(body, raw[:]...)
	for _, value := range []float32{5, 6, 7} {
		binary.LittleEndian.PutUint32(raw[:], mathFloat32bits(value))
		body = append(body, raw[:]...)
	}
	move, err := ParseMonsterMove(body)
	if err != nil {
		t.Fatal(err)
	}
	if move.GUID != 0x1234 || move.Duration != 1000 || move.From.X != 1 || move.To != (WorldPosition{X: 5, Y: 6, Z: 7}) {
		t.Fatalf("move=%+v", move)
	}
}

func mathFloat32bits(value float32) uint32 {
	return math.Float32bits(value)
}
