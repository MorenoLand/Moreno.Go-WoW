package world

import (
	"testing"

	"github.com/MorenoLand/Moreno.WoW/auth"
)

func TestHeaderCryptRoundTrip(t *testing.T) {
	var key [auth.SessionKeyLen]byte
	for i := range key {
		key[i] = byte(i*13 + 5)
	}
	client := NewHeaderCrypt(key)
	server := &HeaderCrypt{incoming: initCipher(clientToServerSeed, key), outgoing: initCipher(serverToClientSeed, key)}
	plain := []byte{0, 12, 0xEC, 1, 0x2A, 0x2A}
	wire := append([]byte(nil), plain...)
	client.Encrypt(wire)
	if string(wire) == string(plain) {
		t.Fatal("encryption was a no-op")
	}
	server.Decrypt(wire)
	if string(wire) != string(plain) {
		t.Fatalf("round trip failed: %x", wire)
	}
}
