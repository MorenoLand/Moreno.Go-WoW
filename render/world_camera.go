package render

import (
	"math"

	"github.com/MorenoLand/Moreno.WoW/world"
	"github.com/g3n/engine/camera"
	"github.com/g3n/engine/core"
	"github.com/g3n/engine/math32"
	"github.com/g3n/engine/window"
)

// Movement commands: the binding names of the movement keys. The controller
// is driven by commands, so the keys that produce them come from the key
// bindings and can be rebound.
const (
	cmdForward     = "MOVEFORWARD"
	cmdBackward    = "MOVEBACKWARD"
	cmdTurnLeft    = "TURNLEFT"
	cmdTurnRight   = "TURNRIGHT"
	cmdStrafeLeft  = "STRAFELEFT"
	cmdStrafeRight = "STRAFERIGHT"
	cmdJump        = "JUMP"
	cmdDescend     = "SITORSTAND"
	cmdPitchUp     = "PITCHUP"
	cmdPitchDown   = "PITCHDOWN"
	cmdAutoRun     = "TOGGLEAUTORUN"
	cmdToggleRun   = "TOGGLERUN"
)

// worldMovementCommands lists the commands the controller understands.
var worldMovementCommands = map[string]bool{
	cmdForward: true, cmdBackward: true, cmdTurnLeft: true, cmdTurnRight: true, cmdStrafeLeft: true, cmdStrafeRight: true,
	cmdJump: true, cmdDescend: true, cmdPitchUp: true, cmdPitchDown: true, cmdAutoRun: true, cmdToggleRun: true,
}

// Distances (yards) that decide whether the mover is in water: it swims once
// the surface is this far above its feet and wades while it is shallower.
const worldSwimDepth = 1.6

type worldCameraController struct {
	position    [3]float32
	velocity    [3]float32
	distance    float32
	yaw         float32
	bodyYaw     float32
	pitch       float32
	commands    map[string]bool
	ground      func(float32, float32) (float32, bool)
	floor       func(float32, float32, float32) (float32, bool)
	move        func([3]float32, [3]float32) [3]float32
	cameraTest  func(math32.Vector3, math32.Vector3) math32.Vector3
	liquid      func(x, y, z float32) (surface float32, ok bool)
	jumpQueued  bool
	grounded    bool
	airTime     float64
	landingTime float64
	dragging    bool
	dragRight   bool
	dragLeft    bool
	// leftDown and rightDown are the mouse buttons held.
	leftDown  bool
	rightDown bool
	lastMouseX  float64
	lastMouseY  float64

	// Server-granted state and speeds.
	speeds   [world.SpeedKindCount]float32
	canFly   bool
	rooted   bool
	levitate bool
	hover    bool
	walking  bool
	autoRun  bool
	mounted  bool
	// slowFall is the safe fall flag the server grants, which caps the falling speed.
	slowFall bool

	// A fall from a jump or a ledge is worked out from where and when it began.
	fallActive bool
	fallTime   float64
	fallStartZ float64
	fallV0     float64

	// Movement mode.
	flying        bool
	swimming      bool
	ascending     bool
	descending    bool
	fallLaunch    world.FallInfo
	jumpDirection [2]float32 // horizontal velocity kept while airborne
	fallStart     float64
	clock         float64

	// Events the movement reporter turns into packets.
	jumped       bool
	landed       bool
	tookOff      bool
	touchedDown  bool
	pitchChanged bool
}

func newWorldCameraController(position world.WorldPosition) *worldCameraController {
	c := &worldCameraController{position: [3]float32{position.X, position.Y, position.Z}, distance: 10, yaw: position.Orientation, bodyYaw: position.Orientation, pitch: -15 * float32(math.Pi) / 180, grounded: true, commands: make(map[string]bool)}
	c.speeds = [world.SpeedKindCount]float32{
		world.SpeedKindWalk: world.SpeedWalk, world.SpeedKindRun: world.SpeedRun, world.SpeedKindRunBack: world.SpeedRunBack,
		world.SpeedKindSwim: world.SpeedSwim, world.SpeedKindSwimBack: world.SpeedSwimBack, world.SpeedKindTurn: world.SpeedTurn,
		world.SpeedKindFlight: world.SpeedFlight, world.SpeedKindFlightBack: world.SpeedFlightBack, world.SpeedKindPitch: world.SpeedPitch,
	}
	return c
}

