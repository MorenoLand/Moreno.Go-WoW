package ui

import (
	"fmt"
	"regexp"
	"strings"
	"time"

	lua "github.com/yuin/gopher-lua"
)

var (
	tooltipYellow = rgba{r: 1, g: 0.82, b: 0, a: 1}
	tooltipWhite  = rgba{r: 1, g: 1, b: 1, a: 1}
	tooltipGray   = rgba{r: 0.5, g: 0.5, b: 0.5, a: 1}
)

// globalString is a FrameXML global string (GlobalStrings.lua), or the
// fallback when the string is not defined.
func (rt *Runtime) globalString(name, fallback string) string {
	if value, ok := rt.L.GetGlobal(name).(lua.LString); ok {
		return string(value)
	}
	return fallback
}

func (rt *Runtime) spellTextContext() spellTextContext {
	level := 0
	if info := rt.unitInfo("player"); info != nil {
		level = info.Level
	}
	return spellTextContext{
		catalog: rt.gameplay.Catalog,
		known:   func(id uint32) bool { return rt.gameplay.Known[id] },
		level:   level,
		male:    true,
	}
}

func (rt *Runtime) powerCostText(info SpellInfo) string {
	if info.Cost <= 0 {
		return ""
	}
	names := map[int][2]string{
		0: {"MANA_COST", "%d Mana"},
		1: {"RAGE_COST", "%d Rage"},
		2: {"FOCUS_COST", "%d Focus"},
		3: {"ENERGY_COST", "%d Energy"},
		5: {"RUNE_COST", "%d Rune"},
		6: {"RUNIC_POWER_COST", "%d Runic Power"},
	}
	entry, ok := names[info.PowerType]
	if !ok {
		return ""
	}
	return strings.ReplaceAll(rt.globalString(entry[0], entry[1]), "%d", fmt.Sprint(info.Cost))
}

func (rt *Runtime) spellRangeText(info SpellInfo) string {
	switch {
	case info.MaxRange <= 0:
		return ""
	case info.MaxRange <= 5:
		return rt.globalString("MELEE_RANGE", "Melee Range")
	}
	return strings.ReplaceAll(rt.globalString("SPELL_RANGE", "%s yd range"), "%s", trimNumber(info.MaxRange))
}

func (rt *Runtime) spellCastText(info SpellInfo) string {
	if info.CastTime <= 0 {
		return rt.globalString("SPELL_CAST_TIME_INSTANT", "Instant cast")
	}
	seconds := float64(info.CastTime) / 1000
	if seconds >= 60 {
		return substituteNumberFormat(rt.globalString("SPELL_CAST_TIME_MIN", "%.2g min cast"), seconds/60)
	}
	return substituteNumberFormat(rt.globalString("SPELL_CAST_TIME_SEC", "%.2g sec cast"), seconds)
}

func (rt *Runtime) spellCooldownText(info SpellInfo) string {
	ms := info.CooldownMS
	if info.CategoryCooldownMS > ms {
		ms = info.CategoryCooldownMS
	}
	if ms <= 0 {
		return ""
	}
	seconds := float64(ms) / 1000
	if seconds >= 60 {
		return substituteNumberFormat(rt.globalString("SPELL_RECAST_TIME_MIN", "%.3g min cooldown"), seconds/60)
	}
	return substituteNumberFormat(rt.globalString("SPELL_RECAST_TIME_SEC", "%.3g sec cooldown"), seconds)
}

// fillSpellTooltip writes the spell's tooltip lines: name and rank, cost and
// range, cast time and cooldown, then the description.
func (rt *Runtime) fillSpellTooltip(w *widget, id uint32) bool {
	info, ok := rt.gameplay.Catalog[id]
	if !ok || info.Name == "" {
		return false
	}
	rt.clearTooltip(w)
	state := w.tooltip()
	state.spellID = id
	state.spellKind = "spell"
	header := tooltipLine{left: info.Name}
	if info.Rank != "" {
		header.right, header.rightColor, header.hasRight = info.Rank, tooltipGray, true
	}
	if info.Attributes&0x40 != 0 {
		header.right = rt.globalString("SPELL_PASSIVE", "Passive")
		header.rightColor, header.hasRight = tooltipGray, true
	}
	rt.tooltipAddLine(w, header)
	if info.Attributes&0x40 == 0 {
		cost, spellRange := rt.powerCostText(info), rt.spellRangeText(info)
		if cost != "" || spellRange != "" {
			rt.tooltipAddLine(w, tooltipLine{left: cost, right: spellRange, leftColor: tooltipWhite, rightColor: tooltipWhite, hasLeft: true, hasRight: true})
		}
		if info.Attributes&0x10 == 0 {
			rt.tooltipAddLine(w, tooltipLine{left: rt.spellCastText(info), right: rt.spellCooldownText(info), leftColor: tooltipWhite, rightColor: tooltipWhite, hasLeft: true, hasRight: true})
		}
	}
	if description := rt.spellTextContext().expand(info.Description, info); description != "" {
		rt.tooltipAddLine(w, tooltipLine{left: description, leftColor: tooltipYellow, hasLeft: true, wrap: true})
	}
	rt.showTooltip(w)
	return true
}

