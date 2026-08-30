package auth

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"math"
	"strings"
)

const (
	Build                uint16 = 12340
	VersionMajor         uint8  = 3
	VersionMinor         uint8  = 3
	VersionPatch         uint8  = 5
	logonChallengeOpcode uint8  = 0x00
	logonProofOpcode     uint8  = 0x01
	realmListOpcode      uint8  = 0x10
)

type RefusedError struct {
	Code   uint8
	Reason string
}

func (e *RefusedError) Error() string {
	return fmt.Sprintf("auth refused with code %#02x: %s", e.Code, e.Reason)
}

type AuthResult uint8

const (
	AuthSuccess AuthResult = iota
	AuthUnknownAccount
	AuthAccountBanned
	AuthAccountSuspended
	AuthAlreadyOnline
	AuthVersionInvalid
	AuthVersionUpdate
	AuthParentalControl
)

func resultForCode(code uint8) AuthResult {
	switch code {
	case 0x00:
		return AuthSuccess
	case 0x04, 0x05:
		return AuthUnknownAccount
	case 0x03:
		return AuthAccountBanned
	case 0x0C:
		return AuthAccountSuspended
	case 0x06:
		return AuthAlreadyOnline
	case 0x09:
		return AuthVersionInvalid
	case 0x0A:
		return AuthVersionUpdate
	case 0x0F:
		return AuthParentalControl
	default:
		return AuthResult(code)
	}
}

func describeResult(result AuthResult) string {
	switch result {
	case AuthSuccess:
		return "success"
	case AuthUnknownAccount:
		return "unknown account or wrong password"
	case AuthAccountBanned:
		return "account banned"
	case AuthAccountSuspended:
		return "account suspended"
	case AuthAlreadyOnline:
		return "account already online"
	case AuthVersionInvalid:
		return "client build rejected"
	case AuthVersionUpdate:
		return "client build needs updating"
	case AuthParentalControl:
		return "blocked by parental controls"
	default:
		return fmt.Sprintf("refused with code %#02x", uint8(result))
	}
}

type Challenge struct {
	BPublic       [KeyLen]byte
	Salt          [KeyLen]byte
	SecurityFlags uint8
}

func BuildLogonChallenge(username, locale string) ([]byte, error) {
	user := []byte(strings.ToUpper(username))
	if len(user) > 255 {
		return nil, fmt.Errorf("account name is too long: %d bytes", len(user))
	}
	localeBytes := make([]byte, 4)
	for i := range localeBytes {
		localeBytes[i] = ' '
	}
	copy(localeBytes, []byte(locale))
	for i, j := 0, len(localeBytes)-1; i < j; i, j = i+1, j-1 {
		localeBytes[i], localeBytes[j] = localeBytes[j], localeBytes[i]
	}
	body := make([]byte, 0, 30+len(user))
	body = append(body, 'W', 'o', 'W', 0, VersionMajor, VersionMinor, VersionPatch)
	var word [4]byte
	binary.LittleEndian.PutUint16(word[:2], Build)
	body = append(body, word[:2]...)
	body = append(body, '6', '8', 'x', 0, 'n', 'i', 'W', 0)
	body = append(body, localeBytes...)
	body = append(body, 0, 0, 0, 0, 0, 0, 0, 0, byte(len(user)))
	body = append(body, user...)
	packet := make([]byte, 4+len(body))
	packet[0] = logonChallengeOpcode
	packet[1] = 8
	binary.LittleEndian.PutUint16(packet[2:4], uint16(len(body)))
	copy(packet[4:], body)
	return packet, nil
}

func ParseChallenge(data []byte) (Challenge, error) {
	if len(data) < 3 {
		return Challenge{}, fmt.Errorf("challenge header truncated: got %d bytes", len(data))
	}
	if data[0] != logonChallengeOpcode {
		return Challenge{}, fmt.Errorf("unexpected challenge opcode %#02x", data[0])
	}
	if result := resultForCode(data[2]); result != AuthSuccess {
		return Challenge{}, &RefusedError{Code: data[2], Reason: describeResult(result)}
	}
	at := 3
	if err := require(data, at, KeyLen, "challenge B"); err != nil {
		return Challenge{}, err
	}
	var bPublic [KeyLen]byte
	copy(bPublic[:], data[at:at+KeyLen])
	at += KeyLen
	if err := require(data, at, 1, "challenge generator length"); err != nil {
		return Challenge{}, err
	}
	gLen := int(data[at])
	at++
	if err := require(data, at, gLen+1, "challenge generator"); err != nil {
		return Challenge{}, err
	}
	at += gLen
	nLen := int(data[at])
	at++
	if nLen != KeyLen {
		return Challenge{}, fmt.Errorf("server sent a %d-byte prime; this client only supports %d", nLen, KeyLen)
	}
	if err := require(data, at, nLen+KeyLen+16+1, "challenge salt"); err != nil {
		return Challenge{}, err
	}
	at += nLen
	var salt [KeyLen]byte
	copy(salt[:], data[at:at+KeyLen])
	at += KeyLen + 16
	return Challenge{BPublic: bPublic, Salt: salt, SecurityFlags: data[at]}, nil
}

