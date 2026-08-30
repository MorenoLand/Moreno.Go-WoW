package auth

import (
	"encoding/binary"
	"testing"
)

func TestLogonChallengeUsesWrathFields(t *testing.T) {
	packet, err := BuildLogonChallenge("Tester", "enUS")
	if err != nil {
		t.Fatal(err)
	}
	if packet[0] != logonChallengeOpcode || packet[1] != 8 {
		t.Fatalf("unexpected header: %x", packet[:2])
	}
	bodyLen := int(binary.LittleEndian.Uint16(packet[2:4]))
	if bodyLen != len(packet)-4 {
		t.Fatalf("body length %d != %d", bodyLen, len(packet)-4)
	}
	body := packet[4:]
	if string(body[:4]) != "WoW\x00" || body[4] != 3 || body[5] != 3 || body[6] != 5 {
		t.Fatalf("unexpected version fields: %x", body[:7])
	}
	if binary.LittleEndian.Uint16(body[7:9]) != Build {
		t.Fatalf("unexpected build: %x", body[7:9])
	}
	if string(body[17:21]) != "SUne" {
		t.Fatalf("unexpected locale: %q", body[17:21])
	}
	if string(body[30:]) != "TESTER" {
		t.Fatalf("unexpected account: %q", body[30:])
	}
}

func TestParseChallenge(t *testing.T) {
	data := []byte{logonChallengeOpcode, 0, 0}
	var bPublic [KeyLen]byte
	for i := range bPublic {
		bPublic[i] = 0xAB
	}
	data = append(data, bPublic[:]...)
	data = append(data, 1, 7, KeyLen)
	data = append(data, nLE[:]...)
	var salt [KeyLen]byte
	for i := range salt {
		salt[i] = 0xCD
	}
	data = append(data, salt[:]...)
	data = append(data, make([]byte, 16)...)
	data = append(data, 0)
	challenge, err := ParseChallenge(data)
	if err != nil {
		t.Fatal(err)
	}
	if challenge.BPublic[0] != 0xAB || challenge.Salt[0] != 0xCD || challenge.SecurityFlags != 0 {
		t.Fatalf("unexpected challenge: %+v", challenge)
	}
}

func TestParseRealmList(t *testing.T) {
	body := make([]byte, 0, 64)
	body = append(body, 0, 0, 0, 0)
	body = append(body, 1, 0)
	body = append(body, 0, 0, 0)
	body = append(body, []byte("Test Realm\x00127.0.0.1:8085\x00")...)
	var population [4]byte
	binary.LittleEndian.PutUint32(population[:], 0x3E000000)
	body = append(body, population[:]...)
	body = append(body, 2, 1, 7)
	body = append(body, 0, 0)
	packet := append([]byte{realmListOpcode, 0, 0}, body...)
	realms, err := ParseRealmList(packet)
	if err != nil {
		t.Fatal(err)
	}
	if len(realms) != 1 || realms[0].Name != "Test Realm" || realms[0].Address != "127.0.0.1:8085" || realms[0].ID != 7 {
		t.Fatalf("unexpected realms: %+v", realms)
	}
}
