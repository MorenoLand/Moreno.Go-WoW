package render

import (
	"fmt"

	"github.com/MorenoLand/Moreno.WoW/ui"
	"github.com/g3n/engine/core"
	"github.com/g3n/engine/graphic"
	"github.com/g3n/engine/material"
	"github.com/g3n/engine/math32"
)

// glueSceneLayout is kept on an invisible child of the scene root. It records,
// per loaded Model frame, the state last applied to its node, so the scene can
// follow the UI's Model frames in place (position, facing, scale, alpha, light,
// sequence) instead of being rebuilt, and the frame height the camera covers.
type glueSceneLayout struct {
	units         []*glueSceneUnit
	frameFraction float32
	aspect        float32
}

type glueSceneUnit struct {
	slot    int // index in the UI's visible model list
	node    *core.Node
	base    float32 // the model's own scale before the frame's scale
	applied ui.GlueSceneModel
}

func glueSceneLayoutOf(root *core.Node) *glueSceneLayout {
	if root == nil {
		return nil
	}
	for _, child := range root.Children() {
		if layout, ok := child.GetNode().UserData().(*glueSceneLayout); ok {
			return layout
		}
	}
	return nil
}

// glueModelAspectFactor is the aspect-dependent part of a plain Model frame's
// scale in the original (Wow.exe FUN_0095fba0): the frame's effective scale
// times 1/sqrt(1+a^2) (FUN_0047c080, a = display aspect) times the constant
// 5/3 at 0xa0b634, which makes it 1 at 4:3.
func glueModelAspectFactor(aspect float32) float32 {
	if aspect <= 0 {
		return 1
	}
	return float32(5.0/3.0) / math32.Sqrt(1+aspect*aspect)
}

// applyGlueSceneAspect refreshes the plain Model frames' scale for the window aspect.
func applyGlueSceneAspect(root *core.Node, aspect float32) {
	layout := glueSceneLayoutOf(root)
	if layout == nil || layout.aspect == aspect {
		return
	}
	layout.aspect = aspect
	for _, unit := range layout.units {
		unit.applyScale(aspect)
	}
}

// glueSceneFrameFraction is the model frame height relative to the 768 unit UI
// height, or 1 when unknown.
func glueSceneFrameFraction(root *core.Node) float32 {
	if layout := glueSceneLayoutOf(root); layout != nil && layout.frameFraction > 0 && !layout.usesViewports() {
		return layout.frameFraction
	}
	return 1
}

func (unit *glueSceneUnit) applyScale(aspect float32) {
	scaleX, scaleY, scaleZ := glueSceneScale(unit.base, unit.applied)
	if !unit.applied.FFX {
		factor := glueModelAspectFactor(aspect)
		scaleX, scaleY, scaleZ = scaleX*factor, scaleY*factor, scaleZ*factor
	}
	unit.node.SetScale(scaleX, scaleY, scaleZ)
}

// apply brings the node in line with the Model frame's current state, touching
// only what changed since the last call. It reports whether anything did.
func (unit *glueSceneUnit) apply(state ui.GlueSceneModel, aspect float32) bool {
	previous := unit.applied
	if state == previous {
		return false
	}
	unit.applied = state
	node := unit.node
	if state.Scale != previous.Scale || state.WidthSquish != previous.WidthSquish || state.HeightSquish != previous.HeightSquish || state.EffectiveScale != previous.EffectiveScale || state.FFX != previous.FFX {
		unit.applyScale(aspect)
	}
	if state.Position != previous.Position || state.EffectiveScale != previous.EffectiveScale || state.FFX != previous.FFX {
		position := glueScenePosition(state)
		node.SetPosition(position[0], position[1], position[2])
	}
	if state.Facing != previous.Facing {
		node.SetRotation(0, float32(state.Facing), 0)
	}
	info, hasInfo := node.UserData().(glueModelInfo)
	if state.Alpha != previous.Alpha {
		node.SetVisible(state.Alpha > 0)
		if hasInfo {
			setGlueModelAlpha(node, info, float32(state.Alpha))
		}
	}
	if hasInfo && info.animation != nil {
		if state.Light != previous.Light || state.HasLight != previous.HasLight {
			light := m2SceneLightFromValues(state.Light, state.HasLight)
			for _, mesh := range info.animation.meshes {
				if mesh != nil {
					mesh.light = light
				}
			}
		}
		if state.Sequence != previous.Sequence {
			info.animation.SetAnimationID(state.Sequence)
		}
	}
	return true
}

