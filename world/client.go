package world

import (
	"crypto/rand"
	"fmt"
	"io"
	"net"
	"strconv"
	"strings"
	"time"

	"github.com/MorenoLand/Moreno.WoW/auth"
)

const DefaultPort uint16 = 8085

type Connection struct {
	conn      net.Conn
	crypt     *HeaderCrypt
	timeout   time.Duration
	started   time.Time
	expansion uint8
}

func Open(address, account string, realmID uint8, sessionKey [auth.SessionKeyLen]byte, timeout time.Duration) (*Connection, error) {
	conn, err := net.DialTimeout("tcp", address, timeout)
	if err != nil {
		return nil, fmt.Errorf("could not reach world server at %s: %w", address, err)
	}
	connection := &Connection{conn: conn, timeout: timeout, started: time.Now()}
	if err := connection.handshake(account, uint32(realmID), sessionKey); err != nil {
		_ = conn.Close()
		return nil, err
	}
	return connection, nil
}

func (c *Connection) Close() error { return c.conn.Close() }

func (c *Connection) Expansion() uint8 { return c.expansion }

func (c *Connection) handshake(account string, realmID uint32, sessionKey [auth.SessionKeyLen]byte) error {
	packet, err := c.receive()
	if err != nil {
		return fmt.Errorf("reading world auth challenge: %w", err)
	}
	if packet.Opcode != AuthChallengeResponse {
		return fmt.Errorf("expected SMSG_AUTH_CHALLENGE, got %#04x", packet.Opcode)
	}
	challenge, err := ParseAuthChallenge(packet.Body)
	if err != nil {
		return err
	}
	var clientSeed [4]byte
	if _, err := rand.Read(clientSeed[:]); err != nil {
		return fmt.Errorf("creating world seed: %w", err)
	}
	body, err := BuildAuthSession(account, realmID, clientSeed, challenge.ServerSeed, sessionKey)
	if err != nil {
		return fmt.Errorf("building world auth session: %w", err)
	}
	if err := c.send(AuthSession, body); err != nil {
		return fmt.Errorf("sending world auth session: %w", err)
	}
	c.crypt = NewHeaderCrypt(sessionKey)
	packet, err = c.expect(AuthResponsePacket)
	if err != nil {
		return err
	}
	response, err := ParseAuthResponse(packet.Body)
	if err != nil {
		return err
	}
	switch response.Kind {
	case AuthOK:
		c.expansion = response.Expansion
		return nil
	case AuthQueued:
		return fmt.Errorf("world login queued at position %d", response.QueuePosition)
	default:
		return fmt.Errorf("world server refused session with code %#02x", response.Code)
	}
}

func (c *Connection) Characters() ([]Character, error) {
	if err := c.send(CharEnum, nil); err != nil {
		return nil, err
	}
	packet, err := c.expect(CharEnumResponse)
	if err != nil {
		return nil, err
	}
	return ParseCharEnum(packet.Body)
}

func (c *Connection) EnterWorld(guid uint64) (WorldPosition, error) {
	if err := c.send(PlayerLogin, BuildPlayerLogin(guid)); err != nil {
		return WorldPosition{}, err
	}
	packet, err := c.expect(LoginVerifyWorld)
	if err != nil {
		return WorldPosition{}, err
	}
	return ParseLoginVerifyWorld(packet.Body)
}

func (c *Connection) expect(opcode uint16) (Packet, error) {
	for skipped := 0; skipped < 512; skipped++ {
		packet, err := c.receive()
		if err != nil {
			return Packet{}, err
		}
		if packet.Opcode == opcode {
			return packet, nil
		}
		if err := c.housekeep(packet); err != nil {
			return Packet{}, err
		}
	}
	return Packet{}, fmt.Errorf("gave up waiting for opcode %#04x", opcode)
}

