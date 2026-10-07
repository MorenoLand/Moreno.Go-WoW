package render

import (
	"encoding/binary"
	"fmt"
	"math"

	"github.com/MorenoLand/Moreno.WoW/ui"
	"github.com/MorenoLand/Moreno.WoW/world"
)

func loadWorldSpellCatalog(rt *ui.Runtime, loader *ui.Loader) error {
	catalog, skills, err := readWorldSpellCatalog(loader)
	if err != nil {
		return err
	}
	rt.SetSpellCatalog(catalog, skills)
	return nil
}

func readWorldSpellCatalog(loader *ui.Loader) (map[uint32]ui.SpellInfo, map[uint32]ui.SpellSkillLine, error) {
	spells, err := loadCharacterDBC(loader, `DBFilesClient\Spell.dbc`)
	if err != nil {
		return nil, nil, err
	}
	if spells.fields < 234 {
		return nil, nil, fmt.Errorf("Spell.dbc requires WotLK layout, got %d fields", spells.fields)
	}
	icons, err := loadCharacterDBC(loader, `DBFilesClient\SpellIcon.dbc`)
	if err != nil {
		return nil, nil, err
	}
	lines, err := loadCharacterDBC(loader, `DBFilesClient\SkillLine.dbc`)
	if err != nil {
		return nil, nil, err
	}
	abilities, err := loadCharacterDBC(loader, `DBFilesClient\SkillLineAbility.dbc`)
	if err != nil {
		return nil, nil, err
	}
	casts, err := loadCharacterDBC(loader, `DBFilesClient\SpellCastTimes.dbc`)
	if err != nil {
		return nil, nil, err
	}
	ranges, err := loadCharacterDBC(loader, `DBFilesClient\SpellRange.dbc`)
	if err != nil {
		return nil, nil, err
	}
	if icons.fields < 2 || lines.fields < 38 || abilities.fields < 14 || casts.fields < 2 || ranges.fields < 5 {
		return nil, nil, fmt.Errorf("spell support DBC layout is truncated")
	}
	iconPaths := make(map[uint32]string)
	for row := 0; row < icons.records; row++ {
		iconPaths[icons.value(row, 0)] = icons.string(row, 1)
	}
	castTimes := make(map[uint32]int)
	for row := 0; row < casts.records; row++ {
		castTimes[casts.value(row, 0)] = int(int32(casts.value(row, 1)))
	}
	spellRanges := make(map[uint32][2]float64)
	for row := 0; row < ranges.records; row++ {
		spellRanges[ranges.value(row, 0)] = [2]float64{float64(math.Float32frombits(ranges.value(row, 1))), float64(math.Float32frombits(ranges.value(row, 3)))}
	}
	skillLines := make(map[uint32]ui.SpellSkillLine)
	for row := 0; row < lines.records; row++ {
		id := lines.value(row, 0)
		skillLines[id] = ui.SpellSkillLine{ID: id, Category: lines.value(row, 1), Name: lines.string(row, 3), Icon: iconPaths[lines.value(row, 37)]}
	}
	spellSkills := make(map[uint32]uint32)
	for row := 0; row < abilities.records; row++ {
		spellSkills[abilities.value(row, 2)] = abilities.value(row, 1)
	}
	catalog := make(map[uint32]ui.SpellInfo, spells.records)
	for row := 0; row < spells.records; row++ {
		id := spells.value(row, 0)
		distance := spellRanges[spells.value(row, 46)]
		info := ui.SpellInfo{ID: id, SkillLine: spellSkills[id], Category: spells.value(row, 1), Attributes: spells.value(row, 4), Name: spells.string(row, 136), Rank: spells.string(row, 153), Icon: iconPaths[spells.value(row, 133)], Cost: int(spells.value(row, 42)), PowerType: int(int32(spells.value(row, 41))), CastTime: castTimes[spells.value(row, 28)], MinRange: distance[0], MaxRange: distance[1]}
		for field := 52; field < 60; field++ {
			if int32(spells.value(row, field)) > 0 {
				info.Recipe = true
			}
		}
		for field := 71; field < 74; field++ {
			if effect := spells.value(row, field); effect == 24 || effect == 114 {
				info.Recipe = true
			}
		}
		info.Funnel = spells.value(row, 6)&0x800 != 0
		info.Interruptible = spells.value(row, 31)&0x8 != 0
		catalog[id] = info
	}
	return catalog, skillLines, nil
}

func applyWorldSpellPacket(rt *ui.Runtime, packet world.Packet, playerGUID uint64) (bool, error) {
	switch packet.Opcode {
	case world.ChannelStart, world.ChannelUpdate:
		r := world.NewReader(packet.Body, "channel update")
		guid, err := world.ReadPackedGUID(r)
		if err != nil {
			return true, err
		}
		id := uint32(0)
		if packet.Opcode == world.ChannelStart {
			id, err = r.U32()
			if err != nil {
				return true, err
			}
		}
		duration, err := r.U32()
		if err != nil {
			return true, err
		}
		if err := r.Finish(); err != nil {
			return true, err
		}
		if guid == playerGUID {
			if packet.Opcode == world.ChannelStart {
				rt.StartSpellChannel(id, duration)
			} else {
				rt.UpdateSpellChannel(duration)
			}
		}
	case world.SpellStart, world.SpellGo:
		cast, err := world.ParseSpellCastHeader(packet.Body)
		if err != nil {
			return true, err
		}
		if cast.Caster == playerGUID {
			if packet.Opcode == world.SpellStart {
				rt.StartSpellCast(cast)
			} else {
				rt.StopSpellCast(cast.SpellID, cast.Count, false)
			}
		}
	case world.CastFailed:
		if len(packet.Body) < 6 {
			return true, fmt.Errorf("SMSG_CAST_FAILED is truncated")
		}
		rt.StopSpellCast(binary.LittleEndian.Uint32(packet.Body[1:5]), packet.Body[0], true)
	case world.InitialSpells:
		initial, err := world.ParseInitialSpells(packet.Body)
		if err != nil {
			return true, err
		}
		rt.SetKnownSpells(initial.Spells)
		rt.SetSpellCooldowns(initial.Cooldowns)
	case world.ActionButtons:
		buttons, _, err := world.ParseActionButtons(packet.Body)
		if err != nil {
			return true, err
		}
		rt.SetActionButtons(buttons)
	case world.LearnedSpell, world.RemovedSpell:
		if len(packet.Body) < 4 {
			return true, fmt.Errorf("spell-list update is truncated")
		}
		rt.LearnSpell(binary.LittleEndian.Uint32(packet.Body), packet.Opcode == world.LearnedSpell)
	case world.SpellCooldowns:
		guid, cooldowns, err := world.ParseSpellCooldowns(packet.Body)
		if err != nil {
			return true, err
		}
		if guid == playerGUID {
			rt.SetSpellCooldowns(cooldowns)
		}
	default:
		return false, nil
	}
	return true, nil
}
