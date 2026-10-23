package ui

import (
	"fmt"
	"strings"

	lua "github.com/yuin/gopher-lua"
)

// UnitInfo is the C-API unit snapshot FrameXML unit frames read through Unit*
// and SetPortraitTexture. Values come from glue character data and later world
// sync; TemporaryPortrait paths follow Wow.exe PortraitButton.cpp:
// Interface\CharacterFrame\TemporaryPortrait-%s-%s with Sex then RaceFile
// (listfile: TemporaryPortrait-Male-Human.blp).
type UnitInfo struct {
	GUID       uint64
	Exists     bool
	Name       string
	Server     string
	Level      int
	RaceID     int
	RaceFile   string
	RaceName   string
	ClassID    int
	ClassFile  string
	ClassName  string
	Sex        int // UnitSex: 2 male, 3 female
	Health     int
	HealthMax  int
	Power      int
	PowerMax   int
	PowerType  int
	PowerToken string
	Connected  bool
	Player     bool
	Dead       bool
	Visible    bool
	XP, XPMax  int
	// Hostile and Friendly are the unit's reaction towards the player; neither
	// means neutral.
	Hostile, Friendly bool
	InCombat          bool
	Mounted           bool
	// Classification is the UnitClassification string ("normal", "elite", ...).
	Classification string
	CreatureType   string
	Auras          []AuraInfo
}

type AuraInfo struct {
	Name              string
	Rank              string
	Texture           string
	Count             int
	DebuffType        string
	Duration          float64
	ExpirationTime    float64
	Caster            string
	CanStealOrPurge   bool
	ShouldConsolidate bool
	Harmful           bool
	SpellID           uint32
}

func (rt *Runtime) ensureUnits() {
	if rt.units == nil {
		rt.units = make(map[string]*UnitInfo)
	}
}

func (rt *Runtime) SetUnit(unit string, info UnitInfo) {
	if rt == nil {
		return
	}
	rt.ensureUnits()
	unit = strings.ToLower(strings.TrimSpace(unit))
	cp := info
	cp.Auras = append([]AuraInfo(nil), info.Auras...)
	rt.units[unit] = &cp
	if rt.widgets["BuffFrame"] != nil {
		rt.FireEvent("UNIT_AURA", lua.LString(unit))
	}
}

// SetTarget replaces the "target" unit and fires PLAYER_TARGET_CHANGED; a nil
// info clears it.
func (rt *Runtime) SetTarget(info *UnitInfo) {
	if rt == nil {
		return
	}
	if info == nil {
		rt.ClearUnit("target")
	} else {
		rt.SetUnit("target", *info)
	}
	rt.FireEvent("PLAYER_TARGET_CHANGED")
}

func (rt *Runtime) ClearUnit(unit string) {
	if rt == nil || rt.units == nil {
		return
	}
	delete(rt.units, strings.ToLower(strings.TrimSpace(unit)))
}

