package ui

import "fmt"

// SkillAbility is a spell a skill teaches and the races and classes it is for.
type SkillAbility struct {
	Spell, RaceMask, ClassMask uint32
}

// SetSkillAbilities gives the spells each skill teaches.
func (rt *Runtime) SetSkillAbilities(abilities map[uint32][]SkillAbility) {
	rt.skillAbilities = abilities
}

// tradeLinkDigits are the characters a link writes six recipes at a time in.
const tradeLinkDigits = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789+/"

// tradeSkillListLink is the link to the recipes of the profession in view: its
// spell, the rank and the most of the skill, whose link it is, and which of the
// recipes of the skill are known.
func (rt *Runtime) tradeSkillListLink() (string, bool) {
	state := &rt.tradeSkill
	info, found := rt.gameplay.Catalog[state.spell]
	if !state.open || !found {
		return "", false
	}
	var entry *SkillEntry
	for index := range rt.skills.entries {
		if rt.skills.entries[index].ID == state.line {
			entry = &rt.skills.entries[index]
		}
	}
	player := rt.unitInfo("player")
	abilities := rt.skillAbilities[state.line]
	if entry == nil || player == nil || len(abilities) == 0 {
		return "", false
	}
	rank, maximum := skillCurrent(entry)
	link := fmt.Sprintf("|cffffd000|Htrade:%d:%d:%d:%X:", state.spell, rank, maximum, rt.playerGUID())
	bits, place := 0, 0
	for _, ability := range abilities {
		if rt.gameplay.Known[ability.Spell] &&
			(ability.RaceMask == 0 || ability.RaceMask&(1<<(uint(player.RaceID)-1)) != 0) &&
			(ability.ClassMask == 0 || ability.ClassMask&(1<<(uint(player.ClassID)-1)) != 0) {
			bits |= 1 << place
		}
		if place++; place > 5 {
			link += string(tradeLinkDigits[bits])
			bits, place = 0, 0
		}
	}
	if place != 0 {
		link += string(tradeLinkDigits[bits])
	}
	return link + fmt.Sprintf("|h[%s]|h|r", info.Name), true
}
