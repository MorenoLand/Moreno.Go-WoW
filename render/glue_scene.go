package render

import (
	"fmt"

	"github.com/MorenoLand/Moreno.WoW/ui"
	"github.com/g3n/engine/core"
	"github.com/g3n/engine/math32"
)

func loadGlueScene(loader *ui.Loader, models []ui.GlueSceneModel) (*core.Node, error) {
	root := core.NewNode()
	aggregate, hasCamera := loadGlueSceneReferenceCamera(loader)
	loaded := 0
	for _, state := range models {
		model, err := loadGlueSceneModel(loader, state.Path, float32(state.Alpha), state.Camera, state.Light, state.HasLight)
		if err != nil {
			continue
		}
		scaleX, scaleY, scaleZ := glueSceneScale(model.Scale().X, state)
		model.SetScale(scaleX, scaleY, scaleZ)
		position := glueScenePosition(state.Position)
		model.SetPosition(position[0], position[1], position[2])
		model.SetRotation(0, float32(state.Facing), 0)
		if info, ok := model.UserData().(glueModelInfo); ok {
			if info.animation != nil {
				info.animation.SetSequence(state.Sequence)
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

func glueScenePosition(position [3]float64) [3]float32 {
	return [3]float32{float32(position[1]), float32(position[2]), -float32(position[0])}
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
