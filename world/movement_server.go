package world

import (
	"fmt"
	"math"
)

// Server-forced movement changes (SMSG_*) and the acknowledgements the client
// sends back (CMSG_*_ACK).
const (
	ForceWalkSpeedChange       uint16 = 0x02DA
	ForceRunSpeedChange        uint16 = 0x00E2
	ForceRunBackSpeedChange    uint16 = 0x00E4
	ForceSwimSpeedChange       uint16 = 0x00E6
	ForceSwimBackSpeedChange   uint16 = 0x02DC
	ForceTurnRateChange        uint16 = 0x02DE
	ForceFlightSpeedChange     uint16 = 0x0381
	ForceFlightBackSpeedChange uint16 = 0x0383
	ForcePitchRateChange       uint16 = 0x045C
	MoveSetCanFly              uint16 = 0x0343
	MoveUnsetCanFly            uint16 = 0x0344
	MoveWaterWalk              uint16 = 0x00DE
	MoveLandWalk               uint16 = 0x00DF
	MoveFeatherFall            uint16 = 0x00F2
	MoveNormalFall             uint16 = 0x00F3
	MoveSetHover               uint16 = 0x00F4
	MoveUnsetHover             uint16 = 0x00F5
	ForceMoveRoot              uint16 = 0x00E8
	ForceMoveUnroot            uint16 = 0x00EA
	MoveKnockBack              uint16 = 0x00EF
	MoveGravityDisable         uint16 = 0x04CE
	MoveGravityEnable          uint16 = 0x04D0
	MoveTeleportAck            uint16 = 0x00C7 // MSG_MOVE_TELEPORT_ACK, server to client
	NewWorld                   uint16 = 0x003E

	AckForceWalkSpeed       ClientOpcode = 0x02DB
	AckForceRunSpeed        ClientOpcode = 0x00E3
	AckForceRunBackSpeed    ClientOpcode = 0x00E5
	AckForceSwimSpeed       ClientOpcode = 0x00E7
	AckForceSwimBackSpeed   ClientOpcode = 0x02DD
	AckForceTurnRate        ClientOpcode = 0x02DF
	AckForceFlightSpeed     ClientOpcode = 0x0382
	AckForceFlightBackSpeed ClientOpcode = 0x0384
	AckForcePitchRate       ClientOpcode = 0x045D
	AckMoveSetCanFly        ClientOpcode = 0x0345
	AckMoveWaterWalk        ClientOpcode = 0x02D0
	AckMoveFeatherFall      ClientOpcode = 0x02CF
	AckMoveHover            ClientOpcode = 0x00F6
	AckForceMoveRoot        ClientOpcode = 0x00E9
	AckForceMoveUnroot      ClientOpcode = 0x00EB
	AckMoveKnockBack        ClientOpcode = 0x00F0
	AckMoveGravityDisable   ClientOpcode = 0x04CF
	AckMoveGravityEnable    ClientOpcode = 0x04D1
)

// SpeedKind names the speeds a mover has.
type SpeedKind int

const (
	SpeedKindWalk SpeedKind = iota
	SpeedKindRun
	SpeedKindRunBack
	SpeedKindSwim
	SpeedKindSwimBack
	SpeedKindTurn
	SpeedKindFlight
	SpeedKindFlightBack
	SpeedKindPitch
	SpeedKindCount
)

// ForcedChange describes what a server-forced movement packet asks of the
// player and how to acknowledge it.
type ForcedChange struct {
	Speed    SpeedKind // valid when HasSpeed
	HasSpeed bool
	Value    float32 // the new speed
	// Flag is the movement flag to set or clear (zero for none).
	Flag uint32
	Set  bool
	// Ack is the acknowledgement opcode and whether it carries the speed.
	Ack      ClientOpcode
	AckSpeed bool
}

