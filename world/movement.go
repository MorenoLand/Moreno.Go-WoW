package world

import (
	"encoding/binary"
	"fmt"
	"math"
)

// Client movement opcodes (MSG_MOVE_*), identical in both directions. The
// opcode values and the MovementInfo layout below were cross-checked against
// MeoWow (crates/world motion.rs, movement.rs, protocol.rs) and WoWee.
const (
	MoveStartForwardOpcode     ClientOpcode = 0x00B5
	MoveStartBackwardOpcode    ClientOpcode = 0x00B6
	MoveStopOpcode             ClientOpcode = 0x00B7
	MoveStartStrafeLeftOpcode  ClientOpcode = 0x00B8
	MoveStartStrafeRightOpcode ClientOpcode = 0x00B9
	MoveStopStrafeOpcode       ClientOpcode = 0x00BA
	MoveSetFacingOpcode        ClientOpcode = 0x00DA
	MoveHeartbeatOpcode        ClientOpcode = 0x00EE
)

// MovementInfo is the state carried by every movement packet. Only the fields
// the client currently produces are modelled; transport, pitch, falling and
// spline elevation are written only when the matching flag is set, and those
// flags are never set by this client yet.
type MovementInfo struct {
	Flags    uint32
	Flags2   uint16
	TimeMS   uint32
	Position WorldPosition
	FallTime uint32
}

// AppendPackedGUID appends guid in the packed form used by update blocks.
func AppendPackedGUID(dst []byte, guid uint64) []byte {
	mask := byte(0)
	packed := make([]byte, 0, 8)
	for i := uint(0); i < 8; i++ {
		if value := byte(guid >> (i * 8)); value != 0 {
			mask |= 1 << i
			packed = append(packed, value)
		}
	}
	dst = append(dst, mask)
	return append(dst, packed...)
}

// BuildMovement builds a movement packet body: the mover's packed GUID, then
// flags, flags2, time, position, orientation and fall time. The leading GUID
// is required in 3.3.5a; omitting it shifts every following field.
func BuildMovement(mover uint64, info MovementInfo) ([]byte, error) {
	const unsupported = MovementFlagOnTransport | MovementFlagSwimming | MovementFlagFlying | MovementFlagFalling | MovementFlagSplineElevation
	if info.Flags&unsupported != 0 || info.Flags2&MovementFlag2Interpolated != 0 || info.Flags2&MovementFlag2Pitching != 0 {
		return nil, fmt.Errorf("movement flags %#x/%#x need fields this writer does not produce", info.Flags, info.Flags2)
	}
	body := AppendPackedGUID(make([]byte, 0, 40), mover)
	body = binary.LittleEndian.AppendUint32(body, info.Flags)
	body = binary.LittleEndian.AppendUint16(body, info.Flags2)
	body = binary.LittleEndian.AppendUint32(body, info.TimeMS)
	for _, value := range []float32{info.Position.X, info.Position.Y, info.Position.Z, info.Position.Orientation} {
		body = binary.LittleEndian.AppendUint32(body, math.Float32bits(value))
	}
	return binary.LittleEndian.AppendUint32(body, info.FallTime), nil
}

// SendMovement sends one MSG_MOVE_* packet for a mover this client controls.
func (c *Connection) SendMovement(opcode ClientOpcode, mover uint64, info MovementInfo) error {
	if c == nil || c.conn == nil {
		return fmt.Errorf("world connection is closed")
	}
	body, err := BuildMovement(mover, info)
	if err != nil {
		return err
	}
	return c.send(opcode, body)
}
