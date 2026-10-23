package render

import (
	"fmt"
	"math"

	"github.com/MorenoLand/Moreno.WoW/ui"
	"github.com/MorenoLand/Moreno.WoW/world"
	"github.com/g3n/engine/camera"
	"github.com/g3n/engine/core"
	"github.com/g3n/engine/gui"
	"github.com/g3n/engine/math32"
	"github.com/g3n/engine/texture"
)

const (
	nameplateRange        = 60.0
	nameplateMaxNewPerRun = 12
	playerFlagsField      = 150 // PLAYER_FLAGS
	playerGuildField      = 151 // PLAYER_GUILDID
	playerFlagGM          = 0x08
)

// worldNameplate is the name drawn over one unit: a small image the size of the
// text, moved over the unit's head each frame.
type worldNameplate struct {
	image *gui.Image
	tex   *texture.Texture2D
	key   string
}

// worldNameplates draws the names (and guild or title lines) over the units
// near the player.
type worldNameplates struct {
	plates map[uint64]*worldNameplate
}

func newWorldNameplates() *worldNameplates {
	return &worldNameplates{plates: make(map[uint64]*worldNameplate)}
}

// nameplateInputs is what a frame of nameplates needs from the world.
type nameplateInputs struct {
	scene         *core.Node
	engine        *ui.UIEngine
	cam           *camera.Camera
	width         float32
	height        float32
	entities      map[uint64]*worldEntity
	ownGUID       uint64
	playerNode    *core.Node
	cameraAt      [3]float32
	names         *worldNames
	factions      *worldFactions
	send          func(world.ClientOpcode, []byte) error
	playerFaction uint32
}

// nameplateColour is the colour of a unit's name from how it relates to the
// player: hostile red, neutral yellow, friendly green for creatures; players
// of the player's side are drawn blue.
func nameplateColour(player, hostile, friendly bool) (uint8, uint8, uint8) {
	switch {
	case hostile:
		return 255, 0, 0
	case player && friendly:
		return 102, 102, 255
	case friendly:
		return 0, 255, 0
	}
	return 255, 255, 0
}

// lines are the text lines over an entity, or nil while its name is unknown.
func (in *nameplateInputs) lines(entity *worldEntity) []ui.NameplateLine {
	hostile, friendly := false, true
	if entity.guid != in.ownGUID && in.factions != nil {
		hostile, friendly = in.factions.reaction(in.playerFaction, entity.fields[unitFactionField])
	}
	switch entity.objectType {
	case world.ObjectTypePlayer:
		name := in.names.players[entity.guid].Name
		if name == "" {
			return nil
		}
		if entity.fields[playerFlagsField]&playerFlagGM != 0 {
			name = "<GM>" + name
		}
		r, g, b := nameplateColour(true, hostile, friendly)
		lines := []ui.NameplateLine{{Text: name, R: r, G: g, B: b}}
		if guildID := entity.fields[playerGuildField]; guildID != 0 {
			in.names.requestGuild(guildID, in.send)
			if guild := in.names.guilds[guildID]; guild != "" {
				lines = append(lines, ui.NameplateLine{Text: "<" + guild + ">", R: r, G: g, B: b})
			}
		}
		return lines
	case world.ObjectTypeUnit:
		creature, ok := in.names.creatures[entity.fields[objectEntryField]]
		if !ok || creature.Name == "" {
			return nil
		}
		r, g, b := nameplateColour(false, hostile, friendly)
		lines := []ui.NameplateLine{{Text: creature.Name, R: r, G: g, B: b}}
		if creature.SubName != "" {
			lines = append(lines, ui.NameplateLine{Text: "<" + creature.SubName + ">", R: r, G: g, B: b})
		}
		return lines
	}
	return nil
}

func nameplateKey(lines []ui.NameplateLine, size int) string {
	key := fmt.Sprintf("%d", size)
	for _, line := range lines {
		key += fmt.Sprintf("|%s/%d.%d.%d", line.Text, line.R, line.G, line.B)
	}
	return key
}

// update places a frame's nameplates.
func (n *worldNameplates) update(in nameplateInputs) {
	if in.engine == nil || in.cam == nil || in.names == nil {
		return
	}
	in.cam.UpdateMatrixWorld()
	seen := make(map[uint64]bool, len(n.plates))
	asked := 0
	for guid, entity := range in.entities {
		node := entity.node
		if guid == in.ownGUID {
			node = in.playerNode
		}
		if node == nil || !node.Visible() || (entity.objectType != world.ObjectTypeUnit && entity.objectType != world.ObjectTypePlayer) {
			continue
		}
		position := node.Position()
		dx, dy, dz := position.X-in.cameraAt[0], position.Y-in.cameraAt[1], position.Z-in.cameraAt[2]
		distance := math.Sqrt(float64(dx*dx + dy*dy + dz*dz))
		if distance > nameplateRange {
			continue
		}
		if !in.names.asked[entity.guid] && asked < nameplateMaxNewPerRun {
			asked++
			in.names.request(entity, in.send)
		}
		lines := in.lines(entity)
		if len(lines) == 0 {
			continue
		}
		// Head height: the model's top in its own units, scaled like the node.
		top := float32(1.8)
		if info, ok := node.UserData().(glueModelInfo); ok && info.modelTop > 0 {
			top = info.modelTop * node.Scale().Y
		}
		head := math32.NewVector3(position.X, position.Y, position.Z+top+0.25)
		screen, visible := worldToScreen(in.cam, *head, in.width, in.height)
		if !visible {
			continue
		}
		// The text gets smaller with distance.
		size := float64(in.height) * 0.0125 * math.Min(1.15, math.Max(0.45, 32/math.Max(distance, 1)))
		bucket := int(math.Round(size/2)) * 2
		if bucket < 6 {
			bucket = 6
		}
		key := nameplateKey(lines, bucket)
		plate := n.plates[guid]
		if plate == nil {
			plate = &worldNameplate{}
			n.plates[guid] = plate
		}
		if plate.key != key {
			picture := in.engine.RenderNameplate(lines, float64(bucket))
			if picture == nil {
				continue
			}
			if plate.image == nil {
				plate.tex = texture.NewTexture2DFromRGBA(picture)
				plate.image = gui.NewImageFromTex(plate.tex)
				plate.image.SetZLayerDelta(-2)
				in.scene.Add(plate.image)
			} else {
				plate.tex.SetFromRGBA(picture)
			}
			plate.image.SetSize(float32(picture.Bounds().Dx()), float32(picture.Bounds().Dy()))
			plate.key = key
		}
		seen[guid] = true
		plate.image.SetPosition(screen.X-plate.image.Width()/2, screen.Y-plate.image.Height())
		plate.image.SetVisible(true)
	}
	for guid, plate := range n.plates {
		if seen[guid] {
			continue
		}
		if plate.image != nil {
			plate.image.SetVisible(false)
		}
		if _, alive := in.entities[guid]; !alive {
			if plate.image != nil {
				in.scene.Remove(plate.image)
				plate.image.Dispose()
			}
			delete(n.plates, guid)
		}
	}
}

// hide takes every nameplate off the screen.
func (n *worldNameplates) hide() {
	for _, plate := range n.plates {
		if plate.image != nil {
			plate.image.SetVisible(false)
		}
	}
}
