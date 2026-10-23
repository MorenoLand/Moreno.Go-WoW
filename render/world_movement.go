package render

import (
	"math"
	"time"

	"github.com/MorenoLand/Moreno.WoW/world"
)

// worldMoveHeartbeat is the interval between MSG_MOVE_HEARTBEAT packets while
// locomoting; WoWee's working 3.3.5 client uses 0.5s (moveHeartbeatInterval_).
const worldMoveHeartbeat = 500 * time.Millisecond

// worldFacingResend limits MSG_MOVE_SET_FACING while standing still, and
// MSG_MOVE_SET_PITCH while flying or swimming.
const worldFacingResend = 100 * time.Millisecond

// worldMovementReporter turns the local controller's state into the movement
// packet stream the server integrates: a start/stop packet when an axis
// changes, jump, fall-land, swim and ascend packets when the mode changes, a
// heartbeat while moving, and a facing packet when turning in place. Position
// comes from the controller; the server never confirms it.
type worldMovementReporter struct {
	guid        uint64
	started     time.Time
	send        func(world.ClientOpcode, world.MovementInfo) error
	flags       uint32
	lastBeat    time.Time
	lastFacing  float32
	facingSent  time.Time
	pitchSent   time.Time
	lastPitch   float32
	haveFacing  bool
	failures    int
	lastFailure error
}

func newWorldMovementReporter(guid uint64, send func(world.ClientOpcode, world.MovementInfo) error) *worldMovementReporter {
	return &worldMovementReporter{guid: guid, started: time.Now(), send: send}
}

// worldInputFlags maps held movement commands to movement intent flags.
// Opposite commands cancel, as in the original client.
func worldInputFlags(commands map[string]bool) uint32 {
	flags := uint32(0)
	if commands[cmdForward] != commands[cmdBackward] {
		if commands[cmdForward] {
			flags |= world.MovementFlagForward
		} else {
			flags |= world.MovementFlagBackward
		}
	}
	if commands[cmdStrafeLeft] != commands[cmdStrafeRight] {
		if commands[cmdStrafeLeft] {
			flags |= world.MovementFlagStrafeLeft
		} else {
			flags |= world.MovementFlagStrafeRight
		}
	}
	return flags
}

// worldMovementTransitions is the packets a change of movement state needs: an
// axis that changed produces a packet and stopping an axis never stops the
// other; the opcode names the axis that changed while the flags carry the
// whole state.
func worldMovementTransitions(before, after uint32) []world.ClientOpcode {
	var out []world.ClientOpcode
	const longitudinal = world.MovementFlagForward | world.MovementFlagBackward
	const lateral = world.MovementFlagStrafeLeft | world.MovementFlagStrafeRight
	const turning = world.MovementFlagTurnLeft | world.MovementFlagTurnRight
	if before&longitudinal != after&longitudinal {
		switch {
		case after&world.MovementFlagForward != 0:
			out = append(out, world.MoveStartForwardOpcode)
		case after&world.MovementFlagBackward != 0:
			out = append(out, world.MoveStartBackwardOpcode)
		default:
			out = append(out, world.MoveStopOpcode)
		}
	}
	if before&lateral != after&lateral {
		switch {
		case after&world.MovementFlagStrafeLeft != 0:
			out = append(out, world.MoveStartStrafeLeftOpcode)
		case after&world.MovementFlagStrafeRight != 0:
			out = append(out, world.MoveStartStrafeRightOpcode)
		default:
			out = append(out, world.MoveStopStrafeOpcode)
		}
	}
	if before&turning != after&turning {
		switch {
		case after&world.MovementFlagTurnLeft != 0:
			out = append(out, world.MoveStartTurnLeftOpcode)
		case after&world.MovementFlagTurnRight != 0:
			out = append(out, world.MoveStartTurnRightOpcode)
		default:
			out = append(out, world.MoveStopTurnOpcode)
		}
	}
	if before&world.MovementFlagWalking != after&world.MovementFlagWalking {
		if after&world.MovementFlagWalking != 0 {
			out = append(out, world.MoveSetWalkModeOpcode)
		} else {
			out = append(out, world.MoveSetRunModeOpcode)
		}
	}
	if before&world.MovementFlagSwimming != after&world.MovementFlagSwimming {
		if after&world.MovementFlagSwimming != 0 {
			out = append(out, world.MoveStartSwimOpcode)
		} else {
			out = append(out, world.MoveStopSwimOpcode)
		}
	}
	const vertical = world.MovementFlagAscending | world.MovementFlagDescending
	if before&vertical != after&vertical {
		switch {
		case after&world.MovementFlagAscending != 0:
			out = append(out, world.MoveStartAscendOpcode)
		case after&world.MovementFlagDescending != 0:
			out = append(out, world.MoveStartDescendOpcode)
		default:
			out = append(out, world.MoveStopAscendOpcode)
		}
	}
	return out
}

