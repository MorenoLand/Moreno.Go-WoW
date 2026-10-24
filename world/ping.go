package world

import "encoding/binary"

const (
	// PingOpcode is CMSG_PING and PongOpcode its answer, SMSG_PONG.
	PingOpcode ClientOpcode = 0x01DC
	PongOpcode uint16       = 0x01DD
)

// BuildPing is the body of CMSG_PING: a sequence number and the latency the
// client last measured.
func BuildPing(sequence, latencyMS uint32) []byte {
	body := make([]byte, 8)
	binary.LittleEndian.PutUint32(body[0:], sequence)
	binary.LittleEndian.PutUint32(body[4:], latencyMS)
	return body
}

// ParsePong reads the sequence number SMSG_PONG echoes.
func ParsePong(body []byte) (uint32, error) {
	r := NewReader(body, "SMSG_PONG")
	return r.U32()
}
