package world

import (
	"bytes"
	"compress/zlib"
	"crypto/sha1"
	"encoding/binary"
	"fmt"
	"math"
	"strings"

	"moreno.warcraft/auth"
)

const (
	Build                uint32 = 12340
	ServerHeaderLen             = 4
	ServerHeaderLargeLen        = 5
	ClientHeaderLen             = 6
	MaxPacket                   = 1 << 20
)

type ClientOpcode uint32

const (
	CharEnum         ClientOpcode = 0x0037
	AuthSession      ClientOpcode = 0x01ED
	TimeSyncResponse ClientOpcode = 0x0391
)

const (
	CharEnumResponse      uint16 = 0x003B
	AuthChallengeResponse uint16 = 0x01EC
	AuthResponsePacket    uint16 = 0x01EE
	TimeSyncRequest       uint16 = 0x0390
	CharacterLoginFailed  uint16 = 0x0041
	LoginVerifyWorld      uint16 = 0x0236
)

type ServerHeader struct {
	Opcode  uint16
	BodyLen int
}

func BuildClientPacket(opcode ClientOpcode, body []byte) ([]byte, error) {
	size := len(body) + 4
	if size > 0xFFFF {
		return nil, fmt.Errorf("client packet is too large: %d bytes", size)
	}
	packet := make([]byte, ClientHeaderLen+len(body))
	binary.BigEndian.PutUint16(packet[:2], uint16(size))
	binary.LittleEndian.PutUint32(packet[2:6], uint32(opcode))
	copy(packet[6:], body)
	return packet, nil
}

func ServerHeaderLenForFirstByte(first byte) int {
	if first&0x80 != 0 {
		return ServerHeaderLargeLen
	}
	return ServerHeaderLen
}

func ParseServerHeader(header []byte) (ServerHeader, error) {
	need := ServerHeaderLenForFirstByte(header[0])
	if len(header) < need {
		return ServerHeader{}, fmt.Errorf("server header truncated: need %d bytes, got %d", need, len(header))
	}
	var size int
	var opcodeAt int
	if header[0]&0x80 != 0 {
		size = int(header[0]&0x7F)<<16 | int(header[1])<<8 | int(header[2])
		opcodeAt = 3
	} else {
		size = int(header[0])<<8 | int(header[1])
		opcodeAt = 2
	}
	if size < 2 {
		return ServerHeader{}, fmt.Errorf("server header is undersized: %d", size)
	}
	bodyLen := size - 2
	if bodyLen > MaxPacket {
		return ServerHeader{}, fmt.Errorf("server packet is too large: %d bytes", bodyLen)
	}
	return ServerHeader{Opcode: binary.LittleEndian.Uint16(header[opcodeAt : opcodeAt+2]), BodyLen: bodyLen}, nil
}

type Reader struct {
	data []byte
	at   int
	what string
}

func NewReader(data []byte, what string) *Reader { return &Reader{data: data, what: what} }

func (r *Reader) Take(count int) ([]byte, error) {
	if count < 0 || r.at+count > len(r.data) {
		return nil, fmt.Errorf("%s truncated at %d: need %d bytes, got %d", r.what, r.at, count, len(r.data))
	}
	data := r.data[r.at : r.at+count]
	r.at += count
	return data, nil
}

func (r *Reader) U8() (uint8, error) {
	data, err := r.Take(1)
	if err != nil {
		return 0, err
	}
	return data[0], nil
}

func (r *Reader) U16() (uint16, error) {
	data, err := r.Take(2)
	if err != nil {
		return 0, err
	}
	return binary.LittleEndian.Uint16(data), nil
}

func (r *Reader) U32() (uint32, error) {
	data, err := r.Take(4)
	if err != nil {
		return 0, err
	}
	return binary.LittleEndian.Uint32(data), nil
}

func (r *Reader) U64() (uint64, error) {
	data, err := r.Take(8)
	if err != nil {
		return 0, err
	}
	return binary.LittleEndian.Uint64(data), nil
}

func (r *Reader) F32() (float32, error) {
	data, err := r.Take(4)
	if err != nil {
		return 0, err
	}
	return math.Float32frombits(binary.LittleEndian.Uint32(data)), nil
}

func (r *Reader) Bytes(count int) ([]byte, error) { return r.Take(count) }

