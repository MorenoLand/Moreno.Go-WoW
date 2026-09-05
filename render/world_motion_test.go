package render

import (
	"math"
	"testing"

	"github.com/MorenoLand/Moreno.WoW/world"
	"github.com/g3n/engine/camera"
	"github.com/g3n/engine/core"
	"github.com/g3n/engine/math32"
	"github.com/g3n/engine/window"
)

func TestWorldCameraSnapsInitialPositionToFloor(t *testing.T) {
	controller := newWorldCameraController(world.WorldPosition{X: 1, Y: 2, Z: 1})
	controller.setFloor(func(float32, float32, float32) (float32, bool) { return 0, true })
	controller.update(1.0/60.0, camera.New(1), nil)
	if controller.position[2] != 0 || controller.velocity[2] != 0 {
		t.Fatalf("position=%v velocity=%v", controller.position, controller.velocity)
	}
}

func TestWorldCameraKeepsAirbornePositionAboveFloor(t *testing.T) {
	controller := newWorldCameraController(world.WorldPosition{Z: 0})
	controller.setFloor(func(float32, float32, float32) (float32, bool) { return 0, true })
	controller.handleKey(window.KeySpace, true)
	controller.update(1.0/60.0, camera.New(1), nil)
	controller.handleKey(window.KeySpace, false)
	controller.update(1.0/60.0, camera.New(1), nil)
	if controller.position[2] <= 0 || controller.velocity[2] <= 0 {
		t.Fatalf("position=%v velocity=%v", controller.position, controller.velocity)
	}
}

func TestWrapWorldModelKeepsStandAttachmentGrounded(t *testing.T) {
	model := core.NewNode()
	model.SetUserData(glueModelInfo{hasStand: true, standPosition: *math32.NewVector3(1, 2, 3), modelBottom: -10})
	root := wrapWorldModel(model, world.WorldPosition{}, 1)
	position := root.Children()[0].Position()
	if position.X != -1 || position.Y != 3 || position.Z != -2 {
		t.Fatalf("model position=%v", position)
	}
}

func TestWorldM2RotationMatchesPlacementAxes(t *testing.T) {
	rotation := worldM2Rotation([3]float32{0, -90, 0})
	got := rotateWorldM2Vector(rotation, [3]float32{1, 0, 0})
	if math.Abs(float64(got[0])) > 0.0001 || math.Abs(float64(got[1]-1)) > 0.0001 || math.Abs(float64(got[2])) > 0.0001 {
		t.Fatalf("rotated point=%v", got)
	}
}

func TestSceneCharacterTransformPreservesNormalizedPosition(t *testing.T) {
	background := glueModelInfo{standPosition: *math32.NewVector3(10, 20, 30), modelScale: 4}
	character := glueModelInfo{modelScale: 2, modelBottom: -1.5}
	scale, position := sceneCharacterTransform(background, character, *math32.NewVector3(-1, 2, 3))
	if scale != 4 || position.X != 8 || position.Y != 27 || position.Z != 36 {
		t.Fatalf("scale=%v position=%v", scale, position)
	}
}

func TestWorldEntityMotionUsesMovementState(t *testing.T) {
	for _, test := range []struct {
		name  string
		flags uint32
		want  uint16
	}{
		{name: "run", want: 5},
		{name: "walk", flags: world.MovementFlagWalking, want: 4},
		{name: "backward", flags: world.MovementFlagBackward, want: 13},
		{name: "left", flags: world.MovementFlagStrafeLeft, want: 11},
		{name: "right", flags: world.MovementFlagStrafeRight, want: 12},
		{name: "swim", flags: world.MovementFlagSwimming, want: 42},
		{name: "swim backward", flags: world.MovementFlagSwimming | world.MovementFlagBackward, want: 45},
		{name: "fly", flags: world.MovementFlagFlying, want: 135},
	} {
		t.Run(test.name, func(t *testing.T) {
			entity := &worldEntity{movement: world.UpdateMovement{MovementFlags: test.flags}}
			if got := worldEntityMotion(entity); got != test.want {
				t.Fatalf("motion=%d want=%d", got, test.want)
			}
		})
	}
}

func TestWorldMonsterMovePreservesActivePositionUntilCompletion(t *testing.T) {
	entities := map[uint64]*worldEntity{7: {
		guid:         7,
		hasPosition:  true,
		movement:     world.UpdateMovement{Position: world.WorldPosition{X: 10, Y: 2, Z: 3, Orientation: 0.25}},
		path:         []world.WorldPosition{{X: 10, Y: 2, Z: 3}, {X: 11, Y: 2, Z: 3}},
		pathLengths:  []float64{1},
		pathTotal:    1,
		pathDuration: 2,
	}}
	applyWorldMonsterMove(entities, world.MonsterMove{GUID: 7, From: world.WorldPosition{X: 100, Y: 100, Z: 100}, To: world.WorldPosition{X: 14, Y: 2, Z: 3}, Duration: 1000, Facing: world.MoveFacing{Kind: 4, Angle: 1.5}})
	entity := entities[7]
	if entity.movement.Position.X != 10 || entity.movement.Position.Orientation != 0.25 {
		t.Fatalf("start=%v", entity.movement.Position)
	}
	advanceWorldEntities(entities, 0.5, nil)
	if math.Abs(float64(entity.movement.Position.Orientation-1.5)) < 0.001 {
		t.Fatalf("arrival facing applied during move: %v", entity.movement.Position.Orientation)
	}
	advanceWorldEntities(entities, 0.6, nil)
	if math.Abs(float64(entity.movement.Position.X-14)) > 0.001 || math.Abs(float64(entity.movement.Position.Orientation-1.5)) > 0.001 {
		t.Fatalf("end=%v", entity.movement.Position)
	}
}