func (rt *Runtime) ApplyUnitFields(unit string, fields map[uint16]uint32) bool {
	info := rt.unitInfo(unit)
	if info == nil {
		return false
	}
	old := *info
	if value, ok := fields[0x17]; ok {
		info.PowerType = int(value >> 24)
		tokens := []string{"MANA", "RAGE", "FOCUS", "ENERGY", "HAPPINESS", "RUNES", "RUNIC_POWER"}
		if info.PowerType < len(tokens) {
			info.PowerToken = tokens[info.PowerType]
		}
	}
	if value, ok := fields[0x18]; ok {
		info.Health = int(value)
		info.Dead = value == 0
	}
	if value, ok := fields[0x20]; ok {
		info.HealthMax = int(value)
	}
	if info.PowerType >= 0 && info.PowerType < 7 {
		if value, ok := fields[uint16(0x19+info.PowerType)]; ok {
			info.Power = int(value)
		}
		if value, ok := fields[uint16(0x21+info.PowerType)]; ok {
			info.PowerMax = int(value)
		}
	}
	if value, ok := fields[0x36]; ok {
		info.Level = int(value)
	}
	if value, ok := fields[634]; ok {
		info.XP = int(value)
	}
	if value, ok := fields[635]; ok {
		info.XPMax = int(value)
	}
	if value, ok := fields[0x3B]; ok {
		info.InCombat = value&0x80000 != 0
	}
	if value, ok := fields[0x45]; ok {
		info.Mounted = value != 0
	}
	changed := false
	for _, entry := range []struct {
		Event   string
		Changed bool
	}{{"UNIT_HEALTH", old.Health != info.Health}, {"UNIT_MAXHEALTH", old.HealthMax != info.HealthMax}, {"UNIT_DISPLAYPOWER", old.PowerType != info.PowerType}, {"UNIT_" + info.PowerToken, old.Power != info.Power}, {"UNIT_MAX" + info.PowerToken, old.PowerMax != info.PowerMax}, {"UNIT_LEVEL", old.Level != info.Level}} {
		if entry.Changed {
			changed = true
			rt.FireEvent(entry.Event, lua.LString(unit))
		}
	}
	if old.XP != info.XP || old.XPMax != info.XPMax {
		changed = true
		rt.FireEvent("PLAYER_XP_UPDATE", lua.LString(unit))
	}
	return changed
}

func (rt *Runtime) SetUnitAuras(unit string, auras []AuraInfo) {
	if rt == nil {
		return
	}
	info := rt.unitInfo(unit)
	if info == nil {
		return
	}
	info.Auras = append([]AuraInfo(nil), auras...)
	if rt.widgets["BuffFrame"] != nil {
		rt.FireEvent("UNIT_AURA", lua.LString(strings.ToLower(strings.TrimSpace(unit))))
	}
}

// UnitGUID returns the GUID stored for a unit token, or zero.
func (rt *Runtime) UnitGUID(unit string) uint64 {
	if info := rt.unitInfo(unit); info != nil && info.Exists {
		return info.GUID
	}
	return 0
}

// RaceFileFromID and ClassFileFromID name the race and class stored in a
// player unit's bytes-0 update field, for portrait paths and class colours.
func RaceFileFromID(id int) string  { return raceFileFromID(id) }
func ClassFileFromID(id int) string { return classFileFromID(id) }

func (rt *Runtime) unitInfo(unit string) *UnitInfo {
	if rt == nil || rt.units == nil {
		return nil
	}
	return rt.units[strings.ToLower(strings.TrimSpace(unit))]
}

func raceFileFromID(id int) string {
	if name, ok := map[int]string{
		1: "Human", 2: "Orc", 3: "Dwarf", 4: "NightElf", 5: "Scourge",
		6: "Tauren", 7: "Gnome", 8: "Troll", 10: "BloodElf", 11: "Draenei",
	}[id]; ok {
		return name
	}
	return ""
}

func raceFileFromToken(race string) string {
	token := strings.ToUpper(strings.TrimSpace(race))
	token = strings.TrimPrefix(token, "RACE_")
	switch token {
	case "HUMAN":
		return "Human"
	case "ORC":
		return "Orc"
	case "DWARF":
		return "Dwarf"
	case "NIGHTELF", "NIGHT_ELF":
		return "NightElf"
	case "SCOURGE", "UNDEAD":
		return "Scourge"
	case "TAUREN":
		return "Tauren"
	case "GNOME":
		return "Gnome"
	case "TROLL":
		return "Troll"
	case "BLOODELF", "BLOOD_ELF":
		return "BloodElf"
	case "DRAENEI":
		return "Draenei"
	default:
		return raceFileFromID(atoiDefault(race, 0))
	}
}

func classFileFromID(id int) string {
	if name, ok := map[int]string{
		1: "WARRIOR", 2: "PALADIN", 3: "HUNTER", 4: "ROGUE", 5: "PRIEST",
		6: "DEATHKNIGHT", 7: "SHAMAN", 8: "MAGE", 9: "WARLOCK", 11: "DRUID",
	}[id]; ok {
		return name
	}
	return ""
}

