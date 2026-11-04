package ui

import (
	"strconv"
	"strings"
	"time"
)

// tradeLink is the recipes another player linked: whose they are, how far the
// skill goes and which of the recipes of the skill are known.
type tradeLink struct {
	guid      uint64
	name      string
	rank, max int
	bits      string
}

// has says whether the recipe at a place in the skill's list is in the link.
func (link *tradeLink) has(index int) bool {
	if index/6 >= len(link.bits) {
		return false
	}
	value := strings.IndexByte(tradeLinkDigits, link.bits[index/6])
	return value >= 0 && value&(1<<(index%6)) != 0
}

type pendingTradeLink struct {
	text  string
	guid  uint64
	since time.Time
}

// viewTradeLink opens the recipes a link holds: the text after "trade:", which
// is the spell, the rank, the most of the skill, the player and the recipes.
func (rt *Runtime) viewTradeLink(text string) {
	fields := strings.Split(text, ":")
	if len(fields) < 5 {
		return
	}
	guid, err := strconv.ParseUint(fields[3], 16, 64)
	if err != nil || guid == 0 {
		return
	}
	if rt.nameOf(guid) == "" {
		rt.requestName(guid)
		rt.tradeSkill.pendingLink = &pendingTradeLink{text: text, guid: guid, since: time.Now()}
		return
	}
	rt.showTradeLink(text, guid)
}

// flushTradeLink opens the link that waited for the name of its player.
func (rt *Runtime) flushTradeLink() {
	pending := rt.tradeSkill.pendingLink
	if pending == nil {
		return
	}
	if rt.nameOf(pending.guid) != "" {
		rt.tradeSkill.pendingLink = nil
		rt.showTradeLink(pending.text, pending.guid)
	} else if time.Since(pending.since) > 10*time.Second {
		rt.tradeSkill.pendingLink = nil
	}
}

func (rt *Runtime) showTradeLink(text string, guid uint64) {
	fields := strings.Split(text, ":")
	spell, _ := strconv.Atoi(fields[0])
	rank, _ := strconv.Atoi(fields[1])
	maximum, _ := strconv.Atoi(fields[2])
	info, ok := rt.gameplay.Catalog[uint32(spell)]
	if !ok || !isTradeSkillSpell(info) || info.SkillLine == 0 {
		return
	}
	bits := fields[len(fields)-1]
	abilities := rt.skillAbilities[info.SkillLine]
	if len(abilities) == 0 || (len(abilities)+5)/6 != len(bits) {
		return
	}
	state := &rt.tradeSkill
	wasOpen := state.open
	state.line, state.spell = info.SkillLine, uint32(spell)
	state.linked = &tradeLink{guid: guid, name: rt.nameOf(guid), rank: rank, max: maximum, bits: bits}
	state.selected = 0
	state.nameFilter, state.minLevel, state.maxLevel, state.onlyMakeable = "", 0, 0, false
	state.subclassOff, state.slotOff = nil, nil
	state.collapsed = map[string]bool{}
	state.repeatLeft = 0
	state.open = true
	if wasOpen {
		rt.FireEvent("TRADE_SKILL_UPDATE")
		return
	}
	rt.FireEvent("TRADE_SKILL_SHOW")
}