// handleCommand starts or stops a movement command; it reports whether the
// controller used it.
func (c *worldCameraController) handleCommand(command string, down bool) bool {
	if !worldMovementCommands[command] {
		return false
	}
	switch command {
	case cmdJump:
		if down && !c.commands[command] {
			c.jumpQueued = true
		}
	case cmdAutoRun:
		if down && !c.commands[command] {
			c.autoRun = !c.autoRun
		}
	case cmdToggleRun:
		if down && !c.commands[command] {
			c.walking = !c.walking
		}
	case cmdForward, cmdBackward:
		if down {
			c.autoRun = false
		}
	}
	c.commands[command] = down
	return true
}

// handleKey keeps the default key layout for callers that have no key
// bindings: the Wrath defaults.
func (c *worldCameraController) handleKey(key window.Key, down bool) bool {
	command := ""
	switch key {
	case window.KeyW, window.KeyUp:
		command = cmdForward
	case window.KeyS, window.KeyDown:
		command = cmdBackward
	case window.KeyA, window.KeyLeft:
		command = cmdTurnLeft
	case window.KeyD, window.KeyRight:
		command = cmdTurnRight
	case window.KeyQ:
		command = cmdStrafeLeft
	case window.KeyE:
		command = cmdStrafeRight
	case window.KeySpace:
		command = cmdJump
	case window.KeyX:
		command = cmdDescend
	}
	return command != "" && c.handleCommand(command, down)
}

func (c *worldCameraController) setGround(ground func(float32, float32) (float32, bool)) {
	c.ground = ground
}

func (c *worldCameraController) setFloor(floor func(float32, float32, float32) (float32, bool)) {
	c.floor = floor
}

func (c *worldCameraController) setMovement(move func([3]float32, [3]float32) [3]float32) {
	c.move = move
}

func (c *worldCameraController) setCameraTest(test func(math32.Vector3, math32.Vector3) math32.Vector3) {
	c.cameraTest = test
}

// setLiquid installs the water lookup: the height of the water surface at a
// point, when there is water there.
func (c *worldCameraController) setLiquid(liquid func(x, y, z float32) (float32, bool)) {
	c.liquid = liquid
}

func (c *worldCameraController) handleScroll(offset float32) bool {
	if offset == 0 {
		return false
	}
	c.distance = float32(math.Max(2.5, math.Min(30, float64(c.distance)*math.Pow(0.88, float64(offset)))))
	return true
}

func (c *worldCameraController) isMoving() bool {
	return c.velocity[0]*c.velocity[0]+c.velocity[1]*c.velocity[1] > 0.04
}

func (c *worldCameraController) isAirborne() bool {
	return !c.grounded && !c.flying && !c.swimming
}

// motion is the animation the player model should play.
func (c *worldCameraController) motion() uint16 {
	if c.mounted {
		return worldAnimMount
	}
	switch {
	case c.flying:
		if c.isMoving() || c.velocity[2]*c.velocity[2] > 0.04 {
			return worldAnimFlyForward
		}
		return worldAnimFlyIdle
	case c.swimming:
		if c.isMoving() || c.velocity[2]*c.velocity[2] > 0.04 {
			return worldAnimSwim
		}
		return worldAnimSwimIdle
	case !c.grounded:
		switch {
		case c.airTime < 0.2:
			return 37
		case c.velocity[2] > 0.5:
			return 38
		default:
			return 40
		}
	case c.landingTime > 0:
		return 39
	case c.isMoving():
		if c.walking || c.moveFlagsBackward() {
			return worldAnimWalk
		}
		return 5
	}
	return 0
}

// Animation ids of AnimationData.dbc the player model switches between.
const (
	worldAnimWalk       = 4
	worldAnimSwimIdle   = 41
	worldAnimSwim       = 42
	worldAnimFlyForward = 135
	worldAnimFlyIdle    = 193
	worldAnimMount      = 91
)

func (c *worldCameraController) moveFlagsBackward() bool {
	return c.commands[cmdBackward] && !c.commands[cmdForward]
}