func classFileFromToken(class string) string {
	token := strings.ToUpper(strings.TrimSpace(class))
	if token == "" {
		return ""
	}
	if f := classFileFromID(atoiDefault(class, 0)); f != "" {
		return f
	}
	return token
}

func defaultPowerForClass(classFile string) (powerType int, token string, powerMax int) {
	switch strings.ToUpper(classFile) {
	case "WARRIOR":
		return 1, "RAGE", 100
	case "ROGUE":
		return 3, "ENERGY", 100
	case "DEATHKNIGHT":
		return 6, "RUNIC_POWER", 100
	default:
		return 0, "MANA", 100
	}
}

func unitSexFromGender(gender int) int {
	switch gender {
	case 0, 2:
		return 2
	case 1, 3:
		return 3
	default:
		return 2
	}
}

func portraitSexToken(sex int) string {
	if sex == 3 {
		return "Female"
	}
	return "Male"
}

func atoiDefault(s string, def int) int {
	s = strings.TrimSpace(s)
	if s == "" {
		return def
	}
	n := 0
	for _, r := range s {
		if r < '0' || r > '9' {
			return def
		}
		n = n*10 + int(r-'0')
	}
	return n
}

func (rt *Runtime) temporaryPortraitPath(unit string) string {
	info := rt.unitInfo(unit)
	if info == nil || !info.Exists {
		return `Interface\CharacterFrame\TemporaryPortrait`
	}
	if strings.EqualFold(unit, "pet") && !info.Player && info.RaceFile == "" {
		return `Interface\CharacterFrame\TemporaryPortrait-Pet`
	}
	race := info.RaceFile
	if race == "" {
		return `Interface\CharacterFrame\TemporaryPortrait-Monster`
	}
	return fmt.Sprintf(`Interface\CharacterFrame\TemporaryPortrait-%s-%s`, portraitSexToken(info.Sex), race)
}

func (rt *Runtime) resolveTextureArg(L *lua.LState, idx int) *widget {
	switch v := L.Get(idx).(type) {
	case *lua.LUserData:
		if w, ok := v.Value.(*widget); ok {
			return w
		}
	case lua.LString:
		if w := rt.widgets[v.String()]; w != nil {
			return w
		}
	}
	return nil
}

func (rt *Runtime) applyPortraitTexture(tex *widget, unit string) {
	if tex == nil {
		return
	}
	tex.textureFile = rt.temporaryPortraitPath(unit)
	tex.portraitUnit = unit
	tex.shown = true
}

func (rt *Runtime) applyPortraitToTexture(tex *widget, path string) {
	if tex == nil {
		return
	}
	path = strings.TrimSpace(path)
	if path == "" {
		return
	}
	tex.textureFile = path
	tex.portraitUnit = ""
	tex.shown = true
}

// SeedPlayerUnitFromGlue fills the "player" unit from the selected glue character.
func (rt *Runtime) SeedPlayerUnitFromGlue() bool {
	if rt == nil {
		return false
	}
	idx := rt.Glue.SelectedCharacter
	if idx < 1 || idx > len(rt.Glue.Characters) {
		if len(rt.Glue.Characters) == 0 {
			return false
		}
		idx = 1
	}
	c := rt.Glue.Characters[idx-1]
	raceFile := raceFileFromID(c.RaceID)
	if raceFile == "" {
		raceFile = raceFileFromToken(c.Race)
	}
	classFile := classFileFromID(c.ClassID)
	if classFile == "" {
		classFile = classFileFromToken(c.Class)
	}
	powerType, powerToken, powerMax := defaultPowerForClass(classFile)
	level := c.Level
	if level < 1 {
		level = 1
	}
	rt.SetUnit("player", UnitInfo{
		Exists:     true,
		Name:       c.Name,
		Level:      level,
		RaceID:     c.RaceID,
		RaceFile:   raceFile,
		RaceName:   c.Race,
		ClassID:    c.ClassID,
		ClassFile:  classFile,
		ClassName:  c.Class,
		Sex:        unitSexFromGender(c.Gender),
		Health:     1,
		HealthMax:  1,
		Power:      powerMax,
		PowerMax:   powerMax,
		PowerType:  powerType,
		PowerToken: powerToken,
		Connected:  true,
		Player:     true,
		Visible:    true,
	})
	return true
}

