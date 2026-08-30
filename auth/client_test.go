package auth

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
	"math/big"
	"net"
	"testing"
	"time"
)

func TestLoginCompletesSRP6AndRealmList(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	serverErrors := make(chan error, 1)
	go func() {
		connection, err := listener.Accept()
		if err != nil {
			serverErrors <- err
			return
		}
		defer connection.Close()
		serverErrors <- serveAuthExchange(connection)
	}()
	result, err := Login("127.0.0.1", uint16(listener.Addr().(*net.TCPAddr).Port), "tester", "hunter2", "enUS", 3*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Realms) != 1 || result.Realms[0].Name != "Test Realm" || result.Realms[0].Address != "127.0.0.1:8085" {
		t.Fatalf("unexpected login result: %+v", result)
	}
	if bytes.Equal(result.SessionKey[:], make([]byte, SessionKeyLen)) {
		t.Fatal("session key is empty")
	}
	if err := <-serverErrors; err != nil {
		t.Fatal(err)
	}
}

func serveAuthExchange(connection net.Conn) error {
	header := make([]byte, 4)
	if _, err := io.ReadFull(connection, header); err != nil {
		return err
	}
	challengeSize := int(binary.LittleEndian.Uint16(header[2:]))
	challengeRequest := make([]byte, challengeSize)
	if _, err := io.ReadFull(connection, challengeRequest); err != nil {
		return err
	}
	if header[0] != logonChallengeOpcode || len(challengeRequest) == 0 {
		return fmt.Errorf("invalid challenge request")
	}
	username := "TESTER"
	password := "hunter2"
	var salt [KeyLen]byte
	for i := range salt {
		salt[i] = byte(i*7 + 11)
	}
	n := fromLittleEndian(nLE[:])
	bPrivate := new(big.Int).SetUint64(9)
	v := Verifier(username, password, salt)
	g := new(big.Int).SetUint64(uint64(generator))
	bPublicInt := new(big.Int).Mul(new(big.Int).SetUint64(uint64(multiplier)), v)
	bPublicInt.Add(bPublicInt, new(big.Int).Exp(g, bPrivate, n))
	bPublicInt.Mod(bPublicInt, n)
	var bPublic [KeyLen]byte
	copy(bPublic[:], toLittleEndian(bPublicInt, KeyLen))
	challenge := []byte{logonChallengeOpcode, 0, 0}
	challenge = append(challenge, bPublic[:]...)
	challenge = append(challenge, 1, generator, KeyLen)
	challenge = append(challenge, nLE[:]...)
	challenge = append(challenge, salt[:]...)
	challenge = append(challenge, make([]byte, 16)...)
	challenge = append(challenge, 0)
	if err := writeFull(connection, challenge); err != nil {
		return err
	}
	proof := make([]byte, 1+KeyLen+20+20+2)
	if _, err := io.ReadFull(connection, proof); err != nil {
		return err
	}
	if proof[0] != logonProofOpcode {
		return fmt.Errorf("invalid proof opcode %#x", proof[0])
	}
	var aPublic [KeyLen]byte
	copy(aPublic[:], proof[1:1+KeyLen])
	clientM1 := proof[1+KeyLen : 1+KeyLen+20]
	uHash := sha1Parts(aPublic[:], bPublic[:])
	u := fromLittleEndian(uHash[:])
	a := fromLittleEndian(aPublic[:])
	secret := new(big.Int).Mul(a, new(big.Int).Exp(v, u, n))
	secret.Mod(secret, n)
	secret.Exp(secret, bPrivate, n)
	var secretLE [KeyLen]byte
	copy(secretLE[:], toLittleEndian(secret, KeyLen))
	key := interleave(secretLE)
	hashN := sha1Parts(nLE[:])
	hashG := sha1Parts([]byte{generator})
	var nXorG [20]byte
	for i := range nXorG {
		nXorG[i] = hashN[i] ^ hashG[i]
	}
	hashUser := sha1Parts([]byte(username))
	expectedM1 := sha1Parts(nXorG[:], hashUser[:], salt[:], aPublic[:], bPublic[:], key[:])
	if !bytes.Equal(clientM1, expectedM1[:]) {
		return fmt.Errorf("client proof did not verify")
	}
	m2 := sha1Parts(aPublic[:], clientM1, key[:])
	proofReply := []byte{logonProofOpcode, 0}
	proofReply = append(proofReply, m2[:]...)
	proofReply = append(proofReply, make([]byte, 10)...)
	if err := writeFull(connection, proofReply); err != nil {
		return err
	}
	request := make([]byte, 5)
	if _, err := io.ReadFull(connection, request); err != nil {
		return err
	}
	if request[0] != realmListOpcode {
		return fmt.Errorf("invalid realm-list opcode %#x", request[0])
	}
	body := make([]byte, 0, 64)
	body = append(body, 0, 0, 0, 0, 1, 0, 0, 0, 0)
	body = append(body, []byte("Test Realm\x00127.0.0.1:8085\x00")...)
	body = append(body, 0, 0, 0, 0, 0, 1, 1)
	body = append(body, 0, 0)
	response := []byte{realmListOpcode, byte(len(body)), byte(len(body) >> 8)}
	response = append(response, body...)
	return writeFull(connection, response)
}

func writeFull(connection net.Conn, data []byte) error {
	for len(data) > 0 {
		written, err := connection.Write(data)
		if err != nil {
			return err
		}
		if written == 0 {
			return io.ErrShortWrite
		}
		data = data[written:]
	}
	return nil
}
