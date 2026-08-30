package world

import (
	"crypto/hmac"
	"crypto/rc4"
	"crypto/sha1"

	"moreno.warcraft/auth"
)

var serverToClientSeed = [16]byte{0xCC, 0x98, 0xAE, 0x04, 0xE8, 0x97, 0xEA, 0xCA, 0x12, 0xDD, 0xC0, 0x93, 0x42, 0x91, 0x53, 0x57}
var clientToServerSeed = [16]byte{0xC2, 0xB3, 0x72, 0x3C, 0xC6, 0xAE, 0xD9, 0xB5, 0x34, 0x3C, 0x53, 0xEE, 0x2F, 0x43, 0x67, 0xCE}

const keystreamDrop = 1024

type HeaderCrypt struct {
	incoming *rc4.Cipher
	outgoing *rc4.Cipher
}

func NewHeaderCrypt(sessionKey [auth.SessionKeyLen]byte) *HeaderCrypt {
	return &HeaderCrypt{incoming: initCipher(serverToClientSeed, sessionKey), outgoing: initCipher(clientToServerSeed, sessionKey)}
}

func (c *HeaderCrypt) Decrypt(header []byte) { c.incoming.XORKeyStream(header, header) }

func (c *HeaderCrypt) Encrypt(header []byte) { c.outgoing.XORKeyStream(header, header) }

func initCipher(seed [16]byte, sessionKey [auth.SessionKeyLen]byte) *rc4.Cipher {
	hash := hmac.New(sha1.New, seed[:])
	_, _ = hash.Write(sessionKey[:])
	cipher, _ := rc4.NewCipher(hash.Sum(nil))
	discard := make([]byte, keystreamDrop)
	cipher.XORKeyStream(discard, discard)
	return cipher
}