func registerUnitAPI(rt *Runtime) {
	L := rt.L
	reg := func(name string, fn lua.LGFunction) {
		L.SetGlobal(name, L.NewFunction(func(L *lua.LState) int {
			defer func() {
				if r := recover(); r != nil {
					L.RaiseError("go panic in %s: %v", name, r)
				}
			}()
			return fn(L)
		}))
	}

	reg("UnitExists", func(L *lua.LState) int {
		info := rt.unitInfo(L.OptString(1, ""))
		L.Push(lua.LBool(info != nil && info.Exists))
		return 1
	})
	reg("UnitName", func(L *lua.LState) int {
		info := rt.unitInfo(L.OptString(1, ""))
		if info == nil || !info.Exists {
			L.Push(lua.LNil)
			L.Push(lua.LNil)
			return 2
		}
		L.Push(lua.LString(info.Name))
		if info.Server != "" {
			L.Push(lua.LString(info.Server))
		} else {
			L.Push(lua.LNil)
		}
		return 2
	})
	reg("UnitPVPName", func(L *lua.LState) int {
		info := rt.unitInfo(L.OptString(1, ""))
		if info == nil || !info.Exists {
			L.Push(lua.LNil)
			return 1
		}
		L.Push(lua.LString(info.Name))
		return 1
	})
	reg("GetUnitName", func(L *lua.LState) int {
		info := rt.unitInfo(L.OptString(1, ""))
		if info == nil || !info.Exists {
			L.Push(lua.LNil)
			return 1
		}
		L.Push(lua.LString(info.Name))
		return 1
	})
	reg("UnitLevel", func(L *lua.LState) int {
		info := rt.unitInfo(L.OptString(1, ""))
		if info == nil || !info.Exists {
			L.Push(lua.LNumber(0))
			return 1
		}
		L.Push(lua.LNumber(info.Level))
		return 1
	})
	reg("UnitXP", func(L *lua.LState) int {
		L.Push(lua.LNumber(0))
		return 1
	})
	reg("UnitXPMax", func(L *lua.LState) int {
		L.Push(lua.LNumber(1))
		return 1
	})
	reg("UnitHealth", func(L *lua.LState) int {
		info := rt.unitInfo(L.OptString(1, ""))
		if info == nil {
			L.Push(lua.LNumber(0))
			return 1
		}
		L.Push(lua.LNumber(info.Health))
		return 1
	})
	reg("UnitHealthMax", func(L *lua.LState) int {
		info := rt.unitInfo(L.OptString(1, ""))
		if info == nil {
			L.Push(lua.LNumber(0))
			return 1
		}
		L.Push(lua.LNumber(info.HealthMax))
		return 1
	})
	reg("UnitPower", func(L *lua.LState) int {
		info := rt.unitInfo(L.OptString(1, ""))
		if info == nil {
			L.Push(lua.LNumber(0))
			return 1
		}
		L.Push(lua.LNumber(info.Power))
		return 1
	})
	reg("UnitPowerMax", func(L *lua.LState) int {
		info := rt.unitInfo(L.OptString(1, ""))
		if info == nil {
			L.Push(lua.LNumber(0))
			return 1
		}
		L.Push(lua.LNumber(info.PowerMax))
		return 1
	})
	reg("UnitPowerType", func(L *lua.LState) int {
		info := rt.unitInfo(L.OptString(1, ""))
		if info == nil || !info.Exists {
			L.Push(lua.LNumber(0))
			L.Push(lua.LString("MANA"))
			return 2
		}
		L.Push(lua.LNumber(info.PowerType))
		L.Push(lua.LString(info.PowerToken))
		return 2
	})
	reg("UnitClass", func(L *lua.LState) int {
		info := rt.unitInfo(L.OptString(1, ""))
		if info == nil || !info.Exists {
			L.Push(lua.LNil)
			L.Push(lua.LNil)
			return 2
		}
		name := info.ClassName
		if name == "" {
			name = info.ClassFile
		}
		L.Push(lua.LString(name))
		L.Push(lua.LString(info.ClassFile))
		return 2
	})
	reg("UnitRace", func(L *lua.LState) int {
		info := rt.unitInfo(L.OptString(1, ""))
		if info == nil || !info.Exists {
			L.Push(lua.LNil)
			L.Push(lua.LNil)
			return 2
		}
		name := info.RaceName
		if name == "" {
			name = info.RaceFile
		}
		L.Push(lua.LString(name))
		L.Push(lua.LString(info.RaceFile))
		return 2
	})
	reg("UnitSex", func(L *lua.LState) int {
		info := rt.unitInfo(L.OptString(1, ""))
		if info == nil || !info.Exists {
			L.Push(lua.LNumber(1))
			return 1
		}
		L.Push(lua.LNumber(info.Sex))
		return 1
	})
	reg("UnitIsConnected", func(L *lua.LState) int {
		info := rt.unitInfo(L.OptString(1, ""))
		L.Push(lua.LBool(info != nil && info.Exists && info.Connected))
		return 1
	})
	reg("UnitIsPlayer", func(L *lua.LState) int {
		info := rt.unitInfo(L.OptString(1, ""))
		L.Push(lua.LBool(info != nil && info.Exists && info.Player))
		return 1
	})
	reg("UnitPlayerControlled", func(L *lua.LState) int {
		info := rt.unitInfo(L.OptString(1, ""))
		L.Push(lua.LBool(info != nil && info.Exists && info.Player))
		return 1
	})
	reg("UnitIsUnit", func(L *lua.LState) int {
		a := strings.ToLower(L.OptString(1, ""))
		b := strings.ToLower(L.OptString(2, ""))
		same := a != "" && a == b
		if !same {
			ga, gb := rt.UnitGUID(a), rt.UnitGUID(b)
			same = ga != 0 && ga == gb
		}
		L.Push(lua.LBool(same))
		return 1
	})
	reg("UnitIsDead", func(L *lua.LState) int {
		info := rt.unitInfo(L.OptString(1, ""))
		L.Push(lua.LBool(info != nil && info.Dead))
		return 1
	})
	reg("UnitIsDeadOrGhost", func(L *lua.LState) int {
		info := rt.unitInfo(L.OptString(1, ""))
		L.Push(lua.LBool(info != nil && info.Dead))
		return 1
	})
	reg("UnitIsGhost", func(L *lua.LState) int {
		info := rt.unitInfo(L.OptString(1, ""))
		L.Push(lua.LBool(info != nil && info.Dead))
		return 1
	})
	reg("UnitIsVisible", func(L *lua.LState) int {
		info := rt.unitInfo(L.OptString(1, ""))
		L.Push(lua.LBool(info != nil && info.Exists && info.Visible))
		return 1
	})
	reg("UnitIsCharmed", func(L *lua.LState) int {
		L.Push(lua.LBool(false))
		return 1
	})
	reg("UnitClassification", func(L *lua.LState) int {
		L.Push(lua.LString("normal"))
		return 1
	})
	reg("UnitCanAttack", func(L *lua.LState) int {
		L.Push(lua.LBool(false))
		return 1
	})
	reg("UnitCanAssist", func(L *lua.LState) int {
		a := strings.ToLower(L.OptString(1, ""))
		b := strings.ToLower(L.OptString(2, ""))
		L.Push(lua.LBool(a != "" && a == b))
		return 1
	})
	reg("UnitIsFriend", func(L *lua.LState) int {
		L.Push(lua.LBool(true))
		return 1
	})
	reg("UnitIsEnemy", func(L *lua.LState) int {
		L.Push(lua.LBool(false))
		return 1
	})
	reg("UnitFactionGroup", func(L *lua.LState) int {
		info := rt.unitInfo(L.OptString(1, "player"))
		if info == nil || !info.Player || info.RaceID == 0 {
			// Not known to be a player of a race: no faction group to name.
			L.Push(lua.LString(""))
			L.Push(lua.LString(""))
			return 2
		}
		group := "Horde"
		switch info.RaceID {
		case 1, 3, 4, 7, 11:
			group = "Alliance"
		}
		L.Push(lua.LString(group))
		L.Push(lua.LString(group))
		return 2
	})
	reg("UnitIsPVP", func(L *lua.LState) int {
		L.Push(lua.LBool(false))
		return 1
	})
	reg("UnitIsPVPFreeForAll", func(L *lua.LState) int {
		L.Push(lua.LFalse)
		return 1
	})
	reg("UnitIsTalking", func(L *lua.LState) int {
		L.Push(lua.LBool(false))
		return 1
	})
	reg("UnitInVehicle", func(L *lua.LState) int {
		L.Push(lua.LBool(false))
		return 1
	})
	reg("UnitVehicleSkin", func(L *lua.LState) int {
		L.Push(lua.LString(""))
		return 1
	})
	reg("UnitHasVehicleUI", func(L *lua.LState) int {
		L.Push(lua.LBool(false))
		return 1
	})
	reg("UnitIsPartyLeader", func(L *lua.LState) int {
		L.Push(lua.LBool(false))
		return 1
	})
	reg("UnitInParty", func(L *lua.LState) int {
		L.Push(lua.LBool(false))
		return 1
	})
	reg("UnitInRaid", func(L *lua.LState) int {
		L.Push(lua.LBool(false))
		return 1
	})
	reg("GetNumPartyMembers", func(L *lua.LState) int {
		L.Push(lua.LNumber(0))
		return 1
	})
	reg("GetNumRaidMembers", func(L *lua.LState) int {
		L.Push(lua.LNumber(0))
		return 1
	})
	reg("IsPartyLeader", func(L *lua.LState) int {
		L.Push(lua.LBool(false))
		return 1
	})
	reg("GetLootMethod", func(L *lua.LState) int {
		L.Push(lua.LString("freeforall"))
		L.Push(lua.LNil)
		return 2
	})
	reg("IsResting", func(L *lua.LState) int {
		L.Push(lua.LBool(false))
		return 1
	})
	reg("GetReadyCheckStatus", func(L *lua.LState) int {
		L.Push(lua.LNil)
		return 1
	})
	reg("GetComboPoints", func(L *lua.LState) int {
		L.Push(lua.LNumber(0))
		return 1
	})
	reg("GetThreatStatusColor", func(L *lua.LState) int {
		L.Push(lua.LNumber(1))
		L.Push(lua.LNumber(1))
		L.Push(lua.LNumber(1))
		return 3
	})
	reg("UnitThreatSituation", func(L *lua.LState) int {
		L.Push(lua.LNil)
		return 1
	})
	reg("UnitDetailedThreatSituation", func(L *lua.LState) int {
		L.Push(lua.LNil)
		return 1
	})
	reg("GetUnitSpeed", func(L *lua.LState) int {
		L.Push(lua.LNumber(0))
		return 1
	})
	reg("UnitAura", func(L *lua.LState) int {
		return rt.pushAura(L, L.OptString(1, ""), L.OptInt(2, 0), L.OptString(3, ""))
	})
	reg("UnitBuff", func(L *lua.LState) int {
		return rt.pushAura(L, L.OptString(1, ""), L.OptInt(2, 0), "HELPFUL")
	})
	reg("UnitDebuff", func(L *lua.LState) int {
		return rt.pushAura(L, L.OptString(1, ""), L.OptInt(2, 0), "HARMFUL")
	})
	reg("SetPortraitTexture", func(L *lua.LState) int {
		tex := rt.resolveTextureArg(L, 1)
		unit := L.OptString(2, "")
		rt.applyPortraitTexture(tex, unit)
		return 0
	})
	reg("SetPortraitToTexture", func(L *lua.LState) int {
		tex := rt.resolveTextureArg(L, 1)
		path := L.OptString(2, "")
		rt.applyPortraitToTexture(tex, path)
		return 0
	})
}

