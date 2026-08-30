package world

import (
	"encoding/binary"
	"math"
	"testing"

	"moreno.warcraft/auth"
)

func TestClientPacketHeader(t *testing.T) {
	packet, err := BuildClientPacket(CharEnum, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(packet) != ClientHeaderLen {
		t.Fatalf("unexpected packet length: %d", len(packet))
	}
	if binary.BigEndian.Uint16(packet[:2]) != 4 {
		t.Fatalf("unexpected packet size: %x", packet[:2])
	}
	if binary.LittleEndian.Uint32(packet[2:]) != uint32(CharEnum) {
		t.Fatalf("unexpected opcode: %x", packet[2:])
	}
}

func TestServerHeaderForms(t *testing.T) {
	normal := []byte{0, 6, byte(CharEnumResponse), byte(CharEnumResponse >> 8)}
	parsed, err := ParseServerHeader(normal)
	if err != nil {
		t.Fatal(err)
	}
	if parsed.Opcode != CharEnumResponse || parsed.BodyLen != 4 {
		t.Fatalf("unexpected normal header: %+v", parsed)
	}
	large := []byte{0x80, 0, 4, byte(CharEnumResponse), byte(CharEnumResponse >> 8)}
	parsed, err = ParseServerHeader(large)
	if err != nil {
		t.Fatal(err)
	}
	if parsed.Opcode != CharEnumResponse || parsed.BodyLen != 2 {
		t.Fatalf("unexpected large header: %+v", parsed)
	}
}

func TestCharacterListParser(t *testing.T) {
	body := make([]byte, 0, 256)
	body = append(body, 1)
	var word8 [8]byte
	binary.LittleEndian.PutUint64(word8[:], 0x0102030405060708)
	body = append(body, word8[:]...)
	body = append(body, []byte("Tester\x00")...)
	body = append(body, 1, 1, 0, 0, 0, 0, 0, 0, 10)
	var word4 [4]byte
	binary.LittleEndian.PutUint32(word4[:], 12)
	body = append(body, word4[:]...)
	binary.LittleEndian.PutUint32(word4[:], 0)
	body = append(body, word4[:]...)
	for _, value := range []float32{1, 2, 3} {
		binary.LittleEndian.PutUint32(word4[:], math.Float32bits(value))
		body = append(body, word4[:]...)
	}
	for i := 0; i < 3; i++ {
		body = append(body, 0, 0, 0, 0)
	}
	body = append(body, 0)
	body = append(body, make([]byte, 12)...)
	for i := 0; i < EquipmentSlots; i++ {
		body = append(body, 0, 0, 0, 0, 0, 0, 0, 0, 0)
	}
	characters, err := ParseCharEnum(body)
	if err != nil {
		t.Fatal(err)
	}
	if len(characters) != 1 {
		t.Fatalf("unexpected character count: %d", len(characters))
	}
	character := characters[0]
	if character.Name != "Tester" || character.GUID != 0x0102030405060708 || character.Level != 10 || character.Zone != 12 {
		t.Fatalf("unexpected character: %+v", character)
	}
	if character.Position != [3]float32{1, 2, 3} {
		t.Fatalf("unexpected position: %v", character.Position)
	}
}

func TestAuthSessionContainsExpectedDigest(t *testing.T) {
	var key [auth.SessionKeyLen]byte
	for i := range key {
		key[i] = byte(i)
	}
	body, err := BuildAuthSession("tester", 7, [4]byte{1, 2, 3, 4}, [4]byte{5, 6, 7, 8}, key)
	if err != nil {
		t.Fatal(err)
	}
	if binary.LittleEndian.Uint32(body[:4]) != Build {
		t.Fatalf("unexpected build: %x", body[:4])
	}
	digest := AuthDigest("tester", [4]byte{1, 2, 3, 4}, [4]byte{5, 6, 7, 8}, key)
	if !contains(body, digest[:]) {
		t.Fatal("auth digest not present")
	}
}

func contains(data, target []byte) bool {
	for i := 0; i+len(target) <= len(data); i++ {
		match := true
		for j := range target {
			if data[i+j] != target[j] {
				match = false
				break
			}
		}
		if match {
			return true
		}
	}
	return false
}