func (rt *Runtime) fillAuraTooltip(w *widget, aura AuraInfo) {
	rt.clearTooltip(w)
	state := w.tooltip()
	state.spellID = aura.SpellID
	state.spellKind = "aura"
	header := tooltipLine{left: aura.Name}
	if aura.Harmful && aura.DebuffType != "" && aura.DebuffType != "none" {
		header.right = aura.DebuffType
		header.rightColor, header.hasRight = tooltipDebuffColor(aura.DebuffType), true
	}
	rt.tooltipAddLine(w, header)
	if info, ok := rt.gameplay.Catalog[aura.SpellID]; ok {
		text := info.AuraDescription
		if text == "" {
			text = info.Description
		}
		if text = rt.spellTextContext().expand(text, info); text != "" {
			rt.tooltipAddLine(w, tooltipLine{left: text, leftColor: tooltipYellow, hasLeft: true, wrap: true})
		}
	}
	if aura.ExpirationTime > 0 {
		remaining := aura.ExpirationTime - time.Since(rt.started).Seconds()
		if remaining < 0 {
			remaining = aura.ExpirationTime
		}
		if remaining > 0 {
			rt.tooltipAddLine(w, tooltipLine{left: rt.auraTimeRemaining(remaining), leftColor: tooltipWhite, hasLeft: true})
		}
	}
	rt.showTooltip(w)
}

func tooltipDebuffColor(kind string) rgba {
	switch strings.ToLower(kind) {
	case "magic":
		return rgba{r: 0.2, g: 0.6, b: 1, a: 1}
	case "curse":
		return rgba{r: 0.6, g: 0, b: 1, a: 1}
	case "disease":
		return rgba{r: 0.6, g: 0.4, b: 0, a: 1}
	case "poison":
		return rgba{r: 0, g: 0.6, b: 0, a: 1}
	}
	return tooltipGray
}

// auraAt returns the index'th aura of a unit that passes the filter.
func (rt *Runtime) auraAt(unit string, index int, filter string) (AuraInfo, bool) {
	info := rt.unitInfo(unit)
	if info == nil || !info.Exists || index < 1 {
		return AuraInfo{}, false
	}
	wantHarmful := strings.Contains(strings.ToUpper(filter), "HARMFUL")
	wantHelpful := strings.Contains(strings.ToUpper(filter), "HELPFUL") || !wantHarmful
	position := 0
	for _, aura := range info.Auras {
		if (wantHarmful && !aura.Harmful) || (wantHelpful && aura.Harmful) {
			continue
		}
		position++
		if position == index {
			return aura, true
		}
	}
	return AuraInfo{}, false
}

func (rt *Runtime) fillUnitTooltip(w *widget, unit string) bool {
	info := rt.unitInfo(unit)
	if info == nil || !info.Exists {
		return false
	}
	rt.clearTooltip(w)
	w.tooltip().unit = unit
	rt.tooltipAddLine(w, tooltipLine{left: info.Name})
	if info.Level > 0 {
		line := fmt.Sprintf("%s %d", rt.globalString("LEVEL", "Level"), info.Level)
		for _, part := range []string{info.RaceName, info.ClassName} {
			if part != "" {
				line += " " + part
			}
		}
		rt.tooltipAddLine(w, tooltipLine{left: line, leftColor: tooltipWhite, hasLeft: true})
	}
	rt.showTooltip(w)
	rt.fire(w, "OnTooltipSetUnit", []lua.LValue{w.luaValue(rt.L)})
	return true
}

