package render

import (
	"testing"

	"github.com/g3n/engine/camera"
	"github.com/g3n/engine/math32"
)

func TestWorldToScreenProjectsCameraTarget(t *testing.T) {
	cam := camera.New(1)
	cam.SetFov(90)
	cam.SetPosition(0, 0, 10)
	cam.LookAt(math32.NewVector3(0, 0, 0), math32.NewVector3(0, 1, 0))
	projected, ok := worldToScreen(cam, *math32.NewVector3(0, 0, 0), 100, 100)
	if !ok {
		t.Fatal("camera target was not projected")
	}
	if projected.X < 49.9 || projected.X > 50.1 || projected.Y < 49.9 || projected.Y > 50.1 {
		t.Fatalf("projected target=%v", projected)
	}
}
