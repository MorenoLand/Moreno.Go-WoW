package render

import (
	"fmt"

	"github.com/MorenoLand/Moreno.WoW/ui"
	"github.com/g3n/engine/core"
)

func loadGlueScene(loader *ui.Loader, models []ui.GlueSceneModel) (*core.Node, error) {
	root := core.NewNode()
	aggregate := glueModelInfo{}
	hasCamera := false
	loaded := 0
	for _, state := range models {
		model, err := loadGlueModel(loader, state.Path)
		if err != nil {
			continue
		}
		if state.Scale <= 0 {
			state.Scale = 1
		}
		modelScale := model.Scale().X * float32(state.Scale)
		model.SetScale(modelScale, modelScale, modelScale)
		model.SetPosition(float32(state.Position[0]), float32(state.Position[1]), float32(state.Position[2]))
		model.SetRotation(0, 0, float32(state.Facing))
		if info, ok := model.UserData().(glueModelInfo); ok {
			if info.animation != nil {
				info.animation.SetSequence(state.Sequence)
			}
			if !hasCamera || info.fov > 0 {
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
