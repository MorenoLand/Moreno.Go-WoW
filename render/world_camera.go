package render

import (
	"math"

	"github.com/MorenoLand/Moreno.WoW/world"
	"github.com/g3n/engine/camera"
	"github.com/g3n/engine/core"
	"github.com/g3n/engine/math32"
	"github.com/g3n/engine/window"
)

type worldCameraController struct {
	position   [3]float32
	velocity   [3]float32
	distance   float32
	yaw        float32
	pitch      float32
	keys       map[window.Key]bool
	ground     func(float32, float32) (float32, bool)
	floor      func(float32, float32, float32) (float32, bool)
	move       func([3]float32, [3]float32) [3]float32
	cameraTest func(math32.Vector3, math32.Vector3) math32.Vector3
	jumpQueued bool
	dragging   bool
	lastMouseX float64
	lastMouseY float64
}

func newWorldCameraController(position world.WorldPosition) *worldCameraController {
	return &worldCameraController{position: [3]float32{position.X, position.Y, position.Z}, distance: 10, yaw: position.Orientation, pitch: -15 * float32(math.Pi) / 180, keys: make(map[window.Key]bool)}
}

func (c *worldCameraController) handleKey(key window.Key, down bool) bool {
	switch key {
	case window.KeyW, window.KeyA, window.KeyS, window.KeyD, window.KeyUp, window.KeyDown, window.KeyLeft, window.KeyRight, window.KeyQ, window.KeyE, window.KeySpace:
		if key == window.KeySpace && down && !c.keys[key] {
			c.jumpQueued = true
		}
		c.keys[key] = down
		return true
	default:
		return false
	}
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
	return math.Abs(float64(c.velocity[2])) > 0.25
}

func (c *worldCameraController) handleMouse(x, y float64, button window.MouseButton, down bool) bool {
	if button != window.MouseButtonRight {
		return false
	}
	c.dragging = down
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

func (c *worldCameraController) update(elapsed float64, cam *camera.Camera, player *core.Node) bool {
	if elapsed <= 0 {
		return false
	}
	delta := float32(math.Min(elapsed, 0.1))
	turn := float32(1.8) * delta
	if c.keys[window.KeyLeft] {
		c.yaw += turn
	}
	if c.keys[window.KeyRight] {
		c.yaw -= turn
	}
	if c.keys[window.KeyUp] {
		c.pitch = clampWorldPitch(c.pitch + turn)
	}
	if c.keys[window.KeyDown] {
		c.pitch = clampWorldPitch(c.pitch - turn)
	}
	forward := [2]float32{float32(math.Cos(float64(c.yaw))), float32(math.Sin(float64(c.yaw)))}
	strafe := [2]float32{-forward[1], forward[0]}
	move := [2]float32{}
	if c.keys[window.KeyW] {
		move[0] += forward[0]
		move[1] += forward[1]
	}
	if c.keys[window.KeyS] {
		move[0] -= forward[0]
		move[1] -= forward[1]
	}
	if c.keys[window.KeyA] {
		move[0] += strafe[0]
		move[1] += strafe[1]
	}
	if c.keys[window.KeyD] {
		move[0] -= strafe[0]
		move[1] -= strafe[1]
	}
	moveLength := float32(math.Hypot(float64(move[0]), float64(move[1])))
	targetSpeed := float32(7)
	if moveLength > 0 {
		move[0], move[1] = move[0]/moveLength*targetSpeed, move[1]/moveLength*targetSpeed
	}
	blend := float32(1 - math.Exp(-12*float64(delta)))
	if moveLength > 0 {
		c.velocity[0] += (move[0] - c.velocity[0]) * blend
		c.velocity[1] += (move[1] - c.velocity[1]) * blend
	} else {
		friction := float32(math.Exp(-14 * float64(delta)))
		c.velocity[0] *= friction
		c.velocity[1] *= friction
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
	} else if c.ground != nil {
		groundHeight, grounded = c.ground(c.position[0], c.position[1])
		if grounded && c.position[2] <= groundHeight+0.05 && c.velocity[2] <= 0 {
			c.position[2] = groundHeight
			c.velocity[2] = 0
		}
	}
	if c.jumpQueued && grounded {
		c.velocity[2] = 8
		grounded = false
	}
	c.jumpQueued = false
	c.velocity[2] -= 24 * delta
	c.position[2] += c.velocity[2] * delta
	if c.floor != nil {
		if height, ok := c.floor(c.position[0], c.position[1], c.position[2]); ok && c.position[2] <= height {
			c.position[2] = height
			c.velocity[2] = 0
		}
	} else if c.ground != nil {
		if height, ok := c.ground(c.position[0], c.position[1]); ok && c.position[2] <= height {
			c.position[2] = height
			c.velocity[2] = 0
		}
	}
	if c.keys[window.KeyQ] {
		c.position[2] += targetSpeed * delta
		c.velocity[2] = 0
	}
	if c.keys[window.KeyE] {
		c.position[2] -= targetSpeed * delta
		c.velocity[2] = 0
	}
	if player != nil {
		player.SetPosition(c.position[0], c.position[1], c.position[2])
		player.SetRotation(0, 0, c.yaw)
	}
	pivot := math32.NewVector3(c.position[0], c.position[1], c.position[2]+1.6)
	cosPitch := float32(math.Cos(float64(c.pitch)))
	sinPitch := float32(math.Sin(float64(c.pitch)))
	forward3 := math32.NewVector3(forward[0]*cosPitch, forward[1]*cosPitch, sinPitch)
	eye := math32.NewVector3(c.position[0]-forward3.X*c.distance, c.position[1]-forward3.Y*c.distance, c.position[2]+1.6-forward3.Z*c.distance)
	if c.cameraTest != nil {
		adjusted := c.cameraTest(*pivot, *eye)
		eye = &adjusted
	}
	cam.SetPositionVec(eye)
	target := pivot
	cam.LookAt(target, math32.NewVector3(0, 0, 1))
	return true
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
