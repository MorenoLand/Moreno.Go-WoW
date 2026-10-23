package render

import (
	"math"

	"github.com/MorenoLand/Moreno.WoW/ui"
	"github.com/MorenoLand/Moreno.WoW/world"
	"github.com/g3n/engine/core"
)

// worldMountRig is a mount model and where its rider sits. The mount model is
// a creature model; its seat is attachment 0, which the model loader keeps as
// the "stand" point.
type worldMountRig struct {
	displayID uint32
	root      *core.Node
	// seat is the rider's position in the mount's own frame (x forward, y left,
	// z up), before the mount's facing is applied.
	seat    [3]float32
	hasSeat bool
	// attachment is the model attachment the rider sits on (id 0) and the
	// ground offset the model was raised by.
	attachmentBone     uint16
	attachmentPosition [3]float32
	groundOffset       float32
}

// buildWorldMount creates the mount model for a creature display id.
func buildWorldMount(loader *ui.Loader, tables *worldCreatureTables, displayID uint32) (*worldMountRig, error) {
	definition, err := tables.definition(loader, displayID, 0)
	if err != nil {
		return nil, err
	}
	model, err := buildWorldCreatureModel(loader, definition)
	if err != nil {
		return nil, err
	}
	displayScale, modelScale := definition.displayScale, definition.modelScale
	if displayScale <= 0 {
		displayScale = 1
	}
	if modelScale <= 0 {
		modelScale = 1
	}
	scale := displayScale * modelScale
	rig := &worldMountRig{displayID: displayID, root: core.NewNode()}
	rig.root.SetScale(scale, scale, scale)
	model.SetRotation(float32(math.Pi/2), 0, 0)
	if info, ok := model.UserData().(glueModelInfo); ok {
		rig.root.SetUserData(info)
		groundOffset := float32(0)
		if info.modelBottom < 0 {
			groundOffset = -info.modelBottom
		}
		model.SetPosition(0, 0, groundOffset)
		rig.groundOffset = groundOffset
	}
	if data, readErr := loader.ReadFile(definition.path); readErr == nil {
		if parsed, parseErr := parseM2(data); parseErr == nil {
			for _, attachment := range parsed.attachments {
				if attachment.id == 0 {
					rig.attachmentBone, rig.attachmentPosition, rig.hasSeat = attachment.bone, attachment.position, true
					rig.seat = [3]float32{attachment.position[0], attachment.position[1], attachment.position[2] + groundOffsetOf(model)}
					break
				}
			}
		}
	}
	rig.root.Add(model)
	return rig, nil
}

// place puts the mount at a ground position and facing and returns where its
// rider's feet go.
func (rig *worldMountRig) place(position [3]float32, yaw float32) [3]float32 {
	rig.root.SetPosition(position[0], position[1], position[2])
	rig.root.SetRotation(0, 0, yaw)
	scale := rig.root.Scale().X
	posed := rig.seatPosition()
	seat := [3]float32{posed[0] * scale, posed[1] * scale, posed[2] * scale}
	if !rig.hasSeat {
		seat[2] = 1.2
	}
	sin, cos := float32(math.Sin(float64(yaw))), float32(math.Cos(float64(yaw)))
	return [3]float32{
		position[0] + seat[0]*cos - seat[1]*sin,
		position[1] + seat[0]*sin + seat[1]*cos,
		position[2] + seat[2],
	}
}

// motion is the animation a mount plays for the way it is moving.
func worldMountMotion(flying, moving bool, hasFlight bool) uint16 {
	switch {
	case flying && moving:
		return worldAnimFlyForward
	case flying && hasFlight:
		return worldAnimFlyIdle
	case moving:
		return 5
	}
	return 0
}

func (rig *worldMountRig) update(elapsed float64, motion uint16, audio func(uint32)) {
	info, ok := rig.root.UserData().(glueModelInfo)
	if !ok {
		return
	}
	if info.animation != nil {
		info.animation.SetMotion(motion)
		for _, soundID := range info.animation.Update(elapsed) {
			if audio != nil {
				audio(uint32(soundID))
			}
		}
	}
	if info.particles != nil {
		info.particles.Update(elapsed)
	}
}

func (rig *worldMountRig) dispose(scene *core.Node) {
	if rig == nil || rig.root == nil {
		return
	}
	scene.Remove(rig.root)
	rig.root.Dispose()
}

// syncWorldPlayerMount makes the player's mount match the display id the
// server replicates, and reports whether the player is mounted.
func syncWorldPlayerMount(scene *core.Node, loader *ui.Loader, tables *worldCreatureTables, rig **worldMountRig, fields map[uint16]uint32) bool {
	displayID := fields[world.UnitMountDisplayIDField]
	if displayID == 0 {
		if *rig != nil {
			(*rig).dispose(scene)
			*rig = nil
		}
		return false
	}
	if *rig != nil && (*rig).displayID == displayID {
		return true
	}
	if *rig != nil {
		(*rig).dispose(scene)
		*rig = nil
	}
	mount, err := buildWorldMount(loader, tables, displayID)
	if err != nil {
		return false
	}
	scene.Add(mount.root)
	*rig = mount
	return true
}

func groundOffsetOf(model *core.Node) float32 {
	if info, ok := model.UserData().(glueModelInfo); ok && info.modelBottom < 0 {
		return -info.modelBottom
	}
	return 0
}

// seatPosition is the rider's seat in the mount's frame: the model attachment
// posed by its bone at the current animation time (the bone matrix maps the
// rest position the attachment is stated in to where it is now).
func (rig *worldMountRig) seatPosition() [3]float32 {
	if !rig.hasSeat {
		return rig.seat
	}
	position := rig.attachmentPosition
	if info, ok := rig.root.UserData().(glueModelInfo); ok && info.animation != nil && int(rig.attachmentBone) < len(info.animation.boneMatrices) {
		matrix := info.animation.boneMatrices[rig.attachmentBone]
		posed := [3]float32{}
		for row := 0; row < 3; row++ {
			posed[row] = matrix.point[row*4]*position[0] + matrix.point[row*4+1]*position[1] + matrix.point[row*4+2]*position[2] + matrix.point[row*4+3]
		}
		position = posed
	}
	return [3]float32{position[0], position[1], position[2] + rig.groundOffset}
}