func (c *worldCameraController) handleMouse(x, y float64, button window.MouseButton, down bool) bool {
	if button != window.MouseButtonRight && button != window.MouseButtonLeft {
		return false
	}
	if button == window.MouseButtonRight {
		c.rightDown = down
	} else {
		c.leftDown = down
	}
	c.dragging = c.leftDown || c.rightDown
	c.dragRight = c.rightDown
	c.dragLeft = c.leftDown && !c.rightDown
	c.lastMouseX = x
	c.lastMouseY = y
	return true
}

func (c *worldCameraController) handleCursor(x, y float64) bool {
	if !c.dragging {
		return false
	}
	deltaX, deltaY := x-c.lastMouseX, y-c.lastMouseY
	c.lastMouseX, c.lastMouseY = x, y
	c.yaw -= float32(deltaX) * 0.006
	c.pitch = clampWorldPitch(c.pitch - float32(deltaY)*0.004)
	return true
}

// applyForcedChange applies a server-forced speed or flag change.
func (c *worldCameraController) applyForcedChange(change world.ForcedChange) {
	if change.HasSpeed {
		c.speeds[change.Speed] = change.Value
		return
	}
	set := func(target *bool) { *target = change.Set }
	switch change.Flag {
	case world.MovementFlagCanFly:
		set(&c.canFly)
		if !c.canFly && c.flying {
			c.flying, c.ascending, c.descending = false, false, false
			c.touchedDown = true
		}
	case world.MovementFlagRoot:
		set(&c.rooted)
	case world.MovementFlagLevitating:
		set(&c.levitate)
	case world.MovementFlagHover:
		set(&c.hover)
	case world.MovementFlagSafeFall:
		set(&c.slowFall)
	}
}

// knockBack launches the mover as SMSG_MOVE_KNOCK_BACK says: horizontal speed
// along (cos, sin) and an upward speed.
func (c *worldCameraController) knockBack(knock world.KnockBack) {
	c.velocity[0] = knock.CosAngle * knock.HorizontalSpeed
	c.velocity[1] = knock.SinAngle * knock.HorizontalSpeed
	c.velocity[2] = -knock.VerticalSpeed
	c.jumpDirection = [2]float32{c.velocity[0], c.velocity[1]}
	c.grounded = false
	c.airTime = 0
	c.fallStart = c.clock
	c.fallLaunch = world.FallInfo{ZSpeed: knock.VerticalSpeed, SinAngle: knock.SinAngle, CosAngle: knock.CosAngle, XYSpeed: knock.HorizontalSpeed}
}

// teleport places the mover at a position the server chose.
func (c *worldCameraController) teleport(position world.WorldPosition) {
	c.position = [3]float32{position.X, position.Y, position.Z}
	c.velocity = [3]float32{}
	c.fallActive = false
	c.yaw, c.bodyYaw = position.Orientation, position.Orientation
	c.grounded = true
	c.flying, c.swimming, c.ascending, c.descending = false, false, false, false
}

// movementSpeed is the speed of the current mode and direction.
func (c *worldCameraController) movementSpeed(backward bool) float32 {
	switch {
	case c.flying:
		if backward {
			return c.speeds[world.SpeedKindFlightBack]
		}
		return c.speeds[world.SpeedKindFlight]
	case c.swimming:
		if backward {
			return c.speeds[world.SpeedKindSwimBack]
		}
		return c.speeds[world.SpeedKindSwim]
	case backward:
		return c.speeds[world.SpeedKindRunBack]
	case c.walking:
		return c.speeds[world.SpeedKindWalk]
	}
	return c.speeds[world.SpeedKindRun]
}

