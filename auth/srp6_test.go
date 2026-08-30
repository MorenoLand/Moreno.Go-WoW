package auth

import (
	"math/big"
	"testing"
)

func TestSRP6ClientMatchesServer(t *testing.T) {
	var salt [KeyLen]byte
	for i := range salt {
		salt[i] = byte(i*7 + 11)
	}
	username, password := "TESTER", "hunter2"
	n := fromLittleEndian(nLE[:])
	bPrivate := fromLittleEndian(make([]byte, privateKeyLen))
	bPrivate.SetUint64(9)
	v := Verifier(username, password, salt)
	g := new(big.Int).SetUint64(uint64(generator))
	bPublicInt := new(big.Int).Mul(new(big.Int).SetUint64(uint64(multiplier)), v)
	bPublicInt.Add(bPublicInt, new(big.Int).Exp(g, bPrivate, n))
	bPublicInt.Mod(bPublicInt, n)
	var bPublic [KeyLen]byte
	copy(bPublic[:], toLittleEndian(bPublicInt, KeyLen))
	private := []byte{4}
	session, err := Respond(username, password, salt, bPublic, private)
	if err != nil {
		t.Fatal(err)
	}
	uHash := sha1Parts(session.APublic[:], bPublic[:])
	u := fromLittleEndian(uHash[:])
	aPublic := fromLittleEndian(session.APublic[:])
	s := new(big.Int).Mul(aPublic, new(big.Int).Exp(v, u, n))
	s.Mod(s, n)
	s.Exp(s, bPrivate, n)
	var serverSecret [KeyLen]byte
	copy(serverSecret[:], toLittleEndian(s, KeyLen))
	if session.Key != interleave(serverSecret) {
		t.Fatal("client and server session keys differ")
	}
	m2 := sha1Parts(session.APublic[:], session.M1[:], session.Key[:])
	if !session.VerifyServer(m2) {
		t.Fatal("client rejected matching server proof")
	}
}
