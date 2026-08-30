package world

import (
	"encoding/binary"
	"fmt"
	"io"
	"math"
	"net"
	"testing"
	"time"

	"moreno.warcraft/auth"
)

func TestConnectionCompletesHandshakeAndReadsCharacters(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	var key [auth.SessionKeyLen]byte
	for i := range key {
		key[i] = byte(i*13 + 5)
	}
	serverErrors := make(chan error, 1)
	go func() {
		connection, err := listener.Accept()
		if err != nil {
			serverErrors <- err
			return
		}
		defer connection.Close()
		serverErrors <- serveWorldHandshake(connection, key)
	}()
	connection, err := Open(listener.Addr().String(), "tester", 7, key, 3*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	characters, err := connection.Characters()
	closeErr := connection.Close()
	if err != nil {
		t.Fatal(err)
	}
	if closeErr != nil {
		t.Fatal(closeErr)
	}
	if len(characters) != 1 || characters[0].Name != "Tester" || characters[0].Level != 10 {
		t.Fatalf("unexpected characters: %+v", characters)
	}
	if err := <-serverErrors; err != nil {
		t.Fatal(err)
	}
}

func serveWorldHandshake(connection net.Conn, key [auth.SessionKeyLen]byte) error {
	challengeBody := append([]byte{1, 0, 0, 0}, []byte{5, 6, 7, 8}...)
	challengeBody = append(challengeBody, make([]byte, 32)...)
	if err := writeServerPacket(connection, nil, AuthChallengeResponse, challengeBody); err != nil {
		return err
	}
	packet, err := readClientPacket(connection, nil)
	if err != nil {
		return err
	}
	if packet.Opcode != uint32(AuthSession) || len(packet.Body) < 4 || binary.LittleEndian.Uint32(packet.Body[:4]) != Build {
		return fmt.Errorf("unexpected auth session: %#v", packet)
	}
	serverCrypt := &HeaderCrypt{incoming: initCipher(clientToServerSeed, key), outgoing: initCipher(serverToClientSeed, key)}
	authResponse := []byte{0x0C, 0, 0, 0, 0, 0, 0, 0, 0, 0, 2}
	if err := writeServerPacket(connection, serverCrypt, AuthResponsePacket, authResponse); err != nil {
		return err
	}
	packet, err = readClientPacket(connection, serverCrypt)
	if err != nil {
		return err
	}
	if packet.Opcode != uint32(CharEnum) {
		return fmt.Errorf("expected CMSG_CHAR_ENUM, got %#x", packet.Opcode)
	}
	return writeServerPacket(connection, serverCrypt, CharEnumResponse, characterBody())
}

func readClientPacket(connection net.Conn, crypt *HeaderCrypt) (struct {
	Opcode uint32
	Body   []byte
}, error) {
	header := make([]byte, ClientHeaderLen)
	if _, err := io.ReadFull(connection, header); err != nil {
		return struct {
			Opcode uint32
			Body   []byte
		}{}, err
	}
	if crypt != nil {
		crypt.Decrypt(header)
	}
	size := int(binary.BigEndian.Uint16(header[:2]))
	if size < 4 {
		return struct {
			Opcode uint32
			Body   []byte
		}{}, fmt.Errorf("invalid client packet size %d", size)
	}
	body := make([]byte, size-4)
	if _, err := io.ReadFull(connection, body); err != nil {
		return struct {
			Opcode uint32
			Body   []byte
		}{}, err
	}
	return struct {
		Opcode uint32
		Body   []byte
	}{Opcode: binary.LittleEndian.Uint32(header[2:]), Body: body}, nil
}

func writeServerPacket(connection net.Conn, crypt *HeaderCrypt, opcode uint16, body []byte) error {
	size := len(body) + 2
	if size > 0x7FFF {
		return fmt.Errorf("test packet too large: %d", size)
	}
	header := []byte{byte(size >> 8), byte(size), byte(opcode), byte(opcode >> 8)}
	if crypt != nil {
		crypt.Encrypt(header)
	}
	if _, err := connection.Write(header); err != nil {
		return err
	}
	_, err := connection.Write(body)
	return err
}

func characterBody() []byte {
	body := []byte{1}
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
	return body
}