// flags is the movement flag word for the controller's current state.
func (c *worldCameraController) flags() uint32 {
	flags := worldInputFlags(c.commands)
	// Both mouse buttons together run forward, and the turn keys slide sideways
	// while the right button turns the character (FUN_005FAE70, FUN_005FAFB0).
	if c.leftDown && c.rightDown && flags&(world.MovementFlagForward|world.MovementFlagBackward) == 0 {
		flags |= world.MovementFlagForward
	}
	if c.rightDown {
		if c.commands[cmdTurnLeft] && !c.commands[cmdTurnRight] {
			flags |= world.MovementFlagStrafeLeft
		}
		if c.commands[cmdTurnRight] && !c.commands[cmdTurnLeft] {
			flags |= world.MovementFlagStrafeRight
		}
	}
	if c.autoRun && flags&(world.MovementFlagForward|world.MovementFlagBackward) == 0 {
		flags |= world.MovementFlagForward
	}
	if c.rooted {
		flags &^= world.MovementFlagForward | world.MovementFlagBackward | world.MovementFlagStrafeLeft | world.MovementFlagStrafeRight
	}
	if c.commands[cmdTurnLeft] != c.commands[cmdTurnRight] && !c.rightDown {
		if c.commands[cmdTurnLeft] {
			flags |= world.MovementFlagTurnLeft
		} else {
			flags |= world.MovementFlagTurnRight
		}
	}
	if c.walking {
		flags |= world.MovementFlagWalking
	}
	if c.canFly {
		flags |= world.MovementFlagCanFly
	}
	if c.rooted {
		flags |= world.MovementFlagRoot
	}
	if c.hover {
		flags |= world.MovementFlagHover
	}
	if c.levitate {
		flags |= world.MovementFlagLevitating
	}
	switch {
	case c.flying:
		flags |= world.MovementFlagFlying
	case c.swimming:
		flags |= world.MovementFlagSwimming
	case !c.grounded:
		flags |= world.MovementFlagFalling
	}
	if c.ascending {
		flags |= world.MovementFlagAscending
	}
	if c.descending {
		flags |= world.MovementFlagDescending
	}
	return flags
}

// takeEvents returns and clears the one-shot events since the last call.
func (c *worldCameraController) takeEvents() (jumped, landed, tookOff, touchedDown bool) {
	jumped, landed, tookOff, touchedDown = c.jumped, c.landed, c.tookOff, c.touchedDown
	c.jumped, c.landed, c.tookOff, c.touchedDown = false, false, false, false
	return
}

func (c *worldCameraController) update(elapsed float64, cam *camera.Camera, player *core.Node) bool {
	if elapsed <= 0 {
		return false
	}
	delta := float32(math.Min(elapsed, 0.1))
	c.clock += float64(delta)
	c.airTime += float64(delta)
	if c.grounded {
		c.airTime = 0
	}
	if c.landingTime > 0 {
		c.landingTime -= float64(delta)
	}
	turn := c.speeds[world.SpeedKindTurn] * delta
	if c.commands[cmdTurnLeft] && !c.rooted && !c.rightDown {
		c.yaw += turn
		c.bodyYaw = c.yaw
	}
	if c.commands[cmdTurnRight] && !c.rooted && !c.rightDown {
		c.yaw -= turn
		c.bodyYaw = c.yaw
	}
	pitchTurn := c.speeds[world.SpeedKindPitch] * delta
	if c.commands[cmdPitchUp] {
		c.pitch = clampWorldPitch(c.pitch + pitchTurn)
	}
	if c.commands[cmdPitchDown] {
		c.pitch = clampWorldPitch(c.pitch - pitchTurn)
	}
	forward := [2]float32{float32(math.Cos(float64(c.yaw))), float32(math.Sin(float64(c.yaw)))}
	strafe := [2]float32{-forward[1], forward[0]}
	flags := c.flags()
	longitudinal := flags & (world.MovementFlagForward | world.MovementFlagBackward)
	backward := longitudinal == world.MovementFlagBackward
	move := [2]float32{}
	if !c.rooted {
		if flags&world.MovementFlagForward != 0 {
			move[0] += forward[0]
			move[1] += forward[1]
		}
		if flags&world.MovementFlagBackward != 0 {
			move[0] -= forward[0]
			move[1] -= forward[1]
		}
		if flags&world.MovementFlagStrafeLeft != 0 {
			move[0] += strafe[0]
			move[1] += strafe[1]
		}
		if flags&world.MovementFlagStrafeRight != 0 {
			move[0] -= strafe[0]
			move[1] -= strafe[1]
		}
	}
	moveLength := float32(math.Hypot(float64(move[0]), float64(move[1])))
	targetSpeed := c.movementSpeed(backward)
	if moveLength > 0 {
		move[0], move[1] = move[0]/moveLength*targetSpeed, move[1]/moveLength*targetSpeed
	}
	// The body faces where the camera looks: all the time the right button is
	// held (the mouse turns the character), and from the moment the character
	// moves; dragging the camera round with the left button alone leaves it be.
	// Walking is relative to that facing, so the character never turns to face
	// the way it is going (InputControl.cpp, FUN_005FA170 and FUN_005FBBC0).
	if c.dragRight || (moveLength > 0 && !c.dragLeft) {
		c.bodyYaw = c.yaw
	}

	wasGrounded := c.grounded
	wasFlying, wasSwimming := c.flying, c.swimming
	c.updateWater()

	switch {
	case c.flying || c.swimming:
		c.updateFreeMovement(delta, moveLength, move, targetSpeed, flags)
	default:
		c.updateWalking(delta, moveLength, move, targetSpeed)
	}
	c.tookOff = c.tookOff || (!wasFlying && c.flying)
	c.touchedDown = c.touchedDown || (wasFlying && !c.flying)
	if wasSwimming != c.swimming {
		c.pitchChanged = true
	}
	if !wasGrounded && c.grounded && !c.flying && !c.swimming {
		c.landed = true
		c.landingTime = 0.16
	}
	if player != nil {
		player.SetPosition(c.position[0], c.position[1], c.position[2])
		player.SetRotation(0, 0, c.bodyYaw)
	}
	cosPitch := float32(math.Cos(float64(c.pitch)))
	sinPitch := float32(math.Sin(float64(c.pitch)))
	pivot := math32.NewVector3(c.position[0], c.position[1], c.position[2]+1.6)
	forward3 := math32.NewVector3(forward[0]*cosPitch, forward[1]*cosPitch, sinPitch)
	eye := math32.NewVector3(c.position[0]-forward3.X*c.distance, c.position[1]-forward3.Y*c.distance, c.position[2]+1.6-forward3.Z*c.distance)
	if c.cameraTest != nil {
		adjusted := c.cameraTest(*pivot, *eye)
		eye = &adjusted
	}
	cam.SetPositionVec(eye)
	cam.LookAt(pivot, math32.NewVector3(0, 0, 1))
	return true
}

