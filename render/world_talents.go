package render

import (
	"sort"

	"github.com/MorenoLand/Moreno.WoW/ui"
	"github.com/MorenoLand/Moreno.WoW/world"
)

// worldTalents is the character's talents: what the server sends and the trees
// Talent.dbc and TalentTab.dbc describe.
type worldTalents struct {
	loaded   bool
	tabs     []worldTalentTab
	state    world.Talents
	received bool
}

type worldTalentTab struct {
	id, classMask, order uint32
	name, icon, backdrop string
	talents              []ui.TalentDef
}

func (talents *worldTalents) load(loader *ui.Loader) {
	if talents.loaded {
		return
	}
	talents.loaded = true
	icons := make(map[uint32]string)
	if table, err := loadCharacterDBC(loader, `DBFilesClient\SpellIcon.dbc`); err == nil {
		for row := 0; row < table.records; row++ {
			icons[table.value(row, 0)] = table.string(row, 1)
		}
	}
	tabs, err := loadCharacterDBC(loader, `DBFilesClient\TalentTab.dbc`)
	if err != nil || tabs.fields < 24 {
		return
	}
	byID := make(map[uint32]int)
	for row := 0; row < tabs.records; row++ {
		tab := worldTalentTab{id: tabs.value(row, 0), name: tabs.string(row, 1), icon: icons[tabs.value(row, 18)], classMask: tabs.value(row, 20), order: tabs.value(row, 22), backdrop: tabs.string(row, 23)}
		if tab.icon != "" {
			tab.icon = `Interface\Icons\` + tab.icon
		}
		byID[tab.id] = len(talents.tabs)
		talents.tabs = append(talents.tabs, tab)
	}
	table, err := loadCharacterDBC(loader, `DBFilesClient\Talent.dbc`)
	if err != nil || table.fields < 17 {
		return
	}
	rowOf := make(map[uint32]int)
	for row := 0; row < table.records; row++ {
		rowOf[table.value(row, 0)] = row
	}
	for row := 0; row < table.records; row++ {
		index, ok := byID[table.value(row, 1)]
		if !ok {
			continue
		}
		def := ui.TalentDef{ID: table.value(row, 0), Row: int(table.value(row, 2)), Col: int(table.value(row, 3)), PrereqID: table.value(row, 13), PrereqRank: int(table.value(row, 16)) + 1}
		for rank := 0; rank < 5; rank++ {
			if spell := table.value(row, 4+rank); spell != 0 {
				def.RankSpells = append(def.RankSpells, spell)
			}
		}
		if def.PrereqID != 0 {
			if _, known := rowOf[def.PrereqID]; !known {
				def.PrereqID = 0
			}
		}
		talents.tabs[index].talents = append(talents.tabs[index].talents, def)
	}
	for index := range talents.tabs {
		list := talents.tabs[index].talents
		sort.SliceStable(list, func(a, b int) bool {
			if list[a].Row != list[b].Row {
				return list[a].Row < list[b].Row
			}
			return list[a].Col < list[b].Col
		})
	}
}

// handle takes a talents packet; it reports whether the state changed.
func (talents *worldTalents) handle(packet world.Packet) bool {
	if packet.Opcode != world.TalentsInfo {
		return false
	}
	state, player, err := world.ParseTalentsInfo(packet.Body)
	if err != nil || !player {
		return false
	}
	talents.state, talents.received = state, true
	return true
}

// push hands the interface the trees of the character's class and what it
// has learned.
func (talents *worldTalents) push(rt *ui.Runtime, loader *ui.Loader, class uint8) {
	if !talents.received {
		return
	}
	talents.load(loader)
	bit := uint32(1) << (class - 1)
	var trees []worldTalentTab
	for _, tab := range talents.tabs {
		if tab.classMask&bit != 0 {
			trees = append(trees, tab)
		}
	}
	sort.SliceStable(trees, func(a, b int) bool { return trees[a].order < trees[b].order })
	defs := make([]ui.TalentTabDef, len(trees))
	for index, tab := range trees {
		defs[index] = ui.TalentTabDef{ID: tab.id, Name: tab.name, Icon: tab.icon, Background: tab.backdrop, Talents: tab.talents}
	}
	specs := make([]ui.TalentSpecState, len(talents.state.Specs))
	for index, spec := range talents.state.Specs {
		specs[index] = ui.TalentSpecState{Ranks: spec.Ranks}
	}
	rt.SetTalents(defs, specs, talents.state.Active, int(talents.state.Unspent))
}

func (h *clientHost) LearnTalent(id, rank uint32) error {
	session, err := h.worldSession()
	if err != nil {
		return err
	}
	return session.SendRaw(world.ClientLearnTalent, world.BuildLearnTalent(id, rank))
}