// tooltipContentMethods are the Set* methods that fill a tooltip from game
// data (units, spells, auras).
func (rt *Runtime) tooltipContentMethods() map[string]func(L *lua.LState, w *widget) int {
	aura := func(unitArg, indexArg, filterArg int, fixedFilter string) func(L *lua.LState, w *widget) int {
		return func(L *lua.LState, w *widget) int {
			unit := "player"
			if unitArg > 0 {
				unit = L.CheckString(unitArg)
			}
			filter := fixedFilter
			if filterArg > 0 {
				filter = L.OptString(filterArg, fixedFilter)
			}
			if found, ok := rt.auraAt(unit, L.CheckInt(indexArg), filter); ok {
				rt.fillAuraTooltip(w, found)
			}
			return 0
		}
	}
	noop := func(L *lua.LState, w *widget) int { return 0 }
	methods := map[string]func(L *lua.LState, w *widget) int{
		"SetUnitBuff":   aura(2, 3, 0, "HELPFUL"),
		"SetUnitDebuff": aura(2, 3, 0, "HARMFUL"),
		"SetUnitAura":   aura(2, 3, 4, "HELPFUL"),
		"SetPlayerBuff": aura(0, 2, 0, "HELPFUL"),
		"SetUnit": func(L *lua.LState, w *widget) int {
			rt.fillUnitTooltip(w, L.CheckString(2))
			return 0
		},
		"SetSpell": func(L *lua.LState, w *widget) int {
			rt.fillSpellTooltip(w, rt.spellForTooltipCall(L, 2, 3))
			return 0
		},
		"SetSpellBookItem": func(L *lua.LState, w *widget) int {
			rt.fillSpellTooltip(w, rt.spellForTooltipCall(L, 2, 3))
			return 0
		},
		"SetAction": func(L *lua.LState, w *widget) int {
			button := rt.action(L.CheckInt(2))
			if button.ID != 0 && button.Type == 0 {
				rt.fillSpellTooltip(w, button.ID)
			}
			return 0
		},
		"SetShapeshift": noop,
		"SetPetAction":  noop,
		"SetTracking":   noop,
	}
	for name, fn := range rt.itemTooltipMethods() {
		methods[name] = fn
	}
	return methods
}

// spellForTooltipCall resolves the (slot, bookType) or (spellId) arguments of
// the spell tooltip setters to a spell id.
func (rt *Runtime) spellForTooltipCall(L *lua.LState, slotArg, bookArg int) uint32 {
	value := L.Get(slotArg)
	book := L.OptString(bookArg, "spell")
	if value.Type() != lua.LTNumber {
		return 0
	}
	id := uint32(L.CheckInt(slotArg))
	if book == "pet" {
		return 0
	}
	if bookArg > 0 && L.Get(bookArg).Type() == lua.LTString && id >= 1 && int(id) <= len(rt.gameplay.Book) {
		return rt.gameplay.Book[id-1]
	}
	return id
}

// expandPluralMarkup resolves "|4singular:plural;" in a string for a number.
func expandPluralMarkup(text string, number int) string {
	for {
		start := strings.Index(text, "|4")
		if start < 0 {
			return text
		}
		end := strings.Index(text[start:], ";")
		if end < 0 {
			return text
		}
		parts := strings.SplitN(text[start+2:start+end], ":", 2)
		choice := parts[0]
		if len(parts) == 2 && number != 1 {
			choice = parts[1]
		}
		text = text[:start] + choice + text[start+end+1:]
	}
}

// auraTimeRemaining writes how long an aura has left, as the buff tooltip does.
func (rt *Runtime) auraTimeRemaining(seconds float64) string {
	switch {
	case seconds >= 86400:
		days := int(seconds / 86400)
		return expandPluralMarkup(strings.ReplaceAll(rt.globalString("SPELL_TIME_REMAINING_DAYS", "%d |4day:days; remaining"), "%d", fmt.Sprint(days)), days)
	case seconds >= 3600:
		hours := int(seconds / 3600)
		return expandPluralMarkup(strings.ReplaceAll(rt.globalString("SPELL_TIME_REMAINING_HOURS", "%d |4hour:hours; remaining"), "%d", fmt.Sprint(hours)), hours)
	case seconds >= 60:
		minutes := int(seconds / 60)
		return expandPluralMarkup(strings.ReplaceAll(rt.globalString("SPELL_TIME_REMAINING_MIN", "%d |4minute:minutes; remaining"), "%d", fmt.Sprint(minutes)), minutes)
	}
	count := int(seconds)
	return expandPluralMarkup(strings.ReplaceAll(rt.globalString("SPELL_TIME_REMAINING_SEC", "%d |4second:seconds; remaining"), "%d", fmt.Sprint(count)), count)
}

var numberFormatPattern = regexp.MustCompile(`%[0-9]*(\.[0-9]+)?[dfgi]`)

// substituteNumberFormat fills the first numeric conversion (%.3g, %d, ...) of a
// localized string with value, the way string.format would.
func substituteNumberFormat(format string, value float64) string {
	done := false
	return numberFormatPattern.ReplaceAllStringFunc(format, func(spec string) string {
		if done {
			return spec
		}
		done = true
		switch spec[len(spec)-1] {
		case 'd', 'i':
			return fmt.Sprintf("%d", int64(value))
		case 'f':
			return fmt.Sprintf(spec, value)
		}
		return fmt.Sprintf(spec, value)
	})
}