// updateWater decides whether the mover is swimming: the water surface lies
// well above its feet. A flying mover stays in the air.
func (c *worldCameraController) updateWater() {
	if c.liquid == nil || c.flying {
		c.swimming = false
		return
	}
	surface, ok := c.liquid(c.position[0], c.position[1], c.position[2])
	c.swimming = ok && surface-c.position[2] > worldSwimDepth
}

// updateWalking is the ground and air movement: running on the floor, a jump
// that keeps its horizontal velocity, and the fall.
func (c *worldCameraController) updateWalking(delta float32, moveLength float32, move [2]float32, targetSpeed float32) {
	blend := float32(1 - math.Exp(-12*float64(delta)))
	if c.grounded {
		if moveLength > 0 {
			c.velocity[0] += (move[0] - c.velocity[0]) * blend
			c.velocity[1] += (move[1] - c.velocity[1]) * blend
		} else {
			friction := float32(math.Exp(-14 * float64(delta)))
			c.velocity[0] *= friction
			c.velocity[1] *= friction
		}
	} else {
		// In the air the horizontal velocity stays what it was at take-off.
		c.velocity[0], c.velocity[1] = c.jumpDirection[0], c.jumpDirection[1]
	}
	from := c.position
	to := from
	to[0] += c.velocity[0] * delta
	to[1] += c.velocity[1] * delta
	if c.move != nil {
		to = c.move(from, to)
	}
	c.position[0], c.position[1] = to[0], to[1]
	grounded := false
	groundHeight := float32(0)
	if c.floor != nil {
		groundHeight, grounded = c.floor(c.position[0], c.position[1], c.position[2])
		if c.fallActive {
			// Airborne until the fall reaches the ground below.
			grounded = false
		}
		if grounded && c.velocity[2] <= 0 && math.Abs(float64(c.position[2]-groundHeight)) <= worldMaxGroundSnap {
			c.position[2] = groundHeight
			c.velocity[2] = 0
			c.fallActive = false
		}
	} else if c.ground != nil {
		groundHeight, grounded = c.ground(c.position[0], c.position[1])
		if grounded && c.velocity[2] == 0 && math.Abs(float64(c.position[2]-groundHeight)) <= worldMaxGroundSnap {
			c.position[2] = groundHeight
		}
	}
	if c.jumpQueued && !c.rooted {
		switch {
		case c.canFly:
			// A climb with flight allowed is a take-off.
			c.flying, c.ascending, c.descending = true, true, false
			c.velocity = [3]float32{}
			c.jumpQueued = false
			return
		case grounded:
			c.velocity[2] = world.JumpInitialSpeed
			c.fallActive, c.fallTime, c.fallStartZ, c.fallV0 = true, 0, float64(c.position[2]), -world.JumpInitialSpeed
			c.jumpDirection = [2]float32{move[0], move[1]}
			if moveLength == 0 {
				c.jumpDirection = [2]float32{}
			}
			c.fallLaunch = world.FallInfo{ZSpeed: -world.JumpInitialSpeed, SinAngle: float32(math.Sin(float64(c.bodyYaw))), CosAngle: float32(math.Cos(float64(c.bodyYaw))), XYSpeed: float32(math.Hypot(float64(c.jumpDirection[0]), float64(c.jumpDirection[1])))}
			c.fallStart = c.clock
			grounded = false
			c.airTime = 0
			c.jumped = true
		}
	}
	c.jumpQueued = false
	if !c.levitate {
		if (!grounded || c.velocity[2] > 0) && !c.fallActive {
			// Off a ledge, or thrown: a fall that begins at the speed it has now.
			c.fallActive, c.fallTime, c.fallStartZ, c.fallV0 = true, 0, float64(c.position[2]), -float64(c.velocity[2])
		}
		if c.fallActive && delta > 0 {
			c.fallTime += float64(delta)
			z := float32(c.fallStartZ - world.FallDistance(c.fallTime, c.slowFall, c.fallV0))
			c.velocity[2] = (z - c.position[2]) / delta
			c.position[2] = z
		}
	} else {
		c.fallActive = false
		c.position[2] += c.velocity[2] * delta
	}
	if c.floor != nil {
		if height, ok := c.floor(c.position[0], c.position[1], c.position[2]); ok && c.position[2] <= height {
			c.position[2] = height
			c.velocity[2] = 0
			c.fallActive = false
			grounded = true
		}
	} else if c.ground != nil {
		if height, ok := c.ground(c.position[0], c.position[1]); ok && c.position[2] <= height {
			c.position[2] = height
			c.velocity[2] = 0
			c.fallActive = false
			grounded = true
		}
	}
	if !grounded && c.grounded && !c.jumped {
		// Walked off an edge: a fall starts, with no horizontal take-off speed
		// beyond the current run.
		c.jumpDirection = [2]float32{c.velocity[0], c.velocity[1]}
		c.fallStart = c.clock
		c.fallLaunch = world.FallInfo{SinAngle: float32(math.Sin(float64(c.bodyYaw))), CosAngle: float32(math.Cos(float64(c.bodyYaw)))}
	}
	c.grounded = grounded
}