// setGlueModelAlpha applies a Model frame's alpha to a loaded model's
// materials the way the loader does for its initial alpha. Particle emitters
// keep the alpha they were built with.
func setGlueModelAlpha(node *core.Node, info glueModelInfo, alpha float32) {
	if info.animation == nil {
		return
	}
	alpha = clampM2Color(alpha)
	children := node.Children()
	for index, animated := range info.animation.meshes {
		if animated == nil || animated.part == nil || index >= len(children) {
			continue
		}
		mesh, ok := children[index].(*graphic.Mesh)
		if !ok {
			continue
		}
		mat, ok := mesh.GetMaterial(0).(*material.Standard)
		if !ok {
			continue
		}
		part := animated.part
		mat.SetOpacity(alpha)
		if m2MaterialBlending(part.material.blend) == material.BlendNone {
			mat.SetTransparent(false)
			mat.SetBlending(material.BlendNone)
		} else {
			mat.SetTransparent(true)
			mat.SetBlending(m2MaterialBlending(part.material.blend))
		}
		mat.SetDepthMask(part.material.flags&0x10 == 0)
		if alpha < 1 && part.material.blend < 2 {
			mat.SetTransparent(true)
			mat.SetBlending(material.BlendNormal)
			mat.SetDepthMask(false)
		}
	}
}

// applyGlueSceneState follows the UI's current Model frames on an already
// loaded scene and returns the model frame height fraction for the camera and
// whether anything changed. models must be the same slots the scene was built
// from (same structure key).
func applyGlueSceneState(root *core.Node, models []ui.GlueSceneModel, aspect float32) (float32, bool) {
	layout := glueSceneLayoutOf(root)
	if layout == nil {
		return 1, false
	}
	changed := false
	for _, unit := range layout.units {
		if unit.slot < len(models) && unit.apply(models[unit.slot], aspect) {
			changed = true
		}
	}
	if layout.aspect != aspect {
		applyGlueSceneAspect(root, aspect)
		changed = true
	}
	for _, unit := range layout.units {
		if !unit.applied.FFX && unit.applied.FrameHeight > 0 {
			layout.frameFraction = float32(unit.applied.FrameHeight / 768)
			break
		}
	}
	return layout.frameFraction, changed
}