func BuildLogonProof(aPublic [KeyLen]byte, m1 [sha1DigestLen]byte) []byte {
	packet := make([]byte, 1+KeyLen+sha1DigestLen+20+2)
	packet[0] = logonProofOpcode
	copy(packet[1:], aPublic[:])
	copy(packet[1+KeyLen:], m1[:])
	return packet
}

func ParseProof(data []byte) ([sha1DigestLen]byte, error) {
	var m2 [sha1DigestLen]byte
	if len(data) < 2 {
		return m2, fmt.Errorf("proof header truncated: got %d bytes", len(data))
	}
	if data[0] != logonProofOpcode {
		return m2, fmt.Errorf("unexpected proof opcode %#02x", data[0])
	}
	if result := resultForCode(data[1]); result != AuthSuccess {
		return m2, &RefusedError{Code: data[1], Reason: describeResult(result)}
	}
	if len(data) < 2+sha1DigestLen {
		return m2, fmt.Errorf("proof M2 truncated: got %d bytes", len(data))
	}
	copy(m2[:], data[2:2+sha1DigestLen])
	return m2, nil
}

func BuildRealmListRequest() []byte { return []byte{realmListOpcode, 0, 0, 0, 0} }

type Realm struct {
	Name       string
	Address    string
	Kind       uint8
	Locked     bool
	Flags      uint8
	Population float32
	Characters uint8
	Timezone   uint8
	ID         uint8
}

func (r Realm) IsOffline() bool { return r.Flags&0x02 != 0 }

func ParseRealmList(data []byte) ([]Realm, error) {
	if len(data) < 9 {
		return nil, fmt.Errorf("realm list header truncated: got %d bytes", len(data))
	}
	if data[0] != realmListOpcode {
		return nil, fmt.Errorf("unexpected realm-list opcode %#02x", data[0])
	}
	count := int(binary.LittleEndian.Uint16(data[7:9]))
	at := 9
	realms := make([]Realm, 0, count)
	for i := 0; i < count; i++ {
		if err := require(data, at, 3, "realm entry"); err != nil {
			return nil, err
		}
		kind := data[at]
		locked := data[at+1] != 0
		flags := data[at+2]
		at += 3
		name, next, err := readCString(data, at, "realm name")
		if err != nil {
			return nil, err
		}
		at = next
		address, next, err := readCString(data, at, "realm address")
		if err != nil {
			return nil, err
		}
		at = next
		if err := require(data, at, 7, "realm metadata"); err != nil {
			return nil, err
		}
		population := math.Float32frombits(binary.LittleEndian.Uint32(data[at : at+4]))
		characters, timezone, id := data[at+4], data[at+5], data[at+6]
		at += 7
		if flags&0x04 != 0 {
			if err := require(data, at, 5, "realm version"); err != nil {
				return nil, err
			}
			at += 5
		}
		realms = append(realms, Realm{Name: name, Address: address, Kind: kind, Locked: locked, Flags: flags, Population: population, Characters: characters, Timezone: timezone, ID: id})
	}
	return realms, nil
}

func require(data []byte, at, count int, what string) error {
	if at < 0 || count < 0 || at+count > len(data) {
		return fmt.Errorf("%s truncated: need %d bytes at offset %d, packet has %d", what, count, at, len(data))
	}
	return nil
}

func readCString(data []byte, at int, what string) (string, int, error) {
	if at < 0 || at > len(data) {
		return "", at, fmt.Errorf("%s truncated", what)
	}
	end := bytes.IndexByte(data[at:], 0)
	if end < 0 {
		return "", at, fmt.Errorf("%s is not terminated", what)
	}
	end += at
	return string(data[at:end]), end + 1, nil
}