func (r *Reader) CString() (string, error) {
	remaining := r.data[r.at:]
	end := bytes.IndexByte(remaining, 0)
	if end < 0 {
		return "", fmt.Errorf("%s string is not terminated", r.what)
	}
	text := string(remaining[:end])
	r.at += end + 1
	return text, nil
}

func (r *Reader) Skip(count int) error {
	_, err := r.Take(count)
	return err
}

func (r *Reader) Finish() error {
	if r.at != len(r.data) {
		return fmt.Errorf("%s has %d trailing bytes", r.what, len(r.data)-r.at)
	}
	return nil
}

type AuthChallenge struct{ ServerSeed [4]byte }

func ParseAuthChallenge(body []byte) (AuthChallenge, error) {
	r := NewReader(body, "SMSG_AUTH_CHALLENGE")
	if err := r.Skip(4); err != nil {
		return AuthChallenge{}, err
	}
	seedBytes, err := r.Bytes(4)
	if err != nil {
		return AuthChallenge{}, err
	}
	var seed [4]byte
	copy(seed[:], seedBytes)
	if err := r.Skip(32); err != nil {
		return AuthChallenge{}, err
	}
	if err := r.Finish(); err != nil {
		return AuthChallenge{}, err
	}
	return AuthChallenge{ServerSeed: seed}, nil
}

func AuthDigest(account string, clientSeed, serverSeed [4]byte, sessionKey [auth.SessionKeyLen]byte) [20]byte {
	hash := sha1.New()
	_, _ = hash.Write([]byte(strings.ToUpper(account)))
	_, _ = hash.Write(make([]byte, 4))
	_, _ = hash.Write(clientSeed[:])
	_, _ = hash.Write(serverSeed[:])
	_, _ = hash.Write(sessionKey[:])
	var digest [20]byte
	copy(digest[:], hash.Sum(nil))
	return digest
}

func BuildAuthSession(account string, realmID uint32, clientSeed, serverSeed [4]byte, sessionKey [auth.SessionKeyLen]byte) ([]byte, error) {
	account = strings.ToUpper(account)
	digest := AuthDigest(account, clientSeed, serverSeed, sessionKey)
	addon, err := emptyAddonBlock()
	if err != nil {
		return nil, err
	}
	body := make([]byte, 0, 96)
	var word [8]byte
	binary.LittleEndian.PutUint32(word[:4], Build)
	body = append(body, word[:4]...)
	body = append(body, 0, 0, 0, 0)
	body = append(body, []byte(account)...)
	body = append(body, 0)
	body = append(body, 0, 0, 0, 0)
	body = append(body, clientSeed[:]...)
	body = append(body, 0, 0, 0, 0)
	body = append(body, 0, 0, 0, 0)
	binary.LittleEndian.PutUint32(word[:4], realmID)
	body = append(body, word[:4]...)
	body = append(body, 0, 0, 0, 0, 0, 0, 0, 0)
	body = append(body, digest[:]...)
	body = append(body, addon...)
	return body, nil
}

func emptyAddonBlock() ([]byte, error) {
	manifest := make([]byte, 8)
	var compressed bytes.Buffer
	writer := zlib.NewWriter(&compressed)
	if _, err := writer.Write(manifest); err != nil {
		return nil, err
	}
	if err := writer.Close(); err != nil {
		return nil, err
	}
	block := make([]byte, 4, 4+compressed.Len())
	binary.LittleEndian.PutUint32(block, uint32(len(manifest)))
	block = append(block, compressed.Bytes()...)
	return block, nil
}

type AuthResponseKind uint8

const (
	AuthOK AuthResponseKind = iota
	AuthQueued
	AuthRefused
)

type AuthResponse struct {
	Kind          AuthResponseKind
	Expansion     uint8
	QueuePosition uint32
	Code          uint8
}

func ParseAuthResponse(body []byte) (AuthResponse, error) {
	r := NewReader(body, "SMSG_AUTH_RESPONSE")
	code, err := r.U8()
	if err != nil {
		return AuthResponse{}, err
	}
	switch code {
	case 0x0C:
		if err := r.Skip(4); err != nil {
			return AuthResponse{}, err
		}
		if err := r.Skip(1); err != nil {
			return AuthResponse{}, err
		}
		if err := r.Skip(4); err != nil {
			return AuthResponse{}, err
		}
		expansion, err := r.U8()
		if err != nil {
			return AuthResponse{}, err
		}
		if err := r.Finish(); err != nil {
			return AuthResponse{}, err
		}
		return AuthResponse{Kind: AuthOK, Expansion: expansion, Code: code}, nil
	case 0x1B:
		position, err := r.U32()
		if err != nil {
			return AuthResponse{}, err
		}
		if err := r.Skip(1); err != nil {
			return AuthResponse{}, err
		}
		if err := r.Finish(); err != nil {
			return AuthResponse{}, err
		}
		return AuthResponse{Kind: AuthQueued, QueuePosition: position, Code: code}, nil
	default:
		return AuthResponse{Kind: AuthRefused, Code: code}, nil
	}
}

