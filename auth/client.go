package auth

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net"
	"strconv"
	"time"
)

const DefaultPort uint16 = 3724

type LoggedIn struct {
	SessionKey [SessionKeyLen]byte
	Realms     []Realm
}

func Login(host string, port uint16, username, password, locale string, timeout time.Duration) (LoggedIn, error) {
	address := net.JoinHostPort(host, strconv.Itoa(int(port)))
	conn, err := net.DialTimeout("tcp", address, timeout)
	if err != nil {
		return LoggedIn{}, fmt.Errorf("could not reach auth server at %s: %w", address, err)
	}
	defer conn.Close()
	challenge, err := BuildLogonChallenge(username, locale)
	if err != nil {
		return LoggedIn{}, err
	}
	if err := writeWithTimeout(conn, challenge, timeout); err != nil {
		return LoggedIn{}, fmt.Errorf("sending logon challenge: %w", err)
	}
	challengeBytes, err := readChallenge(conn, timeout)
	if err != nil {
		return LoggedIn{}, fmt.Errorf("reading logon challenge: %w", err)
	}
	parsedChallenge, err := ParseChallenge(challengeBytes)
	if err != nil {
		return LoggedIn{}, err
	}
	if parsedChallenge.SecurityFlags != 0 {
		return LoggedIn{}, fmt.Errorf("authenticator or PIN required: security flags %#02x", parsedChallenge.SecurityFlags)
	}
	private, err := RandomPrivate()
	if err != nil {
		return LoggedIn{}, fmt.Errorf("creating SRP6 private key: %w", err)
	}
	session, err := Respond(username, password, parsedChallenge.Salt, parsedChallenge.BPublic, private[:])
	if err != nil {
		return LoggedIn{}, fmt.Errorf("computing SRP6 proof: %w", err)
	}
	if err := writeWithTimeout(conn, BuildLogonProof(session.APublic, session.M1), timeout); err != nil {
		return LoggedIn{}, fmt.Errorf("sending logon proof: %w", err)
	}
	proof, err := readProof(conn, timeout)
	if err != nil {
		return LoggedIn{}, fmt.Errorf("reading logon proof: %w", err)
	}
	m2, err := ParseProof(proof)
	if err != nil {
		return LoggedIn{}, err
	}
	if !session.VerifyServer(m2) {
		return LoggedIn{}, errors.New("auth server proof did not verify")
	}
	if err := writeWithTimeout(conn, BuildRealmListRequest(), timeout); err != nil {
		return LoggedIn{}, fmt.Errorf("requesting realm list: %w", err)
	}
	realmList, err := readRealmList(conn, timeout)
	if err != nil {
		return LoggedIn{}, fmt.Errorf("reading realm list: %w", err)
	}
	realms, err := ParseRealmList(realmList)
	if err != nil {
		return LoggedIn{}, err
	}
	return LoggedIn{SessionKey: session.Key, Realms: realms}, nil
}

func readChallenge(conn net.Conn, timeout time.Duration) ([]byte, error) {
	data := make([]byte, 3)
	if err := readWithTimeout(conn, data, timeout); err != nil {
		return nil, err
	}
	if data[2] != 0 {
		return data, nil
	}
	readAppend := func(count int) error {
		part := make([]byte, count)
		if err := readWithTimeout(conn, part, timeout); err != nil {
			return err
		}
		data = append(data, part...)
		return nil
	}
	if err := readAppend(KeyLen + 1); err != nil {
		return nil, err
	}
	gLen := int(data[len(data)-1])
	if err := readAppend(gLen + 1); err != nil {
		return nil, err
	}
	nLen := int(data[len(data)-1])
	if err := readAppend(nLen + KeyLen + 16 + 1); err != nil {
		return nil, err
	}
	return data, nil
}

func readProof(conn net.Conn, timeout time.Duration) ([]byte, error) {
	data := make([]byte, 2)
	if err := readWithTimeout(conn, data, timeout); err != nil {
		return nil, err
	}
	if data[1] == 0 {
		if err := readAppendWithTimeout(conn, &data, 30, timeout); err != nil {
			return nil, err
		}
	}
	return data, nil
}

func readRealmList(conn net.Conn, timeout time.Duration) ([]byte, error) {
	header := make([]byte, 3)
	if err := readWithTimeout(conn, header, timeout); err != nil {
		return nil, err
	}
	size := int(binary.LittleEndian.Uint16(header[1:3]))
	if size > 1<<20 {
		return nil, fmt.Errorf("realm list is too large: %d bytes", size)
	}
	data := make([]byte, 0, 3+size)
	data = append(data, header...)
	if err := readAppendWithTimeout(conn, &data, size, timeout); err != nil {
		return nil, err
	}
	return data, nil
}

func readAppendWithTimeout(conn net.Conn, data *[]byte, count int, timeout time.Duration) error {
	part := make([]byte, count)
	if err := readWithTimeout(conn, part, timeout); err != nil {
		return err
	}
	*data = append(*data, part...)
	return nil
}

func readWithTimeout(conn net.Conn, data []byte, timeout time.Duration) error {
	if err := conn.SetReadDeadline(time.Now().Add(timeout)); err != nil {
		return err
	}
	_, err := io.ReadFull(conn, data)
	return err
}

func writeWithTimeout(conn net.Conn, data []byte, timeout time.Duration) error {
	if err := conn.SetWriteDeadline(time.Now().Add(timeout)); err != nil {
		return err
	}
	for len(data) > 0 {
		written, err := conn.Write(data)
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