// updateFreeMovement is flying and swimming: the mover travels along the
// direction it looks, up and down with the ascend and descend commands.
func (c *worldCameraController) updateFreeMovement(delta float32, moveLength float32, move [2]float32, targetSpeed float32, flags uint32) {
	c.grounded = false
	c.fallActive = false
	c.ascending = c.commands[cmdJump] && !c.commands[cmdDescend]
	c.descending = c.commands[cmdDescend] && !c.commands[cmdJump]
	if c.jumpQueued {
		c.jumpQueued = false
	}
	blend := float32(1 - math.Exp(-10*float64(delta)))
	target := [3]float32{}
	if moveLength > 0 && !c.rooted {
		// Forward and back follow the camera pitch; strafing stays level.
		cosPitch := float32(math.Cos(float64(c.pitch)))
		sinPitch := float32(math.Sin(float64(c.pitch)))
		forwardOnly := flags&(world.MovementFlagForward|world.MovementFlagBackward) != 0
		target[0], target[1] = move[0], move[1]
		if forwardOnly {
			sign := float32(1)
			if flags&world.MovementFlagBackward != 0 && flags&world.MovementFlagForward == 0 {
				sign = -1
			}
			target[0] *= cosPitch
			target[1] *= cosPitch
			target[2] = sign * sinPitch * targetSpeed
		}
	}
	if !c.rooted {
		vertical := targetSpeed
		if c.ascending {
			target[2] += vertical
		}
		if c.descending {
			target[2] -= vertical
		}
	}
	for axis := 0; axis < 3; axis++ {
		c.velocity[axis] += (target[axis] - c.velocity[axis]) * blend
	}
	from := c.position
	to := from
	to[0] += c.velocity[0] * delta
	to[1] += c.velocity[1] * delta
	if c.move != nil {
		to = c.move(from, to)
	}
	c.position[0], c.position[1] = to[0], to[1]
	c.position[2] += c.velocity[2] * delta
	// The ground and the water surface limit the travel.
	if c.ground != nil {
		// A dive cannot pass through terrain from above. A mover already below the
		// surface (inside a mountain, in a city gate or a cave) is not pushed up.
		before, beforeKnown := c.ground(from[0], from[1])
		underground := beforeKnown && from[2] < before-1
		if height, ok := c.ground(c.position[0], c.position[1]); ok && !underground && c.position[2] < height {
			c.position[2] = height
			if c.velocity[2] < 0 {
				c.velocity[2] = 0
			}
		}
	}
	if c.floor != nil {
		if height, ok := c.floor(c.position[0], c.position[1], c.position[2]); ok && c.position[2] <= height {
			c.position[2] = height
			if c.velocity[2] < 0 {
				c.velocity[2] = 0
			}
			if c.flying && (c.descending || !c.canFly) {
				// Touching down ends the flight.
				c.flying, c.ascending, c.descending = false, false, false
				c.grounded = true
			}
		}
	}
	if c.swimming && c.liquid != nil {
		if surface, ok := c.liquid(c.position[0], c.position[1], c.position[2]); ok && c.position[2] > surface-0.6 && c.velocity[2] > 0 {
			c.position[2] = surface - 0.6
			c.velocity[2] = 0
		}
	}
}

