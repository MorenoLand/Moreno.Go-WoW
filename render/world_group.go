package render

import (
	"github.com/MorenoLand/Moreno.WoW/ui"
	"github.com/MorenoLand/Moreno.WoW/world"
)

// SendPacket sends a client packet the interface built.
func (h *clientHost) SendPacket(opcode world.ClientOpcode, body []byte) error {
	session, err := h.worldSession()
	if err != nil {
		return err
	}
	return session.SendRaw(opcode, body)
}

// groupDetailKey is what is known of a member besides the group list; the
// details are handed over again only when it changes.
type groupDetailKey struct {
	name              string
	race, class, sex  int
	level             int
	hostile, friendly bool
	displayID         uint32
	connected         bool
}

// worldGroupSync keeps the group members' class, race and looks up to date.
type worldGroupSync struct {
	last map[uint64]groupDetailKey
}

// syncWorldGroup tells the interface what the world shows of the members of
// the group: their class and race from what is replicated or what a name query
// answered.
func syncWorldGroup(rt *ui.Runtime, sync *worldGroupSync, names *worldNames, entities map[uint64]*worldEntity, factions *worldFactions, playerFaction uint32, send func(world.ClientOpcode, []byte) error) {
	if !rt.InGroup() {
		sync.last = nil
		return
	}
	if sync.last == nil {
		sync.last = make(map[uint64]groupDetailKey)
	}
	for _, guid := range rt.GroupMemberGUIDs() {
		var info ui.UnitInfo
		if entity := entities[guid]; entity != nil {
			info = names.unitInfoFor(entity, factions, playerFaction)
		} else if name, ok := names.players[guid]; ok {
			info = ui.UnitInfo{GUID: guid, Name: name.Name, Player: true, Exists: true, Visible: true, Connected: true,
				RaceID: int(name.Race), ClassID: int(name.Class), Sex: int(name.Gender) + 2,
				RaceFile: ui.RaceFileFromID(int(name.Race)), ClassFile: ui.ClassFileFromID(int(name.Class))}
		} else if !names.asked[guid] && send != nil {
			names.asked[guid] = true
			_ = send(world.NameQueryOpcode, world.BuildNameQuery(guid))
			continue
		} else {
			continue
		}
		key := groupDetailKey{name: info.Name, race: info.RaceID, class: info.ClassID, sex: info.Sex, level: info.Level,
			hostile: info.Hostile, friendly: info.Friendly, displayID: info.DisplayID, connected: info.Connected}
		if sync.last[guid] == key {
			continue
		}
		sync.last[guid] = key
		rt.SetGroupDetails(guid, info)
	}
}