func (rt *Runtime) pushAura(L *lua.LState, unit string, index int, filter string) int {
	info := rt.unitInfo(unit)
	if info == nil || !info.Exists || index < 1 {
		return 0
	}
	wantHarmful := strings.Contains(strings.ToUpper(filter), "HARMFUL")
	wantHelpful := strings.Contains(strings.ToUpper(filter), "HELPFUL") || !wantHarmful
	position := 0
	for _, aura := range info.Auras {
		harmful := aura.Harmful
		if (wantHarmful && !harmful) || (wantHelpful && harmful) {
			continue
		}
		position++
		if position != index {
			continue
		}
		L.Push(lua.LString(aura.Name))
		L.Push(lua.LString(aura.Rank))
		L.Push(lua.LString(aura.Texture))
		L.Push(lua.LNumber(aura.Count))
		if aura.DebuffType == "" {
			L.Push(lua.LNil)
		} else {
			L.Push(lua.LString(aura.DebuffType))
		}
		L.Push(lua.LNumber(aura.Duration))
		L.Push(lua.LNumber(aura.ExpirationTime))
		if aura.Caster == "" {
			L.Push(lua.LNil)
		} else {
			L.Push(lua.LString(aura.Caster))
		}
		L.Push(lua.LBool(aura.CanStealOrPurge))
		L.Push(lua.LBool(aura.ShouldConsolidate))
		L.Push(lua.LNumber(aura.SpellID))
		return 11
	}
	return 0
}