func worldAngleDelta(from, to float32) float32 {
	delta := to - from
	for delta > math.Pi {
		delta -= 2 * math.Pi
	}
	for delta < -math.Pi {
		delta += 2 * math.Pi
	}
	return delta
}

func approachWorldAngle(current, target, amount float32) float32 {
	delta := worldAngleDelta(current, target)
	if math.Abs(float64(delta)) <= float64(amount) {
		return target
	}
	if delta < 0 {
		return current - amount
	}
	return current + amount
}

func clampWorldPitch(pitch float32) float32 {
	const limit = float32(1.2)
	if pitch < -limit {
		return -limit
	}
	if pitch > limit {
		return limit
	}
	return pitch
}

func worldToScreen(cam *camera.Camera, point math32.Vector3, width, height float32) (math32.Vector3, bool) {
	if cam == nil || width <= 0 || height <= 0 {
		return math32.Vector3{}, false
	}
	projected := cam.Project(point.Clone())
	if projected.Z < -1 || projected.Z > 1 || math.IsNaN(float64(projected.X)) || math.IsNaN(float64(projected.Y)) || math.IsNaN(float64(projected.Z)) {
		return math32.Vector3{}, false
	}
	return *math32.NewVector3((projected.X+1)*width*0.5, (1-projected.Y)*height*0.5, projected.Z), true
}

// syncFromUpdate takes the speeds and flight permission the server replicates
// for the player in its object updates.
func (c *worldCameraController) syncFromUpdate(movement world.UpdateMovement) {
	order := [9]world.SpeedKind{world.SpeedKindWalk, world.SpeedKindRun, world.SpeedKindRunBack, world.SpeedKindSwim, world.SpeedKindSwimBack,
		world.SpeedKindFlight, world.SpeedKindFlightBack, world.SpeedKindTurn, world.SpeedKindPitch}
	for index, kind := range order {
		if speed := movement.Speeds[index]; speed > 0.1 && speed < 100 {
			c.speeds[kind] = speed
		}
	}
	if movement.HasPosition {
		canFly := movement.MovementFlags&world.MovementFlagCanFly != 0
		if canFly != c.canFly {
			c.canFly = canFly
			if !canFly && c.flying {
				c.flying, c.ascending, c.descending = false, false, false
				c.touchedDown = true
			}
		}
		c.hover = movement.MovementFlags&world.MovementFlagHover != 0
		c.rooted = movement.MovementFlags&world.MovementFlagRoot != 0
	}
}
