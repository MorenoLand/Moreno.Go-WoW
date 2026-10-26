package render

import (
	"time"

	"github.com/MorenoLand/Moreno.WoW/ui"
	"github.com/MorenoLand/Moreno.WoW/world"
	"github.com/g3n/engine/core"
	"github.com/g3n/engine/gls"
	"github.com/g3n/engine/graphic"
	"github.com/g3n/engine/texture"
)

const (
	questIconSize   = 0.6
	questIconLift   = 0.45
	questIconPeriod = 5 * time.Second
	questIconRange  = 60.0
)

// worldQuestIcons draws the mark over a quest giver's head: a yellow "!" for a
// quest to take, a yellow "?" for one to hand in, a grey one while it is
// unfinished or too low.
type worldQuestIcons struct {
	status    map[uint64]uint8
	meshes    map[uint64]*graphic.Mesh
	textures  map[string]*texture.Texture2D
	lastQuery time.Time
	reads     int
}

func (icons *worldQuestIcons) handle(packet world.Packet) bool {
	switch packet.Opcode {
	case world.QuestGiverStatus:
		if guid, status, err := world.ParseQuestGiverStatus(packet.Body); err == nil {
			icons.set(guid, status)
		}
	case world.QuestGiverStatusMultiple:
		if statuses, err := world.ParseQuestGiverStatusMultiple(packet.Body); err == nil {
			for guid, status := range statuses {
				icons.set(guid, status)
			}
		}
	default:
		return false
	}
	return true
}

func (icons *worldQuestIcons) set(guid uint64, status uint8) {
	if icons.status == nil {
		icons.status = make(map[uint64]uint8)
	}
	icons.status[guid] = status
}

// iconFor names the picture of a status, "" for none.
func questIconFor(status uint8) (string, bool) {
	switch status {
	case world.QuestStatusLowLevelAvailable, 3, 4:
		return `Interface\GossipFrame\AvailableQuestIcon.blp`, true
	case world.QuestStatusIncomplete:
		return `Interface\GossipFrame\IncompleteQuestIcon.blp`, false
	case world.QuestStatusRewardRep, world.QuestStatusReward2, world.QuestStatusReward:
		return `Interface\GossipFrame\ActiveQuestIcon.blp`, false
	case 7, world.QuestStatusAvailable:
		return `Interface\GossipFrame\AvailableQuestIcon.blp`, false
	}
	return "", false
}

func (icons *worldQuestIcons) texture(loader *ui.Loader, path string) *texture.Texture2D {
	if tex, ok := icons.textures[path]; ok {
		return tex
	}
	if icons.textures == nil {
		icons.textures = make(map[string]*texture.Texture2D)
	}
	tex := loadModelTexture(loader, path)
	if tex != nil {
		tex.SetWrapS(gls.CLAMP_TO_EDGE)
		tex.SetWrapT(gls.CLAMP_TO_EDGE)
	}
	icons.textures[path] = tex
	return tex
}

// update asks for the statuses now and then and shows the marks over the
// creatures near the player.
func (icons *worldQuestIcons) update(scene *core.Node, loader *ui.Loader, entities map[uint64]*worldEntity, near [3]float32, send func(world.ClientOpcode, []byte) error, now time.Time) {
	if now.Sub(icons.lastQuery) >= questIconPeriod && send != nil {
		icons.lastQuery = now
		_ = send(world.ClientQuestGiverStatusMultiple, nil)
	}
	if icons.meshes == nil {
		icons.meshes = make(map[uint64]*graphic.Mesh)
	}
	icons.reads = 8
	shown := make(map[uint64]bool)
	for guid, status := range icons.status {
		path, grey := questIconFor(status)
		entity := entities[guid]
		if path == "" || entity == nil || entity.node == nil || !entity.node.Visible() {
			continue
		}
		position := entity.node.Position()
		dx, dy := position.X-near[0], position.Y-near[1]
		if dx*dx+dy*dy > questIconRange*questIconRange {
			continue
		}
		tex := icons.texture(loader, path)
		if tex == nil {
			continue
		}
		mesh := icons.meshes[guid]
		if mesh == nil {
			var mat *nameplateMaterial
			mesh, mat = newNameplateMesh(tex)
			// The material holds a reference of its own: the mark's texture is shared,
			// and the first mark taken down would otherwise delete it from under the rest
			// (they then drew black).
			tex.Incref()
			// A mark is part of the world: things in front of it hide it.
			mat.SetDepthTest(true)
			mesh.SetRenderOrder(0)
			mat.size = [2]float32{questIconSize, questIconSize}
			icons.meshes[guid] = mesh
			scene.Add(mesh)
		} else if material, ok := mesh.GetMaterial(0).(*nameplateMaterial); ok && grey {
			material.size = [2]float32{questIconSize * 0.9, questIconSize * 0.9}
		}
		anchor := nameplateAnchor(loader, entity.node, entity.modelPath, &icons.reads)
		anchor.Z += questIconLift
		mesh.SetPositionVec(&anchor)
		mesh.SetVisible(true)
		shown[guid] = true
	}
	for guid, mesh := range icons.meshes {
		if shown[guid] {
			continue
		}
		mesh.SetVisible(false)
		if entities[guid] == nil {
			scene.Remove(mesh)
			mesh.Dispose()
			delete(icons.meshes, guid)
		}
	}
}
