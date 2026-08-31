package render

import (
	"encoding/binary"
	"testing"
)

func TestParseAreaNames(t *testing.T) {
	data := make([]byte, 20+12*4+11)
	copy(data, []byte("WDBC"))
	binary.LittleEndian.PutUint32(data[4:8], 1)
	binary.LittleEndian.PutUint32(data[8:12], 12)
	binary.LittleEndian.PutUint32(data[12:16], 12*4)
	binary.LittleEndian.PutUint32(data[16:20], 11)
	base := 20
	binary.LittleEndian.PutUint32(data[base:base+4], 1519)
	binary.LittleEndian.PutUint32(data[base+11*4:base+12*4], 1)
	copy(data[20+12*4:], []byte{0, 'S', 't', 'o', 'r', 'm', 'w', 'i', 'n', 'd', 0})
	names := parseAreaNames(data)
	if names[1519] != "Stormwind" {
		t.Fatalf("area name=%q", names[1519])
	}
}
