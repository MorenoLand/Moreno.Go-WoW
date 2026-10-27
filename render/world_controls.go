package render

import (
	"math"
	"sort"

	"github.com/MorenoLand/Moreno.WoW/world"
)

// worldControls are the calls of the interface that move the character, turn the
// camera and pick targets, set once the world is up.
type worldControls struct {
	movement  func(command string, down bool)
	mouse     func(button int, down bool)
	zoom      func(yards float64)
	yaw       func(degrees float64)
	view      func(distance float64) float64
	distance  func() float64
	nearest   func(kind string, reverse bool)
	last      func(kind string)
	follow    func(guid uint64)
	interact  func(guid uint64)
	targetOf  func(guid uint64) uint64
	falling   func() bool
	state     func() (swimming, flying, indoors bool)
	framerate func() float64
}

func (h *clientHost) Movement(command string, down bool) {
	if h.controls != nil && h.controls.movement != nil {
		h.controls.movement(command, down)
	}
}

func (h *clientHost) MouseAction(button int, down bool) {
	if h.controls != nil && h.controls.mouse != nil {
		h.controls.mouse(button, down)
	}
}

func (h *clientHost) CameraZoom(yards float64) {
	if h.controls != nil && h.controls.zoom != nil {
		h.controls.zoom(yards)
	}
}

func (h *clientHost) CameraYaw(degrees float64) {
	if h.controls != nil && h.controls.yaw != nil {
		h.controls.yaw(degrees)
	}
}

func (h *clientHost) SetCameraView(distance float64) float64 {
	if h.controls != nil && h.controls.view != nil {
		return h.controls.view(distance)
	}
	return distance
}

func (h *clientHost) CameraDistance() float64 {
	if h.controls != nil && h.controls.distance != nil {
		return h.controls.distance()
	}
	return 0
}

func (h *clientHost) TargetNearest(kind string, reverse bool) {
	if h.controls != nil && h.controls.nearest != nil {
		h.controls.nearest(kind, reverse)
	}
}

func (h *clientHost) TargetLast(kind string) {
	if h.controls != nil && h.controls.last != nil {
		h.controls.last(kind)
	}
}

func (h *clientHost) Follow(guid uint64) {
	if h.controls != nil && h.controls.follow != nil {
		h.controls.follow(guid)
	}
}

func (h *clientHost) Interact(guid uint64) {
	if h.controls != nil && h.controls.interact != nil {
		h.controls.interact(guid)
	}
}

func (h *clientHost) TargetOf(guid uint64) uint64 {
	if h.controls != nil && h.controls.targetOf != nil {
		return h.controls.targetOf(guid)
	}
	return 0
}

func (h *clientHost) Falling() bool {
	return h.controls != nil && h.controls.falling != nil && h.controls.falling()
}

func (h *clientHost) MoveState() (bool, bool, bool) {
	if h.controls != nil && h.controls.state != nil {
		return h.controls.state()
	}
	return false, false, false
}

func (h *clientHost) Framerate() float64 {
	if h.controls != nil && h.controls.framerate != nil {
		return h.controls.framerate()
	}
	return 0
}

// worldTargetCandidates lists the units a kind of targeting picks from, nearest
// first.
func worldTargetCandidates(kind string, entities map[uint64]*worldEntity, player *worldEntity, factions *worldFactions, position [3]float32, group map[uint64]bool) []uint64 {
	type candidate struct {
		guid     uint64
		distance float32
	}
	var found []candidate
	for guid, entity := range entities {
		if entity == player || !entity.hasPosition || entity.fields[unitHealthField] == 0 {
			continue
		}
		if entity.objectType != world.ObjectTypeUnit && entity.objectType != world.ObjectTypePlayer {
			continue
		}
		hostile, _ := factions.reaction(player.fields[unitFactionField], entity.fields[unitFactionField])
		isPlayer := entity.objectType == world.ObjectTypePlayer
		switch kind {
		case "enemyplayer":
			if !hostile || !isPlayer {
				continue
			}
		case "friend":
			if hostile {
				continue
			}
		case "friendplayer":
			if hostile || !isPlayer {
				continue
			}
		case "party", "raid":
			if !group[guid] {
				continue
			}
		default:
			if !hostile {
				continue
			}
		}
		dx, dy, dz := entity.movement.Position.X-position[0], entity.movement.Position.Y-position[1], entity.movement.Position.Z-position[2]
		distance := float32(math.Sqrt(float64(dx*dx + dy*dy + dz*dz)))
		if distance > 41 {
			continue
		}
		found = append(found, candidate{guid, distance})
	}
	sort.Slice(found, func(a, b int) bool {
		if found[a].distance != found[b].distance {
			return found[a].distance < found[b].distance
		}
		return found[a].guid < found[b].guid
	})
	guids := make([]uint64, len(found))
	for index, entry := range found {
		guids[index] = entry.guid
	}
	return guids
}
