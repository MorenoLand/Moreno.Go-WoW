package render

import (
	"fmt"

	"github.com/MorenoLand/Moreno.WoW/ui"
	"github.com/MorenoLand/Moreno.WoW/world"
)

type worldSpellInfo struct {
	name       string
	rank       string
	icon       string
	debuffType string
}

type worldSpellCache struct {
	loaded bool
	spells map[uint32]worldSpellInfo
}

func (cache *worldSpellCache) load(loader *ui.Loader) {
	if cache.loaded {
		return
	}
	cache.loaded = true
	cache.spells = make(map[uint32]worldSpellInfo)
	spells, err := loadCharacterDBC(loader, `DBFilesClient\Spell.dbc`)
	if err != nil || spells.fields <= 133 {
		return
	}
	icons, _ := loadCharacterDBC(loader, `DBFilesClient\SpellIcon.dbc`)
	iconPaths := make(map[uint32]string)
	for record := 0; record < icons.records; record++ {
		iconPaths[icons.value(record, 0)] = icons.string(record, 1)
	}
	for record := 0; record < spells.records; record++ {
		id := spells.value(record, 0)
		if id == 0 {
			continue
		}
		info := worldSpellInfo{name: spells.string(record, 136), rank: spells.string(record, 153)}
		if iconID := spells.value(record, 133); iconID != 0 {
			info.icon = iconPaths[iconID]
		}
		switch spells.value(record, 2) {
		case 1:
			info.debuffType = "Magic"
		case 2:
			info.debuffType = "Curse"
		case 3:
			info.debuffType = "Disease"
		case 4:
			info.debuffType = "Poison"
		}
		cache.spells[id] = info
	}
}

func (cache *worldSpellCache) info(loader *ui.Loader, spellID uint32) worldSpellInfo {
	cache.load(loader)
	info := cache.spells[spellID]
	if info.name == "" {
		info.name = fmt.Sprintf("Spell %d", spellID)
	}
	if info.icon == "" {
		info.icon = `Interface\Icons\INV_Misc_QuestionMark`
	}
	return info
}

type worldAuraState struct {
	slots  map[uint64][]world.AuraSlot
	spells worldSpellCache
}

func (state *worldAuraState) reset() { state.slots = make(map[uint64][]world.AuraSlot) }

func (state *worldAuraState) apply(rt *ui.Runtime, loader *ui.Loader, update world.AuraUpdateData, all bool, playerGUID uint64) bool {
	if state.slots == nil {
		state.reset()
	}
	slots := state.slots[update.GUID]
	if all {
		slots = nil
	}
	for _, entry := range update.Updates {
		if len(slots) <= int(entry.Slot) {
			slots = append(slots, make([]world.AuraSlot, int(entry.Slot)+1-len(slots))...)
		}
		slots[entry.Slot] = entry.Aura
	}
	state.slots[update.GUID] = slots
	if update.GUID != playerGUID {
		return false
	}
	auras := make([]ui.AuraInfo, 0, len(slots))
	for _, slot := range slots {
		if slot.Empty() {
			continue
		}
		info := state.spells.info(loader, slot.SpellID)
		aura := ui.AuraInfo{Name: info.name, Rank: info.rank, Texture: info.icon, Count: int(slot.Charges), DebuffType: info.debuffType, Harmful: slot.Flags&0x80 != 0}
		if aura.Count == 0 {
			aura.Count = 1
		}
		if aura.Harmful && aura.DebuffType == "" {
			aura.DebuffType = "none"
		}
		if slot.MaxDuration > 0 {
			aura.Duration = float64(slot.MaxDuration) / 1000
			if slot.Duration > 0 {
				aura.ExpirationTime = float64(slot.Duration) / 1000
			}
		}
		auras = append(auras, aura)
	}
	rt.SetUnitAuras("player", auras)
	return true
}
