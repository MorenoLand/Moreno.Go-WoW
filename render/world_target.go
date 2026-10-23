package render

import (
	"math"
	"sort"

	"github.com/MorenoLand/Moreno.WoW/ui"
	"github.com/MorenoLand/Moreno.WoW/world"
	"github.com/g3n/engine/camera"
	"github.com/g3n/engine/math32"
	"github.com/g3n/engine/window"
)

// Unit update fields used for targeting.
const (
	objectEntryField   = 0x03
	unitHealthField    = 0x18
	unitLevelField     = 0x36
	unitFactionField   = 0x37
	unitFlagsField     = 0x3B
	unitBoundingRadius = 0x41
	unitCombatReach    = 0x42
	unitDynamicField   = 0x4E
	unitNPCFlagsField  = 0x51
)

// worldFactionTemplate is one FactionTemplate.dbc row: who a faction is
// friendly or hostile to, by faction and by group.
type worldFactionTemplate struct {
	faction, flags, group, friendGroup, enemyGroup uint32
	enemies, friends                               [4]uint32
}

type worldFactions struct {
	templates map[uint32]worldFactionTemplate
}

func readWorldFactions(loader *ui.Loader) *worldFactions {
	factions := &worldFactions{templates: make(map[uint32]worldFactionTemplate)}
	readWorldDBCRows(loader, `DBFilesClient\FactionTemplate.dbc`, 14, func(row []uint32) {
		template := worldFactionTemplate{faction: row[1], flags: row[2], group: row[3], friendGroup: row[4], enemyGroup: row[5]}
		copy(template.enemies[:], row[6:10])
		copy(template.friends[:], row[10:14])
		factions.templates[row[0]] = template
	})
	return factions
}

// reaction is how the unit with template `other` stands to the one with
// template `self`: listed enemies, listed friends, then the group masks.
func (f *worldFactions) reaction(self, other uint32) (hostile, friendly bool) {
	if f == nil {
		return false, false
	}
	a, ok := f.templates[self]
	b, ok2 := f.templates[other]
	if !ok || !ok2 {
		return false, false
	}
	for _, enemy := range a.enemies {
		if enemy != 0 && enemy == b.faction {
			return true, false
		}
	}
	for _, friend := range a.friends {
		if friend != 0 && friend == b.faction {
			return false, true
		}
	}
	if a.enemyGroup&b.group != 0 {
		return true, false
	}
	if a.friendGroup&b.group != 0 {
		return false, true
	}
	return false, false
}

// worldNames remembers the answers to name queries and asks once per unit.
type worldNames struct {
	creatures map[uint32]world.CreatureInfo
	players   map[uint64]world.PlayerName
	asked     map[uint64]bool
	// guilds are guild names by id, guildAsked the ids already queried.
	guilds     map[uint32]string
	guildAsked map[uint32]bool
}

func newWorldNames() *worldNames {
	return &worldNames{creatures: make(map[uint32]world.CreatureInfo), players: make(map[uint64]world.PlayerName), asked: make(map[uint64]bool), guilds: make(map[uint32]string), guildAsked: make(map[uint32]bool)}
}

// request queries the name of an entity the first time it is needed.
func (n *worldNames) request(entity *worldEntity, send func(world.ClientOpcode, []byte) error) {
	if n == nil || entity == nil || n.asked[entity.guid] || send == nil {
		return
	}
	n.asked[entity.guid] = true
	switch entity.objectType {
	case world.ObjectTypePlayer:
		_ = send(world.NameQueryOpcode, world.BuildNameQuery(entity.guid))
	case world.ObjectTypeUnit:
		if entry := entity.fields[objectEntryField]; entry != 0 {
			if _, known := n.creatures[entry]; !known {
				_ = send(world.CreatureQueryOpcode, world.BuildCreatureQuery(entry, entity.guid))
			}
		}
	}
}

var worldCreatureTypeNames = map[uint32]string{1: "Beast", 2: "Dragonkin", 3: "Demon", 4: "Elemental", 5: "Giant", 6: "Undead", 7: "Humanoid", 8: "Critter", 9: "Mechanical", 10: "Not specified", 11: "Totem", 12: "Non-combat Pet", 13: "Gas Cloud"}

var worldClassificationNames = map[uint32]string{0: "normal", 1: "elite", 2: "rareelite", 3: "worldboss", 4: "rare"}

// unitInfoFor describes an entity for the interface: name, class, level and
// how it reacts to the player.
func (n *worldNames) unitInfoFor(entity *worldEntity, factions *worldFactions, playerFaction uint32) ui.UnitInfo {
	info := ui.UnitInfo{GUID: entity.guid, Exists: true, Connected: true, Visible: true, PowerToken: "MANA"}
	info.Player = entity.objectType == world.ObjectTypePlayer
	bytes0 := entity.fields[world.UnitBytes0Field]
	info.PowerType = int(bytes0 >> 24)
	info.Level = int(entity.fields[unitLevelField])
	if info.Player {
		info.RaceID, info.ClassID = int(bytes0&0xff), int(bytes0>>8&0xff)
		info.RaceFile, info.ClassFile = ui.RaceFileFromID(info.RaceID), ui.ClassFileFromID(info.ClassID)
		info.Sex = int(bytes0>>16&0xff) + 2
		if name, ok := n.players[entity.guid]; ok {
			info.Name = name.Name
		}
	} else if creature, ok := n.creatures[entity.fields[objectEntryField]]; ok {
		info.Name = creature.Name
		info.Classification = worldClassificationNames[creature.Rank]
		info.CreatureType = worldCreatureTypeNames[creature.Type]
	}
	info.Hostile, info.Friendly = factions.reaction(playerFaction, entity.fields[unitFactionField])
	return info
}

