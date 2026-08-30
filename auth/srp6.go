package auth

import (
	"crypto/rand"
	"crypto/sha1"
	"errors"
	"math/big"
	"strings"
)

const (
	KeyLen        = 32
	SessionKeyLen = 40
	privateKeyLen = 19
	sha1DigestLen = 20
)

var (
	invalidServerKey = errors.New("server sent B = 0")
	nLE              = [KeyLen]byte{0xB7, 0x9B, 0x3E, 0x2A, 0x87, 0x82, 0x3C, 0xAB, 0x8F, 0x5E, 0xBF, 0xBF, 0x8E, 0xB1, 0x01, 0x08, 0x53, 0x50, 0x06, 0x29, 0x8B, 0x5B, 0xAD, 0xBD, 0x5B, 0x53, 0xE1, 0x89, 0x5E, 0x64, 0x4B, 0x89}
)

const (
	generator  uint8 = 7
	multiplier uint8 = 3
)

type Session struct {
	APublic    [KeyLen]byte
	M1         [sha1DigestLen]byte
	Key        [SessionKeyLen]byte
	expectedM2 [sha1DigestLen]byte
}

func (s Session) VerifyServer(m2 [sha1DigestLen]byte) bool { return s.expectedM2 == m2 }

func RandomPrivate() ([privateKeyLen]byte, error) {
	var private [privateKeyLen]byte
	_, err := rand.Read(private[:])
	return private, err
}

func Respond(username, password string, salt, bPublic [KeyLen]byte, private []byte) (Session, error) {
	n := fromLittleEndian(nLE[:])
	g := new(big.Int).SetUint64(uint64(generator))
	b := fromLittleEndian(bPublic[:])
	if new(big.Int).Mod(new(big.Int).Set(b), n).Sign() == 0 {
		return Session{}, invalidServerKey
	}
	a := fromLittleEndian(private)
	aPublicInt := new(big.Int).Exp(g, a, n)
	aPublicBytes := toLittleEndian(aPublicInt, KeyLen)
	var aPublic [KeyLen]byte
	copy(aPublic[:], aPublicBytes)
	uHash := sha1Parts(aPublic[:], bPublic[:])
	u := fromLittleEndian(uHash[:])
	x := DeriveX(username, password, salt)
	gX := new(big.Int).Exp(g, x, n)
	kgX := new(big.Int).Mul(new(big.Int).SetUint64(uint64(multiplier)), gX)
	kgX.Mod(kgX, n)
	base := new(big.Int).Sub(b, kgX)
	base.Mod(base, n)
	exponent := new(big.Int).Mul(u, x)
	exponent.Add(exponent, a)
	s := new(big.Int).Exp(base, exponent, n)
	var sLittle [KeyLen]byte
	copy(sLittle[:], toLittleEndian(s, KeyLen))
	key := interleave(sLittle)
	hashN := sha1Parts(nLE[:])
	hashG := sha1Parts([]byte{generator})
	var nXorG [sha1DigestLen]byte
	for i := range nXorG {
		nXorG[i] = hashN[i] ^ hashG[i]
	}
	hashUser := sha1Parts([]byte(strings.ToUpper(username)))
	m1 := sha1Parts(nXorG[:], hashUser[:], salt[:], aPublic[:], bPublic[:], key[:])
	expectedM2 := sha1Parts(aPublic[:], m1[:], key[:])
	return Session{APublic: aPublic, M1: m1, Key: key, expectedM2: expectedM2}, nil
}

func DeriveX(username, password string, salt [KeyLen]byte) *big.Int {
	identity := strings.ToUpper(username) + ":" + strings.ToUpper(password)
	inner := sha1Parts([]byte(identity))
	digest := sha1Parts(salt[:], inner[:])
	return fromLittleEndian(digest[:])
}

func Verifier(username, password string, salt [KeyLen]byte) *big.Int {
	n := fromLittleEndian(nLE[:])
	g := new(big.Int).SetUint64(uint64(generator))
	return new(big.Int).Exp(g, DeriveX(username, password, salt), n)
}

func interleave(s [KeyLen]byte) [SessionKeyLen]byte {
	skip := 0
	for skip < len(s) && s[skip] == 0 {
		skip++
	}
	if skip%2 == 1 {
		skip++
	}
	offset := skip / 2
	even := make([]byte, KeyLen/2)
	odd := make([]byte, KeyLen/2)
	for i := 0; i < KeyLen/2; i++ {
		even[i] = s[i*2]
		odd[i] = s[i*2+1]
	}
	hashEven := sha1Parts(even[offset:])
	hashOdd := sha1Parts(odd[offset:])
	var key [SessionKeyLen]byte
	for i := 0; i < sha1DigestLen; i++ {
		key[i*2] = hashEven[i]
		key[i*2+1] = hashOdd[i]
	}
	return key
}

func fromLittleEndian(data []byte) *big.Int {
	bigEndian := make([]byte, len(data))
	for i, b := range data {
		bigEndian[len(data)-1-i] = b
	}
	return new(big.Int).SetBytes(bigEndian)
}

func toLittleEndian(value *big.Int, length int) []byte {
	bigEndian := value.Bytes()
	littleEndian := make([]byte, length)
	for i := 0; i < len(bigEndian) && i < length; i++ {
		littleEndian[i] = bigEndian[len(bigEndian)-1-i]
	}
	return littleEndian
}

func sha1Parts(parts ...[]byte) [sha1DigestLen]byte {
	hash := sha1.New()
	for _, part := range parts {
		_, _ = hash.Write(part)
	}
	var result [sha1DigestLen]byte
	copy(result[:], hash.Sum(nil))
	return result
}
