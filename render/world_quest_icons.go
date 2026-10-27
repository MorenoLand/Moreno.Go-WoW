package render

import (
	"time"

	"github.com/MorenoLand/Moreno.WoW/ui"
	"github.com/MorenoLand/Moreno.WoW/world"
	"github.com/g3n/engine/core"
)

const (
	// questIconSize and questIconLift belong to the flat marks this replaced; the
	// capture harness still names them.
	questIconSize   = 0.6
	questIconLift   = 0.45
	questIconPeriod = 5 * time.Second
	questIconRange  = 60.0
	// questMarkerLargeHeight is the head height above which a marker grows, and
	// questMarkerGrowth what it grows by for each yard of it.
	questMarkerLargeHeight = 4.0
	questMarkerGrowth      = 0.25 * 1.5
)

// questMarkerModels are the models over a quest giver's head by the giver's
// status: a yellow "!" for a quest to take, a yellow "?" for one to hand in, a
// grey one while it is unfinished or cannot be taken, blue for what repeats.
// The statuses 2 to 4 (a quest the character has outleveled) show only while
// low-level quests are tracked.
var questMarkerModels = map[uint8]string{
	1:  `Interface\Buttons\TalkToMeGrey.m2`,
	2:  `Interface\Buttons\TalkToMe.m2`,
	3:  `Interface\Buttons\TalkToMeQuestion_LTBlue.m2`,
	4:  `Interface\Buttons\TalkToMeBlue.m2`,
	5:  `Interface\Buttons\TalkToMeQuestion_Grey.m2`,
	6:  `Interface\Buttons\TalkToMeQuestion_LTBlue.m2`,
	7:  `Interface\Buttons\TalkToMeBlue.m2`,
	8:  `Interface\Buttons\TalkToMe.m2`,
	9:  `Interface\Buttons\TalkToMeQuestionMark.m2`,
	10: `Interface\Buttons\TalkToMeQuestionMark.m2`,
}

// questMarkerFor names the model of a status, "" for none.
func questMarkerFor(status uint8, trackLowLevel bool) string {
	if status >= 2 && status <= 4 && !trackLowLevel {
		return ""
	}
	return questMarkerModels[status]
}

// questMarker is the model over one quest giver.
type questMarker struct {
	node *core.Node
	path string
}

// worldQuestIcons draws the marks over the heads of the creatures that give or
// take quests.
type worldQuestIcons struct {
	status    map[uint64]uint8
	markers   map[uint64]*questMarker
	lastQuery time.Time
	reads     int
	// trackLowLevel is the minimap's choice to follow quests below the
	// character's level.
	trackLowLevel bool
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

// questMarkerScale is the size a marker takes: the model's own over a creature
// whose head is not above four yards, a share of the head height above that.
func questMarkerScale(headHeight float32) float32 {
	if headHeight > questMarkerLargeHeight {
		return headHeight * questMarkerGrowth
	}
	return 1
}

// update asks for the statuses now and then and puts the marks over the
// creatures near the player.
func (icons *worldQuestIcons) update(scene *core.Node, loader *ui.Loader, entities map[uint64]*worldEntity, tops map[uint64]float32, near [3]float32, send func(world.ClientOpcode, []byte) error, now time.Time, elapsed float64) {
	if now.Sub(icons.lastQuery) >= questIconPeriod && send != nil {
		icons.lastQuery = now
		_ = send(world.ClientQuestGiverStatusMultiple, nil)
	}
	if icons.markers == nil {
		icons.markers = make(map[uint64]*questMarker)
	}
	icons.reads = 8
	shown := make(map[uint64]bool)
	for guid, status := range icons.status {
		path := questMarkerFor(status, icons.trackLowLevel)
		entity := entities[guid]
		if path == "" || entity == nil || entity.node == nil || !entity.node.Visible() {
			continue
		}
		position := entity.node.Position()
		dx, dy := position.X-near[0], position.Y-near[1]
		if dx*dx+dy*dy > questIconRange*questIconRange {
			continue
		}
		marker := icons.markers[guid]
		if marker != nil && marker.path != path {
			scene.Remove(marker.node)
			marker.node.Dispose()
			marker, icons.markers[guid] = nil, nil
		}
		if marker == nil {
			// The mark is lit by itself, not by the light of the place it hangs in.
			model, err := loadGlueModelWithNormalization(loader, path, false, 1, 0, m2SceneLight{})
			if err != nil || model == nil {
				continue
			}
			node := wrapWorldModel(model, world.WorldPosition{}, 1)
			marker = &questMarker{node: node, path: path}
			icons.markers[guid] = marker
			scene.Add(node)
		}
		// The mark hangs from the point of the creature's model that names it,
		// at the size its head height gives.
		anchor := nameplateAnchor(loader, entity.node, entity.modelPath, &icons.reads)
		scale := questMarkerScale(anchor.Z - position.Z)
		// Above the name when there is one.
		if top, ok := tops[guid]; ok && top > anchor.Z {
			anchor.Z = top
		}
		marker.node.SetPositionVec(&anchor)
		marker.node.SetScale(scale, scale, scale)
		marker.node.SetVisible(true)
		if info, ok := marker.node.UserData().(glueModelInfo); ok && info.animation != nil {
			info.animation.SetMotion(0)
			info.animation.Update(elapsed)
		}
		shown[guid] = true
	}
	for guid, marker := range icons.markers {
		if marker == nil || shown[guid] {
			continue
		}
		marker.node.SetVisible(false)
		if entities[guid] == nil {
			scene.Remove(marker.node)
			marker.node.Dispose()
			delete(icons.markers, guid)
		}
	}
}
