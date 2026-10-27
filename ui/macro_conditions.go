package ui

import (
	"strconv"
	"strings"
)

// parseCmdOptions evaluates the conditionals of a macro line
// ("[mod:shift,target=focus] one; [help] two; three") and returns the text of
// the first clause whose conditions hold, with the target it names.
func (rt *Runtime) parseCmdOptions(text string) (string, string) {
	for _, clause := range splitMacroClauses(text) {
		clause = strings.TrimSpace(clause)
		groups, rest := leadingConditionGroups(clause)
		if len(groups) == 0 {
			return rest, ""
		}
		for _, group := range groups {
			if ok, target := rt.conditionsHold(group); ok {
				return rest, target
			}
		}
	}
	return "", ""
}

// splitMacroClauses cuts a macro line at its semicolons outside brackets.
func splitMacroClauses(text string) []string {
	var clauses []string
	depth, start := 0, 0
	for index, char := range text {
		switch char {
		case '[':
			depth++
		case ']':
			if depth > 0 {
				depth--
			}
		case ';':
			if depth == 0 {
				clauses = append(clauses, text[start:index])
				start = index + 1
			}
		}
	}
	return append(clauses, text[start:])
}

// leadingConditionGroups takes the bracketed groups a clause starts with and
// returns them with the text after them.
func leadingConditionGroups(clause string) ([]string, string) {
	var groups []string
	for strings.HasPrefix(clause, "[") {
		end := strings.Index(clause, "]")
		if end < 0 {
			break
		}
		groups = append(groups, clause[1:end])
		clause = strings.TrimSpace(clause[end+1:])
	}
	return groups, clause
}

// conditionsHold evaluates the conditions of one bracketed group: every one of
// them must hold. It returns the target the group names, if any.
func (rt *Runtime) conditionsHold(group string) (bool, string) {
	target, named := "target", ""
	var conditions []string
	for _, part := range strings.Split(group, ",") {
		part = strings.TrimSpace(part)
		switch {
		case strings.HasPrefix(part, "@"):
			target, named = part[1:], part[1:]
		case strings.HasPrefix(strings.ToLower(part), "target="):
			target, named = part[len("target="):], part[len("target="):]
		case part != "":
			conditions = append(conditions, part)
		}
	}
	for _, condition := range conditions {
		name, argument := condition, ""
		if colon := strings.Index(condition, ":"); colon >= 0 {
			name, argument = condition[:colon], condition[colon+1:]
		}
		name = strings.ToLower(strings.TrimSpace(name))
		negate := false
		held, known := rt.condition(name, argument, target)
		if !known && strings.HasPrefix(name, "no") {
			held, known = rt.condition(name[2:], argument, target)
			negate = true
		}
		if !known {
			return false, ""
		}
		if held == negate {
			return false, ""
		}
	}
	return true, named
}

// listHas reports whether a value is among slash separated alternatives.
func listHas(list string, match func(string) bool) bool {
	for _, item := range strings.Split(list, "/") {
		if match(strings.TrimSpace(item)) {
			return true
		}
	}
	return false
}

func (rt *Runtime) unitFor(token string) *UnitInfo {
	if token == "" {
		token = "target"
	}
	return rt.unitInfo(token)
}