func (r *worldMovementReporter) report(opcode world.ClientOpcode, info world.MovementInfo) {
	if err := r.send(opcode, info); err != nil {
		r.failures++
		r.lastFailure = err
	}
}

// info is the movement state the packet carries.
func (r *worldMovementReporter) info(now time.Time, c *worldCameraController, flags uint32) world.MovementInfo {
	// The original client faces the character along the camera while it is
	// moving; the local body turns toward the move direction for presentation
	// only, so reporting it would point the server's integration the wrong way.
	facing := c.bodyYaw
	if flags&(world.MovementFlagForward|world.MovementFlagBackward|world.MovementFlagStrafeLeft|world.MovementFlagStrafeRight) != 0 {
		facing = c.yaw
	}
	info := world.MovementInfo{
		Flags:  flags,
		TimeMS: uint32(now.Sub(r.started).Milliseconds()),
		Position: world.WorldPosition{
			X: c.position[0], Y: c.position[1], Z: c.position[2],
			Orientation: worldNormalizeOrientation(facing),
		},
	}
	if flags&(world.MovementFlagSwimming|world.MovementFlagFlying) != 0 {
		info.Pitch = c.pitch
	}
	if flags&world.MovementFlagFalling != 0 {
		info.FallTime = uint32(math.Max(0, (c.clock-c.fallStart)*1000))
		info.Fall = c.fallLaunch
	}
	return info
}

// update is called once per frame after the controller has advanced.
func (r *worldMovementReporter) update(now time.Time, c *worldCameraController) {
	if r == nil || c == nil || r.send == nil {
		return
	}
	flags := c.flags()
	jumped, landed, _, touchedDown := c.takeEvents()
	info := r.info(now, c, flags)
	if jumped {
		// The jump packet is the one that first carries the falling block.
		jump := info
		jump.Flags |= world.MovementFlagFalling
		jump.FallTime = 0
		jump.Fall = c.fallLaunch
		r.report(world.MoveJumpOpcode, jump)
		r.lastBeat = now
		r.flags |= world.MovementFlagFalling
	}
	for _, opcode := range worldMovementTransitions(r.flags, flags) {
		r.report(opcode, info)
		r.lastBeat = now
	}
	if landed {
		land := info
		land.Flags &^= world.MovementFlagFalling
		r.report(world.MoveFallLandOpcode, land)
		r.lastBeat = now
	}
	if touchedDown {
		// Landing from flight is said with a heartbeat: it carries the flags and
		// a player who lands and stands still sends nothing else.
		r.report(world.MoveHeartbeatOpcode, info)
		r.lastBeat = now
	}
	r.flags = flags
	moving := flags&(world.MovementFlagForward|world.MovementFlagBackward|world.MovementFlagStrafeLeft|world.MovementFlagStrafeRight|
		world.MovementFlagFalling|world.MovementFlagAscending|world.MovementFlagDescending) != 0
	if moving {
		if now.Sub(r.lastBeat) >= worldMoveHeartbeat {
			r.report(world.MoveHeartbeatOpcode, info)
			r.lastBeat = now
		}
		r.lastFacing, r.haveFacing = info.Position.Orientation, true
		r.lastPitch = info.Pitch
		return
	}
	if flags&(world.MovementFlagSwimming|world.MovementFlagFlying) != 0 &&
		math.Abs(float64(info.Pitch-r.lastPitch)) > 0.02 && now.Sub(r.pitchSent) >= worldFacingResend {
		r.report(world.MoveSetPitchOpcode, info)
		r.lastPitch, r.pitchSent = info.Pitch, now
	}
	if !r.haveFacing {
		r.lastFacing, r.haveFacing = info.Position.Orientation, true
		return
	}
	if flags&(world.MovementFlagTurnLeft|world.MovementFlagTurnRight) == 0 &&
		math.Abs(float64(worldAngleDelta(r.lastFacing, info.Position.Orientation))) > 0.01 && now.Sub(r.facingSent) >= worldFacingResend {
		r.report(world.MoveSetFacingOpcode, info)
		r.lastFacing, r.facingSent = info.Position.Orientation, now
	}
}

func worldNormalizeOrientation(angle float32) float32 {
	const twoPi = 2 * math.Pi
	value := math.Mod(float64(angle), twoPi)
	if value < 0 {
		value += twoPi
	}
	return float32(value)
}