type Equipment struct {
	DisplayID     uint32
	InventoryType uint8
	EnchantAura   uint32
}

const EquipmentSlots = 23

type Character struct {
	GUID           uint64
	Name           string
	Race           uint8
	Class          uint8
	Gender         uint8
	Skin           uint8
	Face           uint8
	HairStyle      uint8
	HairColor      uint8
	FacialHair     uint8
	Level          uint8
	Zone           uint32
	Map            uint32
	Position       [3]float32
	GuildID        uint32
	Flags          uint32
	CustomizeFlags uint32
	FirstLogin     bool
	PetDisplayID   uint32
	PetLevel       uint32
	PetFamily      uint32
	Equipment      [EquipmentSlots]Equipment
}

func (c Character) NeedsRename() bool { return c.Flags&0x4000 != 0 }

func ParseCharEnum(body []byte) ([]Character, error) {
	r := NewReader(body, "SMSG_CHAR_ENUM")
	count, err := r.U8()
	if err != nil {
		return nil, err
	}
	characters := make([]Character, 0, int(count))
	for i := 0; i < int(count); i++ {
		var character Character
		if character.GUID, err = r.U64(); err != nil {
			return nil, err
		}
		if character.Name, err = r.CString(); err != nil {
			return nil, err
		}
		fields := []*uint8{&character.Race, &character.Class, &character.Gender, &character.Skin, &character.Face, &character.HairStyle, &character.HairColor, &character.FacialHair, &character.Level}
		for _, field := range fields {
			if *field, err = r.U8(); err != nil {
				return nil, err
			}
		}
		if character.Zone, err = r.U32(); err != nil {
			return nil, err
		}
		if character.Map, err = r.U32(); err != nil {
			return nil, err
		}
		for i := range character.Position {
			if character.Position[i], err = r.F32(); err != nil {
				return nil, err
			}
		}
		if character.GuildID, err = r.U32(); err != nil {
			return nil, err
		}
		if character.Flags, err = r.U32(); err != nil {
			return nil, err
		}
		if character.CustomizeFlags, err = r.U32(); err != nil {
			return nil, err
		}
		firstLogin, err := r.U8()
		if err != nil {
			return nil, err
		}
		character.FirstLogin = firstLogin != 0
		if character.PetDisplayID, err = r.U32(); err != nil {
			return nil, err
		}
		if character.PetLevel, err = r.U32(); err != nil {
			return nil, err
		}
		if character.PetFamily, err = r.U32(); err != nil {
			return nil, err
		}
		for slot := range character.Equipment {
			if character.Equipment[slot].DisplayID, err = r.U32(); err != nil {
				return nil, err
			}
			if character.Equipment[slot].InventoryType, err = r.U8(); err != nil {
				return nil, err
			}
			if character.Equipment[slot].EnchantAura, err = r.U32(); err != nil {
				return nil, err
			}
		}
		characters = append(characters, character)
	}
	if err := r.Finish(); err != nil {
		return nil, err
	}
	return characters, nil
}

func ParseResultCode(body []byte, what string) (uint8, error) {
	r := NewReader(body, what)
	code, err := r.U8()
	if err != nil {
		return 0, err
	}
	return code, nil
}

func ParseTimeSyncRequest(body []byte) (uint32, error) {
	r := NewReader(body, "SMSG_TIME_SYNC_REQ")
	counter, err := r.U32()
	if err != nil {
		return 0, err
	}
	if err := r.Finish(); err != nil {
		return 0, err
	}
	return counter, nil
}

func BuildTimeSyncResponse(counter, ticks uint32) []byte {
	body := make([]byte, 8)
	binary.LittleEndian.PutUint32(body[:4], counter)
	binary.LittleEndian.PutUint32(body[4:], ticks)
	return body
}