// registerUnitExtras defines unit queries beyond the basic ones.
func registerUnitExtras(rt *Runtime) {
	L := rt.L
	L.SetGlobal("UnitGUID", L.NewFunction(func(L *lua.LState) int {
		guid := rt.UnitGUID(L.CheckString(1))
		if guid == 0 {
			L.Push(lua.LNil)
			return 1
		}
		L.Push(lua.LString(fmt.Sprintf("0x%016X", guid)))
		return 1
	}))
	L.SetGlobal("UnitClassBase", L.NewFunction(func(L *lua.LState) int {
		info := rt.unitInfo(L.CheckString(1))
		if info == nil || !info.Exists {
			L.Push(lua.LNil)
			return 1
		}
		L.Push(lua.LString(info.ClassFile))
		return 1
	}))
}

// RefreshTarget updates the identity and reaction of the current target
// (keeping its vitals) and reports whether anything changed.
func (rt *Runtime) RefreshTarget(info UnitInfo) bool {
	current := rt.unitInfo("target")
	if current == nil || !current.Exists {
		return false
	}
	changedName := current.Name != info.Name
	changedFaction := current.Hostile != info.Hostile || current.Friendly != info.Friendly
	changedLevel := info.Level != 0 && current.Level != info.Level
	if !changedName && !changedFaction && !changedLevel && current.Classification == info.Classification && current.CreatureType == info.CreatureType {
		return false
	}
	current.Name, current.Level = info.Name, info.Level
	current.Hostile, current.Friendly = info.Hostile, info.Friendly
	current.Classification, current.CreatureType = info.Classification, info.CreatureType
	current.RaceID, current.RaceFile, current.ClassID, current.ClassFile, current.Sex = info.RaceID, info.RaceFile, info.ClassID, info.ClassFile, info.Sex
	if changedName {
		rt.FireEvent("UNIT_NAME_UPDATE", lua.LString("target"))
	}
	if changedFaction {
		rt.FireEvent("UNIT_FACTION", lua.LString("target"))
	}
	if changedLevel {
		rt.FireEvent("UNIT_LEVEL", lua.LString("target"))
	}
	rt.FireEvent("PLAYER_TARGET_CHANGED")
	return true
}
