package render

import (
	"fmt"
	"math"

	"github.com/MorenoLand/Moreno.WoW/ui"
	"github.com/MorenoLand/Moreno.WoW/world"
	"github.com/g3n/engine/camera"
	"github.com/g3n/engine/core"
	"github.com/g3n/engine/graphic"
	"github.com/g3n/engine/window"
	"github.com/go-gl/glfw/v3.3/glfw"
)

// worldHover is the unit under the mouse: it shows the cursor for what a click
// on it does and has the model lit up (Wow.exe FUN_0051F790 and FUN_004F7A50).
type worldHover struct {
	guid    uint64
	cursors map[string]*glfw.Cursor
	shown   string
}

func newWorldHover() *worldHover {
	return &worldHover{cursors: make(map[string]*glfw.Cursor)}
}

// worldCursorNames are the cursors of Interface\Cursor by the index Wow.exe gives
// them, from 1; the "Unable" variant of one is the same name with that prefix.
var worldCursorNames = [...]string{"", "Point", "Cast", "Buy", "Attack", "Interact", "Speak", "Inspect", "Pickup", "Taxi", "Trainer", "Mine", "Skin", "GatherHerbs", "PickLock", "Mail", "LootAll", "Repair", "RepairNPC", "Item", "SkinHorde", "SkinAlliance", "Innkeeper", "Quest", "QuestRepeatable", "QuestTurnIn"}

// worldInteractRange is how far past its combat reach a unit can be used from.
const worldInteractRange = 4.0

// NPC flags (UNIT_NPC_FLAGS).
const (
	npcFlagGossip       = 0x1
	npcFlagTrainer      = 0x10
	npcFlagVendor       = 0x80
	npcFlagRepair       = 0x1000
	npcFlagFlightMaster = 0x2000
	npcFlagSpiritHealer = 0x4000
	npcFlagSpiritGuide  = 0x8000
	npcFlagInnkeeper    = 0x10000
	npcFlagBanker       = 0x20000
	npcFlagPetitioner   = 0x40000
	npcFlagTabard       = 0x80000
	npcFlagBattlemaster = 0x100000
	npcFlagAuctioneer   = 0x200000
	npcFlagStable       = 0x400000
	npcFlagGuildBanker  = 0x800000
	npcFlagSpellClick   = 0x1000000
)

// worldUnitCursor is the cursor a unit under the mouse shows: by what it offers
// when it is a friend with something to do, to attack when it is an enemy, to
// loot when it is dead and has something. out is set when it is too far to use.
func worldUnitCursor(entity *worldEntity, player [3]float32, hostile bool) (name string, ok bool) {
	if entity == nil {
		return "", false
	}
	flags := entity.fields[unitNPCFlagsField]
	health := entity.fields[unitHealthField]
	reach := math.Float32frombits(entity.fields[unitCombatReach])
	dx := entity.movement.Position.X - player[0]
	dy := entity.movement.Position.Y - player[1]
	dz := entity.movement.Position.Z - player[2]
	distance := dx*dx + dy*dy + dz*dz
	limit := (reach + worldInteractRange) * (reach + worldInteractRange)
	out := limit < distance
	pick := func(index int) (string, bool) {
		cursor := worldCursorNames[index]
		if out {
			cursor = "Unable" + cursor
		}
		return cursor, true
	}
	if health == 0 {
		if entity.fields[unitDynamicField]&0x1 != 0 {
			return pick(8)
		}
		return "", false
	}
	if hostile {
		// Out of melee reach the sword is shown as the one that cannot strike.
		cursor := "Attack"
		if distance > (reach+worldInteractRange)*(reach+worldInteractRange) {
			cursor = "UnableAttack"
		}
		return cursor, true
	}
	switch {
	case flags&npcFlagRepair != 0:
		return pick(18)
	case flags&npcFlagInnkeeper != 0:
		return pick(22)
	case flags&npcFlagFlightMaster != 0:
		return pick(9)
	case flags&npcFlagTrainer != 0:
		return pick(10)
	case flags&(npcFlagSpiritHealer|npcFlagSpiritGuide) != 0:
		return pick(6)
	case flags&(npcFlagBanker|npcFlagGuildBanker) != 0:
		return pick(3)
	case flags&(npcFlagPetitioner|npcFlagTabard|npcFlagBattlemaster) != 0:
		return pick(6)
	case flags&npcFlagAuctioneer != 0:
		return pick(3)
	case flags&npcFlagStable != 0:
		return pick(6)
	case flags&npcFlagVendor != 0:
		return pick(8)
	case flags&npcFlagGossip != 0:
		return pick(6)
	case flags&npcFlagSpellClick != 0:
		return pick(5)
	}
	return "", false
}

// setCursor shows a named cursor, the pointer when the name is empty.
func (h *worldHover) setCursor(win *window.GlfwWindow, loader *ui.Loader, name string) {
	if name == "" {
		name = "Point"
	}
	if name == h.shown {
		return
	}
	cursor, ok := h.cursors[name]
	if !ok {
		cursor = loadWorldCursor(loader, name)
		h.cursors[name] = cursor
	}
	if cursor == nil {
		return
	}
	h.shown = name
	win.Window.SetCursor(cursor)
}

func loadWorldCursor(loader *ui.Loader, name string) *glfw.Cursor {
	data, err := loader.ReadAsset(fmt.Sprintf(`Interface\Cursor\%s.blp`, name))
	if err != nil {
		return nil
	}
	image, err := ui.DecodeBLP(data)
	if err != nil {
		return nil
	}
	return glfw.CreateCursor(image, 0, 0)
}

// update picks the unit under the mouse and shows its cursor and highlight.
func (h *worldHover) update(win *window.GlfwWindow, loader *ui.Loader, cam *camera.Camera, entities map[uint64]*worldEntity, ownGUID uint64, factions *worldFactions, playerAt [3]float32, x, y float64, overUI bool) {
	guid := uint64(0)
	if !overUI {
		width, height := win.GetSize()
		guid = worldPickEntity(cam, entities, ownGUID, x, y, float64(width), float64(height))
	}
	if guid != h.guid {
		if previous := entities[h.guid]; previous != nil && previous.node != nil {
			setNodeHighlight(previous.node, false)
		}
		if entity := entities[guid]; entity != nil && entity.node != nil {
			setNodeHighlight(entity.node, true)
		}
		h.guid = guid
	}
	cursor := ""
	if entity, player := entities[guid], entities[ownGUID]; entity != nil && player != nil {
		hostile := false
		if factions != nil {
			hostile, _ = factions.reaction(player.fields[unitFactionField], entity.fields[unitFactionField])
		}
		if name, ok := worldUnitCursor(entity, playerAt, hostile); ok {
			cursor = name
		}
	}
	h.setCursor(win, loader, cursor)
}

// setNodeHighlight lights a model up, or puts its look back. The highlight is
// the colour of the light, as Wow.exe gives the model (FUN_00743C70).
func setNodeHighlight(node core.INode, on bool) {
	colour := [3]float32{}
	if on && worldAtmo.valid {
		colour = worldAtmo.diffuse
	} else if on {
		colour = [3]float32{0.5, 0.5, 0.5}
	}
	var walk func(node core.INode)
	walk = func(node core.INode) {
		if mesh, ok := node.(*graphic.Mesh); ok {
			if mat, ok := mesh.GetMaterial(0).(*worldMaterial); ok {
				mat.highlight = colour
			}
		}
		for _, child := range node.GetNode().Children() {
			walk(child)
		}
	}
	walk(node)
}

var _ = world.MovementFlagFlying
