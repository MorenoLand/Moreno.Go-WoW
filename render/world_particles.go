package render

import (
	"github.com/MorenoLand/Moreno.WoW/world"
	"math"

	"github.com/MorenoLand/Moreno.WoW/ui"
	"github.com/g3n/engine/core"
	"github.com/g3n/engine/math32"
)

// worldParticleBudget is how many particle emitting doodads one tile may run.
const worldParticleBudget = 192

// worldParticleDistance is how far from the player a doodad's particles move.
const worldParticleDistance = 120

// worldParticleSource is a placed doodad whose model has particle emitters
// (smoke, glows, sparks).
type worldParticleSource struct {
	node   *core.Node
	system *m2ParticleSystem
	centre [3]float32
	// info is set for an animated doodad (a piston, a wheel, a waterfall), which
	// plays its animation and runs its own emitters.
	info *glueModelInfo
}

// worldEmitterPlacement is where a doodad with emitters stands, kept so every
// tile that shows the same map object can run its own particles.
type worldEmitterPlacement struct {
	model    *parsedM2
	origin   [3]float32
	rotation *math32.Quaternion
	scale    float32
	// path is the model of an animated doodad.
	path string
}

// worldDoodadAnimated reports whether a doodad's model moves: bones, textures
// or colours animate. Such a doodad is not merged into the static scene.
func (cache *worldSceneAssetCache) worldDoodadAnimated(loader *ui.Loader, path string) bool {
	cache.animatedMu.Lock()
	defer cache.animatedMu.Unlock()
	if animated, ok := cache.animated[path]; ok {
		return animated
	}
	animated := false
	if model, err := loadSharedM2(loader, path); err == nil && len(model.sequences) > 0 {
		animated = modelHasAnimation(&model) || modelHasTextureAnimation(&model) || modelHasColorAnimation(&model)
	}
	cache.animated[path] = animated
	return animated
}

// newWorldAnimatedDoodad builds a doodad that plays its animation.
func newWorldAnimatedDoodad(loader *ui.Loader, path string, origin [3]float32, rotation *math32.Quaternion, scale float32) *worldParticleSource {
	model, err := buildWorldCreatureModel(loader, worldCreatureDefinition{path: path, scale: 1, displayScale: 1, modelScale: 1})
	if err != nil {
		return nil
	}
	info, _ := model.UserData().(glueModelInfo)
	outer := wrapWorldModel(model, world.WorldPosition{X: origin[0], Y: origin[1], Z: origin[2]}, scale)
	if rotation != nil {
		outer.SetRotationQuat(rotation)
	}
	return &worldParticleSource{node: outer, centre: origin, info: &info}
}

// newWorldParticleSource runs a model's emitters at a place in the world.
func newWorldParticleSource(loader *ui.Loader, model *parsedM2, cache *worldSceneAssetCache, origin [3]float32, rotation *math32.Quaternion, scale float32) *worldParticleSource {
	inner := core.NewNode()
	// The emitters are built in the glue models' space; the world's doodads are in
	// the model's own, which this turn undoes (as wrapWorldModel does).
	inner.SetRotation(float32(math.Pi/2), 0, 0)
	system := buildM2ParticleSystem(loader, model, inner, 1, cache.textures, 1)
	if system == nil {
		return nil
	}
	outer := core.NewNode()
	outer.SetPosition(origin[0], origin[1], origin[2])
	if rotation != nil {
		outer.SetRotationQuat(rotation)
	}
	outer.SetScale(scale, scale, scale)
	outer.Add(inner)
	return &worldParticleSource{node: outer, system: system, centre: origin}
}

// updateParticles moves the particles of the tiles near the player.
func (s *worldTerrainStreamer) updateParticles(x, y float32, elapsed float64) {
	if elapsed <= 0 {
		return
	}
	limit := float32(worldParticleDistance * worldParticleDistance)
	for _, tile := range s.tiles {
		for _, source := range tile.collision.particles {
			dx, dy := source.centre[0]-x, source.centre[1]-y
			if dx*dx+dy*dy > limit {
				continue
			}
			if source.system != nil {
				source.system.Update(elapsed)
			}
			if info := source.info; info != nil {
				if info.animation != nil {
					info.animation.Update(elapsed)
				}
				if info.particles != nil {
					info.particles.Update(elapsed)
				}
			}
		}
	}
}