// syncWorldTarget derives the "target" unit from the player's replicated
// UNIT_FIELD_TARGET (two uint32 halves) and refreshes its vitals and names.
func syncWorldTarget(rt *ui.Runtime, entities map[uint64]*worldEntity, player *worldEntity, names *worldNames, factions *worldFactions, send func(world.ClientOpcode, []byte) error) bool {
	if rt == nil || player == nil {
		return false
	}
	guid := uint64(player.fields[world.UnitTargetField]) | uint64(player.fields[world.UnitTargetField+1])<<32
	entity := entities[guid]
	current := rt.UnitGUID("target")
	if guid == 0 || entity == nil {
		if current != 0 {
			rt.SetTarget(nil)
			return true
		}
		return false
	}
	names.request(entity, send)
	info := names.unitInfoFor(entity, factions, player.fields[unitFactionField])
	changed := false
	if current != guid {
		rt.SetTarget(&info)
		changed = true
	} else if rt.RefreshTarget(info) {
		changed = true
	}
	if rt.ApplyUnitFields("target", entity.fields) {
		changed = true
	}
	return changed
}

// worldEntityPickRadius is the size of the sphere a click or Tab picks a unit
// by: its combat reach, never under a body's width.
func worldEntityPickRadius(entity *worldEntity) float32 {
	scale := float32(1)
	if value := math.Float32frombits(entity.fields[world.ObjectScaleField]); value > 0 {
		scale = value
	}
	reach := math.Float32frombits(entity.fields[unitCombatReach])
	radius := reach * 0.6 * scale
	if bounding := math.Float32frombits(entity.fields[unitBoundingRadius]) * scale * 1.5; bounding > radius {
		radius = bounding
	}
	if radius < 0.6 {
		radius = 0.6
	}
	return radius
}

func worldEntityPickCenter(entity *worldEntity) math32.Vector3 {
	radius := worldEntityPickRadius(entity)
	p := entity.movement.Position
	return math32.Vector3{X: p.X, Y: p.Y, Z: p.Z + radius}
}

// worldPickEntity returns the nearest selectable unit under a window position.
func worldPickEntity(cam *camera.Camera, entities map[uint64]*worldEntity, ownGUID uint64, x, y, width, height float64) uint64 {
	if cam == nil || width <= 0 || height <= 0 {
		return 0
	}
	ndcX, ndcY := float32(x/width*2-1), float32(1-y/height*2)
	far := cam.Unproject(math32.NewVector3(ndcX, ndcY, 0.9))
	origin := cam.Position()
	dir := far.Sub(&origin)
	dir.Normalize()
	best, bestDistance := uint64(0), float32(math.MaxFloat32)
	for guid, entity := range entities {
		if guid == ownGUID || !entity.hasPosition || entity.node == nil || !entity.node.Visible() {
			continue
		}
		if entity.objectType != world.ObjectTypeUnit && entity.objectType != world.ObjectTypePlayer {
			continue
		}
		center := worldEntityPickCenter(entity)
		radius := worldEntityPickRadius(entity)
		oc := center.Clone().Sub(&origin)
		along := oc.Dot(dir)
		if along <= 0 {
			continue
		}
		closest2 := oc.LengthSq() - along*along
		if closest2 > radius*radius {
			continue
		}
		if along < bestDistance {
			best, bestDistance = guid, along
		}
	}
	return best
}

// worldTabTargets lists the units Tab cycles through: living ones the player
// could attack, within range, nearest first.
func worldTabTargets(entities map[uint64]*worldEntity, player *worldEntity, factions *worldFactions, position [3]float32, maxDistance float32) []uint64 {
	type candidate struct {
		guid     uint64
		distance float32
	}
	var found []candidate
	for guid, entity := range entities {
		if entity == player || !entity.hasPosition || entity.objectType != world.ObjectTypeUnit {
			continue
		}
		if entity.fields[unitHealthField] == 0 {
			continue
		}
		hostile, _ := factions.reaction(player.fields[unitFactionField], entity.fields[unitFactionField])
		if !hostile {
			continue
		}
		dx, dy, dz := entity.movement.Position.X-position[0], entity.movement.Position.Y-position[1], entity.movement.Position.Z-position[2]
		distance := float32(math.Sqrt(float64(dx*dx + dy*dy + dz*dz)))
		if distance > maxDistance {
			continue
		}
		found = append(found, candidate{guid, distance})
	}
	sort.Slice(found, func(i, j int) bool { return found[i].distance < found[j].distance })
	out := make([]uint64, len(found))
	for i, c := range found {
		out[i] = c.guid
	}
	return out
}

// worldClickSlop is how far (in pixels) a mouse button may travel between
// press and release and still be a click rather than a camera drag.
const worldClickSlop = 5

// worldMousePress is a mouse button pressed over the world rather than the
// interface.
type worldMousePress struct {
	x, y   float64
	button window.MouseButton
	active bool
}

// requestGuild queries a guild's name the first time it is needed.
func (n *worldNames) requestGuild(id uint32, send func(world.ClientOpcode, []byte) error) {
	if n == nil || id == 0 || send == nil || n.guildAsked[id] {
		return
	}
	n.guildAsked[id] = true
	_ = send(world.GuildQueryOpcode, world.BuildGuildQuery(id))
}