// condition evaluates one macro condition; known is false for a name that is not
// one.
func (rt *Runtime) condition(name, argument, target string) (held bool, known bool) {
	player := rt.unitInfo("player")
	unit := rt.unitFor(target)
	switch name {
	case "exists":
		return unit != nil && unit.Exists, true
	case "dead":
		return unit != nil && unit.Exists && unit.Dead, true
	case "harm":
		return unit != nil && unit.Exists && unit.Hostile, true
	case "help":
		return unit != nil && unit.Exists && !unit.Hostile, true
	case "party":
		return unit != nil && unit.Exists && rt.inPartyUnit(unit), true
	case "raid":
		return unit != nil && unit.Exists && rt.inRaidUnit(unit), true
	case "combat":
		return player != nil && player.InCombat, true
	case "mounted":
		return player != nil && player.Mounted, true
	case "stealth":
		return rt.playerHasAuraType(16), true
	case "swimming", "flying", "indoors", "outdoors", "flyable":
		swimming, flying, indoors := false, false, false
		if host := rt.controls(); host != nil {
			swimming, flying, indoors = host.MoveState()
		}
		switch name {
		case "swimming":
			return swimming, true
		case "flying":
			return flying, true
		case "indoors":
			return indoors, true
		case "outdoors":
			return !indoors, true
		}
		return !indoors, true
	case "group":
		switch strings.ToLower(argument) {
		case "party":
			return rt.group.in && !rt.isRaid(), true
		case "raid":
			return rt.isRaid(), true
		}
		return rt.group.in, true
	case "modifier", "mod":
		if argument == "" {
			return rt.modifierDown("SHIFT") || rt.modifierDown("CTRL") || rt.modifierDown("ALT"), true
		}
		return listHas(argument, func(item string) bool {
			parts := strings.Split(item, "+")
			for _, part := range parts {
				if !rt.modifierDown(strings.TrimSpace(part)) {
					return false
				}
			}
			return true
		}), true
	case "button", "btn":
		button := rt.lastButton
		numbers := map[string]string{"LeftButton": "1", "RightButton": "2", "MiddleButton": "3", "Button4": "4", "Button5": "5"}
		return listHas(argument, func(item string) bool {
			return strings.EqualFold(item, "any") || item == numbers[button]
		}), true
	case "actionbar", "bar":
		number, err := strconv.Atoi(strings.TrimSpace(argument))
		return err == nil && rt.gameplay.Page == number, true
	case "stance", "form":
		form := rt.shapeshiftForm()
		if argument == "" {
			return form != 0, true
		}
		return listHas(argument, func(item string) bool {
			number, err := strconv.Atoi(item)
			return err == nil && number == form
		}), true
	case "spec":
		return listHas(argument, func(item string) bool {
			number, err := strconv.Atoi(item)
			return err == nil && number-1 == rt.talents.active
		}), true
	case "pet":
		pet := rt.unitInfo("pet")
		if pet == nil || !pet.Exists {
			return false, true
		}
		return argument == "" || listHas(argument, func(item string) bool {
			return strings.EqualFold(item, pet.Name) || strings.EqualFold(item, pet.CreatureType)
		}), true
	case "channeling":
		casting := rt.gameplay.Cast.Channel && rt.gameplay.Cast.ID != 0
		if argument == "" || !casting {
			return casting, true
		}
		return listHas(argument, func(item string) bool {
			return strings.EqualFold(item, rt.gameplay.Catalog[rt.gameplay.Cast.ID].Name)
		}), true
	case "equipped", "worn":
		return listHas(argument, rt.equippedType), true
	case "vehicleui", "unithasvehicleui":
		return unit != nil && unit.Exists && false, true
	case "canexitvehicle":
		return false, true
	}
	return false, false
}

func (rt *Runtime) inPartyUnit(unit *UnitInfo) bool {
	if unit.GUID == rt.playerGUID() {
		return rt.group.in
	}
	for _, member := range rt.partyMembers() {
		if member.GUID == unit.GUID {
			return true
		}
	}
	return false
}

func (rt *Runtime) inRaidUnit(unit *UnitInfo) bool {
	if !rt.isRaid() {
		return false
	}
	if unit.GUID == rt.playerGUID() {
		return true
	}
	return rt.memberByGUID(unit.GUID) != nil
}

// playerHasAuraType reports whether an aura of an effect type is on the
// character.
func (rt *Runtime) playerHasAuraType(aura int) bool {
	player := rt.unitInfo("player")
	if player == nil {
		return false
	}
	for _, held := range player.Auras {
		for _, effect := range rt.gameplay.Catalog[held.SpellID].Effects {
			if effect.Aura == aura {
				return true
			}
		}
	}
	return false
}

// equippedType reports whether an item of a type is worn: its subclass name or
// the name of its slot ("Shields", "Two-Hand").
func (rt *Runtime) equippedType(want string) bool {
	invNames := map[uint32]string{13: "INVTYPE_WEAPON", 14: "INVTYPE_SHIELD", 15: "INVTYPE_RANGED", 17: "INVTYPE_2HWEAPON", 21: "INVTYPE_WEAPONMAINHAND", 22: "INVTYPE_WEAPONOFFHAND", 23: "INVTYPE_HOLDABLE", 26: "INVTYPE_RANGEDRIGHT"}
	for slot := 0; slot < 19; slot++ {
		template := rt.equippedTemplate(slot)
		if template == nil {
			continue
		}
		if name := rt.inventory.classes.SubClass[[2]uint32{template.Class, template.SubClass}]; strings.EqualFold(name, want) {
			return true
		}
		if global, ok := invNames[template.InventoryType]; ok && strings.EqualFold(rt.globalString(global, ""), want) {
			return true
		}
	}
	return false
}
