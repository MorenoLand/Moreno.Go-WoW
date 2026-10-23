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
	MoveJumpOpcode             ClientOpcode = 0x00BB
	MoveStartTurnLeftOpcode    ClientOpcode = 0x00BC
	MoveStartTurnRightOpcode   ClientOpcode = 0x00BD
	MoveStopTurnOpcode         ClientOpcode = 0x00BE
	MoveStartPitchUpOpcode     ClientOpcode = 0x00BF
	MoveStartPitchDownOpcode   ClientOpcode = 0x00C0
	MoveStopPitchOpcode        ClientOpcode = 0x00C1
	MoveSetRunModeOpcode       ClientOpcode = 0x00C2
	MoveSetWalkModeOpcode      ClientOpcode = 0x00C3
	MoveTeleportAckOpcode      ClientOpcode = 0x00C7
	MoveFallLandOpcode         ClientOpcode = 0x00C9
	MoveStartSwimOpcode        ClientOpcode = 0x00CA
	MoveStopSwimOpcode         ClientOpcode = 0x00CB
	MoveSetFacingOpcode        ClientOpcode = 0x00DA
	MoveSetPitchOpcode         ClientOpcode = 0x00DB
	MoveWorldportAckOpcode     ClientOpcode = 0x00DC
	MoveHeartbeatOpcode        ClientOpcode = 0x00EE
	MoveFallResetOpcode        ClientOpcode = 0x02CA
	MoveSetFlyOpcode           ClientOpcode = 0x0346
	MoveStartAscendOpcode      ClientOpcode = 0x0359
	MoveStopAscendOpcode       ClientOpcode = 0x035A
	MoveStartDescendOpcode     ClientOpcode = 0x03A7
	SetActiveMoverOpcode       ClientOpcode = 0x026A
)

// Movement flags (3.3.5a MovementFlags). Forward..PitchDown are the input
// state, the rest describe what the mover is doing.
const (
	MovementFlagTurnLeft     uint32 = 0x00000010
	MovementFlagTurnRight    uint32 = 0x00000020
	MovementFlagPitchUp      uint32 = 0x00000040
	MovementFlagPitchDown    uint32 = 0x00000080
	MovementFlagLevitating   uint32 = 0x00000400
	MovementFlagRoot         uint32 = 0x00000800
	MovementFlagFallingFar   uint32 = 0x00002000
	MovementFlagAscending    uint32 = 0x00400000
	MovementFlagDescending   uint32 = 0x00800000
	MovementFlagCanFly       uint32 = 0x01000000
	MovementFlagWaterWalking uint32 = 0x10000000
	MovementFlagSafeFall     uint32 = 0x20000000
	MovementFlagHover        uint32 = 0x40000000

	// MovementFlag2AlwaysAllowPitching is the 3.3.5a "pitching" flags2 bit that
	// makes the pitch field present even when not swimming or flying.
	MovementFlag2AlwaysAllowPitching = MovementFlag2Pitching
)

// Physics constants the client and the server agree on (yards, seconds).
const (
	JumpInitialSpeed = 7.955547
	Gravity          = 19.2911
	TerminalFall     = 60.1481
)

// Default unit speeds (yards per second, turn rate in radians per second).
const (
	SpeedWalk       = 2.5
	SpeedRun        = 7.0
	SpeedRunBack    = 4.5
	SpeedSwim       = 4.722222
	SpeedSwimBack   = 2.5
	SpeedFlight     = 7.0
	SpeedFlightBack = 4.5
	SpeedTurn       = math.Pi
	SpeedPitch      = math.Pi
)

// Transport is the part of a movement packet that places the mover on a
// transport.
type Transport struct {
	GUID     uint64
	Position WorldPosition
	TimeMS   uint32
	Seat     int8
	TimeMS2  uint32 // present only with MovementFlag2Interpolated
}

// FallInfo is the jump block: the vertical speed (negative is upwards), the
// horizontal direction the jump started in and its speed.
type FallInfo struct {
	ZSpeed   float32
	SinAngle float32
	CosAngle float32
	XYSpeed  float32
}

// MovementInfo is the state carried by every movement packet.
type MovementInfo struct {
	Flags           uint32
	Flags2          uint16
	TimeMS          uint32
	Position        WorldPosition
	Transport       Transport
	Pitch           float32
	FallTime        uint32
	Fall            FallInfo
	SplineElevation float32
}