func loadGlueScene(loader *ui.Loader, models []ui.GlueSceneModel) (*core.Node, error) {
	root := core.NewNode()
	layout := &glueSceneLayout{frameFraction: 1}
	aggregate, hasCamera := loadGlueSceneReferenceCamera(loader)
	loaded := 0
	for slot, state := range models {
		model, err := loadGlueSceneModel(loader, state.Path, float32(state.Alpha), state.Camera, state.Light, state.HasLight)
		if err != nil {
			continue
		}
		unit := &glueSceneUnit{slot: slot, node: model, base: model.Scale().X, applied: state}
		layout.units = append(layout.units, unit)
		unit.applyScale(0)
		position := glueScenePosition(state)
		model.SetPosition(position[0], position[1], position[2])
		if !state.FFX && loaded == 0 && state.FrameHeight > 0 {
			layout.frameFraction = float32(state.FrameHeight / 768)
		}
		model.SetRotation(0, float32(state.Facing), 0)
		if info, ok := model.UserData().(glueModelInfo); ok {
			if info.animation != nil {
				info.animation.SetAnimationID(state.Sequence)
			}
			if !hasCamera && info.fov > 0 {
				aggregate = info
				aggregate.animation = nil
				aggregate.particles = nil
				hasCamera = hasCamera || info.fov > 0
			}
			aggregate.stats.parts += info.stats.parts
			aggregate.stats.vertices += info.stats.vertices
			aggregate.stats.triangles += info.stats.triangles
			aggregate.stats.textures += info.stats.textures
			aggregate.stats.opaqueBatches += info.stats.opaqueBatches
			aggregate.stats.transparentBatches += info.stats.transparentBatches
			aggregate.stats.particleEmitters += info.stats.particleEmitters
			aggregate.stats.particlePoints += info.stats.particlePoints
		}
		if state.Alpha <= 0 {
			model.SetVisible(false)
		}
		root.Add(model)
		loaded++
	}
	if loaded == 0 {
		return nil, fmt.Errorf("login scene has no renderable models")
	}
	root.SetUserData(aggregate)
	holder := core.NewNode()
	holder.SetUserData(layout)
	root.Add(holder)
	return root, nil
}

func loadGlueSceneReferenceCamera(loader *ui.Loader) (glueModelInfo, bool) {
	data, err := loader.ReadFile(`Character\Human\Male\HumanMale.m2`)
	if err != nil {
		return glueModelInfo{}, false
	}
	model, err := parseM2(data)
	if err != nil {
		return glueModelInfo{}, false
	}
	camera := m2CameraAt(&model, 1)
	if camera == nil {
		return glueModelInfo{}, false
	}
	position := modelVector(camera.position)
	target := modelVector(camera.target)
	return glueModelInfo{position: *math32.NewVector3(position[0], position[1], position[2]), target: *math32.NewVector3(target[0], target[1], target[2]), fov: camera.fov, near: camera.nearClip, far: camera.farClip}, true
}

// glueScenePosition places a model frame's SetPosition(x, y, z). The original
// writes those three values, times the frame's effective scale, straight into
// the model matrix's translation (Wow.exe FUN_0095fba0 / FUN_008251d0), in the
// same M2 space the scene camera is expressed in, so they map to render space
// exactly as M2 vertices and cameras do (modelVector). ModelFFX frames keep the
// earlier mapping because that path was not traced.
func glueScenePosition(state ui.GlueSceneModel) [3]float32 {
	position := state.Position
	if state.FFX {
		return [3]float32{float32(position[1]), float32(position[2]), -float32(position[0])}
	}
	scale := state.EffectiveScale
	if scale <= 0 {
		scale = 1
	}
	return modelVector([3]float32{float32(position[0] * scale), float32(position[1] * scale), float32(position[2] * scale)})
}

func glueSceneScale(base float32, state ui.GlueSceneModel) (float32, float32, float32) {
	scale := state.Scale
	if scale <= 0 {
		scale = 1
	}
	widthSquish, heightSquish := state.WidthSquish, state.HeightSquish
	if widthSquish <= 0 {
		widthSquish = 1
	}
	if heightSquish <= 0 {
		heightSquish = 1
	}
	if !state.FFX && state.EffectiveScale > 0 {
		scale *= state.EffectiveScale
	}
	modelScale := base * float32(scale)
	return modelScale / float32(widthSquish), modelScale / float32(heightSquish), modelScale
}

func updateGlueSceneNode(node *core.Node, elapsed float64) []uint32 {
	if node == nil {
		return nil
	}
	sounds := make([]uint32, 0)
	if info, ok := node.UserData().(glueModelInfo); ok {
		if info.animation != nil {
			sounds = append(sounds, info.animation.Update(elapsed)...)
		}
		if info.particles != nil {
			info.particles.Update(elapsed)
		}
	}
	for _, child := range node.Children() {
		sounds = append(sounds, updateGlueSceneNode(child.GetNode(), elapsed)...)
	}
	return sounds
}