// forcedChanges maps a server opcode to its effect. Counter and GUID come from
// the packet.
var forcedChanges = map[uint16]ForcedChange{
	ForceWalkSpeedChange:       {Speed: SpeedKindWalk, HasSpeed: true, Ack: AckForceWalkSpeed, AckSpeed: true},
	ForceRunSpeedChange:        {Speed: SpeedKindRun, HasSpeed: true, Ack: AckForceRunSpeed, AckSpeed: true},
	ForceRunBackSpeedChange:    {Speed: SpeedKindRunBack, HasSpeed: true, Ack: AckForceRunBackSpeed, AckSpeed: true},
	ForceSwimSpeedChange:       {Speed: SpeedKindSwim, HasSpeed: true, Ack: AckForceSwimSpeed, AckSpeed: true},
	ForceSwimBackSpeedChange:   {Speed: SpeedKindSwimBack, HasSpeed: true, Ack: AckForceSwimBackSpeed, AckSpeed: true},
	ForceTurnRateChange:        {Speed: SpeedKindTurn, HasSpeed: true, Ack: AckForceTurnRate, AckSpeed: true},
	ForceFlightSpeedChange:     {Speed: SpeedKindFlight, HasSpeed: true, Ack: AckForceFlightSpeed, AckSpeed: true},
	ForceFlightBackSpeedChange: {Speed: SpeedKindFlightBack, HasSpeed: true, Ack: AckForceFlightBackSpeed, AckSpeed: true},
	ForcePitchRateChange:       {Speed: SpeedKindPitch, HasSpeed: true, Ack: AckForcePitchRate, AckSpeed: true},
	MoveSetCanFly:              {Flag: MovementFlagCanFly, Set: true, Ack: AckMoveSetCanFly},
	MoveUnsetCanFly:            {Flag: MovementFlagCanFly, Set: false, Ack: AckMoveSetCanFly},
	MoveWaterWalk:              {Flag: MovementFlagWaterWalking, Set: true, Ack: AckMoveWaterWalk},
	MoveLandWalk:               {Flag: MovementFlagWaterWalking, Set: false, Ack: AckMoveWaterWalk},
	MoveFeatherFall:            {Flag: MovementFlagSafeFall, Set: true, Ack: AckMoveFeatherFall},
	MoveNormalFall:             {Flag: MovementFlagSafeFall, Set: false, Ack: AckMoveFeatherFall},
	MoveSetHover:               {Flag: MovementFlagHover, Set: true, Ack: AckMoveHover},
	MoveUnsetHover:             {Flag: MovementFlagHover, Set: false, Ack: AckMoveHover},
	ForceMoveRoot:              {Flag: MovementFlagRoot, Set: true, Ack: AckForceMoveRoot},
	ForceMoveUnroot:            {Flag: MovementFlagRoot, Set: false, Ack: AckForceMoveUnroot},
	MoveGravityDisable:         {Flag: MovementFlagLevitating, Set: true, Ack: AckMoveGravityDisable},
	MoveGravityEnable:          {Flag: MovementFlagLevitating, Set: false, Ack: AckMoveGravityEnable},
}

// ForcedChangeFor returns the effect of a server opcode, if it is one.
func ForcedChangeFor(opcode uint16) (ForcedChange, bool) {
	change, ok := forcedChanges[opcode]
	return change, ok
}

// ParseForcedChange reads a server-forced change packet: the mover's packed
// GUID, the change counter and (for speeds) the new speed.
func ParseForcedChange(opcode uint16, body []byte) (guid uint64, counter uint32, change ForcedChange, err error) {
	change, ok := forcedChanges[opcode]
	if !ok {
		return 0, 0, change, fmt.Errorf("opcode %#x is not a forced movement change", opcode)
	}
	r := NewReader(body, "forced movement change")
	if guid, err = ReadPackedGUID(r); err != nil {
		return
	}
	if counter, err = r.U32(); err != nil {
		return
	}
	if change.HasSpeed {
		// Run speed carries one extra byte before the value in 3.3.5a; read by
		// what is left so both shapes parse.
		remaining := r.Remaining()
		switch {
		case remaining >= 8:
			err = r.Skip(4)
		case remaining >= 5:
			err = r.Skip(1)
		}
		if err != nil {
			return
		}
		var value float32
		if value, err = r.F32(); err != nil {
			return
		}
		if math.IsNaN(float64(value)) || value < 0.1 || value > 100 {
			err = fmt.Errorf("forced speed %v is out of range", value)
			return
		}
		change.Value = value
	}
	return
}

// KnockBack is SMSG_MOVE_KNOCK_BACK: the horizontal direction (cos, sin), the
// horizontal speed and the vertical speed.
type KnockBack struct {
	GUID                           uint64
	Counter                        uint32
	CosAngle, SinAngle             float32
	HorizontalSpeed, VerticalSpeed float32
}

// ParseKnockBack reads SMSG_MOVE_KNOCK_BACK.
func ParseKnockBack(body []byte) (KnockBack, error) {
	r := NewReader(body, "SMSG_MOVE_KNOCK_BACK")
	var knock KnockBack
	var err error
	if knock.GUID, err = ReadPackedGUID(r); err != nil {
		return knock, err
	}
	if knock.Counter, err = r.U32(); err != nil {
		return knock, err
	}
	for _, target := range []*float32{&knock.CosAngle, &knock.SinAngle, &knock.HorizontalSpeed, &knock.VerticalSpeed} {
		if *target, err = r.F32(); err != nil {
			return knock, err
		}
	}
	return knock, nil
}

// Teleport is MSG_MOVE_TELEPORT_ACK from the server: the mover, a counter and
// the destination as movement info.
type Teleport struct {
	GUID     uint64
	Counter  uint32
	Movement MovementInfo
}

// ParseTeleport reads the server's teleport instruction.
func ParseTeleport(body []byte) (Teleport, error) {
	r := NewReader(body, "MSG_MOVE_TELEPORT_ACK")
	var tele Teleport
	var err error
	if tele.GUID, err = ReadPackedGUID(r); err != nil {
		return tele, err
	}
	if tele.Counter, err = r.U32(); err != nil {
		return tele, err
	}
	tele.Movement, err = ReadMovementInfo(r)
	return tele, err
}