func (info MovementInfo) hasPitch() bool {
	return info.Flags&(MovementFlagSwimming|MovementFlagFlying) != 0 || info.Flags2&MovementFlag2AlwaysAllowPitching != 0
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

func appendFloat(dst []byte, values ...float32) []byte {
	for _, value := range values {
		dst = binary.LittleEndian.AppendUint32(dst, math.Float32bits(value))
	}
	return dst
}

// AppendMovementInfo writes the movement layout: the optional blocks follow
// the flags, not whether the fields happen to be populated, because the reader
// on the other side has only the flags to go on.
func AppendMovementInfo(body []byte, info MovementInfo) []byte {
	body = binary.LittleEndian.AppendUint32(body, info.Flags)
	body = binary.LittleEndian.AppendUint16(body, info.Flags2)
	body = binary.LittleEndian.AppendUint32(body, info.TimeMS)
	body = appendFloat(body, info.Position.X, info.Position.Y, info.Position.Z, info.Position.Orientation)
	if info.Flags&MovementFlagOnTransport != 0 {
		body = AppendPackedGUID(body, info.Transport.GUID)
		body = appendFloat(body, info.Transport.Position.X, info.Transport.Position.Y, info.Transport.Position.Z, info.Transport.Position.Orientation)
		body = binary.LittleEndian.AppendUint32(body, info.Transport.TimeMS)
		body = append(body, byte(info.Transport.Seat))
		if info.Flags2&MovementFlag2Interpolated != 0 {
			body = binary.LittleEndian.AppendUint32(body, info.Transport.TimeMS2)
		}
	}
	if info.hasPitch() {
		body = appendFloat(body, info.Pitch)
	}
	body = binary.LittleEndian.AppendUint32(body, info.FallTime)
	if info.Flags&MovementFlagFalling != 0 {
		body = appendFloat(body, info.Fall.ZSpeed, info.Fall.SinAngle, info.Fall.CosAngle, info.Fall.XYSpeed)
	}
	if info.Flags&MovementFlagSplineElevation != 0 {
		body = appendFloat(body, info.SplineElevation)
	}
	return body
}

// ReadMovementInfo reads the layout AppendMovementInfo writes.
func ReadMovementInfo(r *Reader) (MovementInfo, error) {
	var info MovementInfo
	var err error
	if info.Flags, err = r.U32(); err != nil {
		return info, err
	}
	if info.Flags2, err = r.U16(); err != nil {
		return info, err
	}
	if info.TimeMS, err = r.U32(); err != nil {
		return info, err
	}
	readPosition := func() (WorldPosition, error) {
		var position WorldPosition
		var perr error
		for _, target := range []*float32{&position.X, &position.Y, &position.Z, &position.Orientation} {
			if *target, perr = r.F32(); perr != nil {
				return position, perr
			}
		}
		return position, nil
	}
	if info.Position, err = readPosition(); err != nil {
		return info, err
	}
	if info.Flags&MovementFlagOnTransport != 0 {
		if info.Transport.GUID, err = ReadPackedGUID(r); err != nil {
			return info, err
		}
		if info.Transport.Position, err = readPosition(); err != nil {
			return info, err
		}
		if info.Transport.TimeMS, err = r.U32(); err != nil {
			return info, err
		}
		seat, serr := r.U8()
		if serr != nil {
			return info, serr
		}
		info.Transport.Seat = int8(seat)
		if info.Flags2&MovementFlag2Interpolated != 0 {
			if info.Transport.TimeMS2, err = r.U32(); err != nil {
				return info, err
			}
		}
	}
	if info.hasPitch() {
		if info.Pitch, err = r.F32(); err != nil {
			return info, err
		}
	}
	if info.FallTime, err = r.U32(); err != nil {
		return info, err
	}
	if info.Flags&MovementFlagFalling != 0 {
		for _, target := range []*float32{&info.Fall.ZSpeed, &info.Fall.SinAngle, &info.Fall.CosAngle, &info.Fall.XYSpeed} {
			if *target, err = r.F32(); err != nil {
				return info, err
			}
		}
	}
	if info.Flags&MovementFlagSplineElevation != 0 {
		if info.SplineElevation, err = r.F32(); err != nil {
			return info, err
		}
	}
	return info, nil
}

// BuildMovement builds a movement packet body: the mover's packed GUID, then
// the movement layout. The leading GUID is required in 3.3.5a; omitting it
// shifts every following field.
func BuildMovement(mover uint64, info MovementInfo) ([]byte, error) {
	body := AppendPackedGUID(make([]byte, 0, 64), mover)
	return AppendMovementInfo(body, info), nil
}

// BuildMovementAck builds the body of the acknowledgements a client sends for
// a server-forced change: the mover's unpacked GUID, the change counter, the
// movement layout and then any value the change carries (a speed).
func BuildMovementAck(mover uint64, counter uint32, info MovementInfo, extra ...float32) []byte {
	body := binary.LittleEndian.AppendUint64(make([]byte, 0, 64), mover)
	body = binary.LittleEndian.AppendUint32(body, counter)
	body = AppendMovementInfo(body, info)
	return appendFloat(body, extra...)
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

// SendRaw sends a client packet with a body the caller built.
func (c *Connection) SendRaw(opcode ClientOpcode, body []byte) error {
	if c == nil || c.conn == nil {
		return fmt.Errorf("world connection is closed")
	}
	return c.send(opcode, body)
}
