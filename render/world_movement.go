package render

import (
	"math"
	"time"

	"github.com/MorenoLand/Moreno.WoW/world"
	"github.com/g3n/engine/window"
)

// worldMoveHeartbeat is the interval between MSG_MOVE_HEARTBEAT packets while
// locomoting; WoWee's working 3.3.5 client uses 0.5s (moveHeartbeatInterval_).
const worldMoveHeartbeat = 500 * time.Millisecond

// worldFacingResend limits MSG_MOVE_SET_FACING while standing still.
const worldFacingResend = 100 * time.Millisecond

// worldMovementReporter turns the local controller's input into the movement
// packet stream the server integrates: a start/stop packet when an axis
// changes, a heartbeat while moving, and a facing packet when turning in
// place. Position comes from the controller; the server never confirms it.
//
// Not implemented yet: jump/fall (MSG_MOVE_JUMP and the falling block), swim,
// fly, transports and server-driven speed changes. The controller's local
// physics also differ from the server's, so the packets report the position
// the controller reached rather than a server-simulated one.
type worldMovementReporter struct {
	guid        uint64
	started     time.Time
	send        func(world.ClientOpcode, world.MovementInfo) error
	flags       uint32
	lastBeat    time.Time
	lastFacing  float32
	facingSent  time.Time
	haveFacing  bool
	failures    int
	lastFailure error
}

func newWorldMovementReporter(guid uint64, send func(world.ClientOpcode, world.MovementInfo) error) *worldMovementReporter {
	return &worldMovementReporter{guid: guid, started: time.Now(), send: send}
}

// worldInputFlags maps held keys to movement intent flags. Opposite keys
// cancel, as in the original client.
func worldInputFlags(keys map[window.Key]bool) uint32 {
	flags := uint32(0)
	if keys[window.KeyW] != keys[window.KeyS] {
		if keys[window.KeyW] {
			flags |= world.MovementFlagForward
		} else {
			flags |= world.MovementFlagBackward
		}
	}
	if keys[window.KeyA] != keys[window.KeyD] {
		if keys[window.KeyA] {
			flags |= world.MovementFlagStrafeLeft
		} else {
			flags |= world.MovementFlagStrafeRight
		}
	}
	return flags
}

// worldMovementTransitions mirrors MeoWow's Motion::transitions: only an axis
// that changed produces a packet, and stopping an axis never stops the other.
func worldMovementTransitions(before, after uint32) []world.ClientOpcode {
	var out []world.ClientOpcode
	const longitudinal = world.MovementFlagForward | world.MovementFlagBackward
	const lateral = world.MovementFlagStrafeLeft | world.MovementFlagStrafeRight
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
	return out
}

func (r *worldMovementReporter) report(opcode world.ClientOpcode, info world.MovementInfo) {
	if err := r.send(opcode, info); err != nil {
		r.failures++
		r.lastFailure = err
	}
}

// update is called once per frame after the controller has advanced.
func (r *worldMovementReporter) update(now time.Time, c *worldCameraController) {
	if r == nil || c == nil || r.send == nil {
		return
	}
	flags := worldInputFlags(c.keys)
	// The original client faces the character along the camera while it is
	// moving; the local body turns toward the move direction for presentation
	// only, so reporting it would point the server's integration the wrong way.
	facing := c.bodyYaw
	if flags != 0 {
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
	for _, opcode := range worldMovementTransitions(r.flags, flags) {
		r.report(opcode, info)
		r.lastBeat = now
	}
	r.flags = flags
	if flags != 0 {
		if now.Sub(r.lastBeat) >= worldMoveHeartbeat {
			r.report(world.MoveHeartbeatOpcode, info)
			r.lastBeat = now
		}
		r.lastFacing, r.haveFacing = info.Position.Orientation, true
		return
	}
	if !r.haveFacing {
		r.lastFacing, r.haveFacing = info.Position.Orientation, true
		return
	}
	if math.Abs(float64(worldAngleDelta(r.lastFacing, info.Position.Orientation))) > 0.01 && now.Sub(r.facingSent) >= worldFacingResend {
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