func (c *Connection) housekeep(packet Packet) error {
	switch packet.Opcode {
	case TimeSyncRequest:
		counter, err := ParseTimeSyncRequest(packet.Body)
		if err != nil {
			return err
		}
		ticks := uint32(time.Since(c.started).Milliseconds())
		return c.send(TimeSyncResponse, BuildTimeSyncResponse(counter, ticks))
	case CharacterLoginFailed:
		code, err := ParseResultCode(packet.Body, "SMSG_CHARACTER_LOGIN_FAILED")
		if err != nil {
			return err
		}
		return fmt.Errorf("character login failed with code %#02x", code)
	default:
		return nil
	}
}

type Packet struct {
	Opcode uint16
	Body   []byte
}

func (c *Connection) send(opcode ClientOpcode, body []byte) error {
	packet, err := BuildClientPacket(opcode, body)
	if err != nil {
		return err
	}
	if c.crypt != nil {
		c.crypt.Encrypt(packet[:ClientHeaderLen])
	}
	if err := c.conn.SetWriteDeadline(time.Now().Add(c.timeout)); err != nil {
		return err
	}
	for len(packet) > 0 {
		written, err := c.conn.Write(packet)
		if err != nil {
			return err
		}
		if written == 0 {
			return io.ErrShortWrite
		}
		packet = packet[written:]
	}
	return nil
}

func (c *Connection) receive() (Packet, error) {
	var header [ServerHeaderLargeLen]byte
	if err := c.readFull(header[:1]); err != nil {
		return Packet{}, err
	}
	if c.crypt != nil {
		c.crypt.Decrypt(header[:1])
	}
	headerLen := ServerHeaderLenForFirstByte(header[0])
	if err := c.readFull(header[1:headerLen]); err != nil {
		return Packet{}, err
	}
	if c.crypt != nil {
		c.crypt.Decrypt(header[1:headerLen])
	}
	parsed, err := ParseServerHeader(header[:headerLen])
	if err != nil {
		return Packet{}, err
	}
	body := make([]byte, parsed.BodyLen)
	if err := c.readFull(body); err != nil {
		return Packet{}, err
	}
	return Packet{Opcode: parsed.Opcode, Body: body}, nil
}

func (c *Connection) readFull(data []byte) error {
	if err := c.conn.SetReadDeadline(time.Now().Add(c.timeout)); err != nil {
		return err
	}
	_, err := io.ReadFull(c.conn, data)
	return err
}

func SplitRealmAddress(address string) (string, uint16, error) {
	address = strings.TrimSpace(address)
	if address == "" {
		return "", 0, fmt.Errorf("realm address is empty")
	}
	if host, port, err := net.SplitHostPort(address); err == nil {
		value, err := strconv.ParseUint(port, 10, 16)
		if err != nil || value == 0 {
			return "", 0, fmt.Errorf("invalid realm port in %q", address)
		}
		return host, uint16(value), nil
	}
	if strings.HasPrefix(address, "[") {
		close := strings.IndexByte(address, ']')
		if close < 0 {
			return "", 0, fmt.Errorf("invalid bracketed realm address %q", address)
		}
		if len(address) == close+1 {
			return address[1:close], DefaultPort, nil
		}
		if address[close+1] != ':' {
			return "", 0, fmt.Errorf("invalid realm address %q", address)
		}
		value, err := strconv.ParseUint(address[close+2:], 10, 16)
		if err != nil || value == 0 {
			return "", 0, fmt.Errorf("invalid realm port in %q", address)
		}
		return address[1:close], uint16(value), nil
	}
	if strings.Count(address, ":") == 1 {
		index := strings.LastIndexByte(address, ':')
		value, err := strconv.ParseUint(address[index+1:], 10, 16)
		if err != nil || value == 0 {
			return "", 0, fmt.Errorf("invalid realm port in %q", address)
		}
		return address[:index], uint16(value), nil
	}
	return strings.Trim(address, "[]"), DefaultPort, nil
}
