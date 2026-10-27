package ui

import (
	"math"

	"github.com/MorenoLand/Moreno.WoW/world"
	lua "github.com/yuin/gopher-lua"
)

// Update field indices of the player's own statistics (UNIT_END is 148).
const (
	fieldBaseAttackTime        = 6 + 0x38
	fieldRangedAttackTime      = 6 + 0x3A
	fieldMinDamage             = 6 + 0x40
	fieldStat0                 = 6 + 0x4E
	fieldPosStat0              = 6 + 0x53
	fieldNegStat0              = 6 + 0x58
	fieldResistances           = 6 + 0x5D
	fieldResistancePos         = 6 + 0x64
	fieldResistanceNeg         = 6 + 0x6B
	fieldAttackPower           = 6 + 0x75
	fieldAttackPowerMods       = 6 + 0x76
	fieldAttackPowerMult       = 6 + 0x77
	fieldRangedAttackPower     = 6 + 0x78
	fieldRangedAttackPowerMods = 6 + 0x79
	fieldRangedAttackPowerMult = 6 + 0x7A
	fieldMinRangedDamage       = 6 + 0x7B
	fieldMaxHealthModifier     = 6 + 0x8B
	fieldPowerRegenFlat        = 6 + 0x22
	fieldPowerRegenInterrupted = 6 + 0x29

	fieldBlockPercentage     = 148 + 0x36C
	fieldDodgePercentage     = 148 + 0x36D
	fieldParryPercentage     = 148 + 0x36E
	fieldExpertise           = 148 + 0x36F
	fieldOffhandExpertise    = 148 + 0x370
	fieldCritPercentage      = 148 + 0x371
	fieldRangedCritPercent   = 148 + 0x372
	fieldSpellCritPercentage = 148 + 0x374
	fieldShieldBlock         = 148 + 0x37B
	fieldRestStateExperience = 148 + 0x3FD
	fieldModDamageDonePos    = 148 + 0x3FF
	fieldModDamageDoneNeg    = 148 + 0x406
	fieldModDamageDonePct    = 148 + 0x40D
	fieldModHealingDonePos   = 148 + 0x414
	fieldModTargetResistance = 148 + 0x417
	fieldKills               = 148 + 0x435
	fieldTodayContribution   = 148 + 0x436
	fieldYesterdayContrib    = 148 + 0x437
	fieldLifetimeKills       = 148 + 0x438
	fieldCombatRating1       = 148 + 0x43B
	fieldPetSpellPower       = 148 + 0x499
)

// ratingCount is how many combat ratings there are.
const ratingCount = 25

// GameTables are the gt*.dbc tables of per class and level values, by name
// without the prefix and extension.
type GameTables map[string][]float32

// gameTableWidth is how many entries a row of each table has.
var gameTableWidth = map[string]int{
	"CombatRatings":              100,
	"ChanceToMeleeCrit":          100,
	"ChanceToMeleeCritBase":      1,
	"ChanceToSpellCrit":          100,
	"ChanceToSpellCritBase":      1,
	"OCTRegenHP":                 100,
	"OCTRegenMP":                 100,
	"RegenHPPerSpt":              100,
	"RegenMPPerSpt":              100,
	"OCTClassCombatRatingScalar": 32,
}

// SetGameTables gives the interface the per class and level tables.
func (rt *Runtime) SetGameTables(tables GameTables) { rt.gameTables = tables }

// SetPlayerFields keeps the player's update fields for the statistics the
// character sheet reads from them.
func (rt *Runtime) SetPlayerFields(fields map[uint16]uint32) { rt.playerFields = fields }

func (rt *Runtime) gameTable(name string, x, y int) float64 {
	values := rt.gameTables[name]
	index := x + y*gameTableWidth[name]
	if x < 0 || y < 0 || index >= len(values) {
		return 0
	}
	return float64(values[index])
}

func (rt *Runtime) field(index int) uint32 { return rt.playerFields[uint16(index)] }

func (rt *Runtime) fieldFloat(index int) float64 {
	return float64(math.Float32frombits(rt.field(index)))
}

// fieldCount reads a field as a count that never goes below zero.
func (rt *Runtime) fieldCount(index int) int {
	value := int32(rt.field(index))
	if value < 0 {
		return 0
	}
	return int(value)
}

// isSelf reports whether a unit token names the player, whose fields are the
// ones the client holds in full.
func (rt *Runtime) isSelf(unit string) bool {
	info := rt.unitInfo(unit)
	if info == nil {
		return false
	}
	player := rt.unitInfo("player")
	return player != nil && (unit == "player" || (info.GUID != 0 && info.GUID == player.GUID))
}

func (rt *Runtime) playerLevelClass() (int, int) {
	if info := rt.unitInfo("player"); info != nil {
		return info.Level, info.ClassID
	}
	return 0, 0
}

// ratingBonus is what a combat rating is worth, as a percentage or points.
func (rt *Runtime) ratingBonus(rating int) float64 {
	level, class := rt.playerLevelClass()
	divisor := rt.gameTable("CombatRatings", level-1, rating)
	scalar := rt.gameTable("OCTClassCombatRatingScalar", rating+1, class-1)
	if divisor == 0 || scalar == 0 {
		return 0
	}
	return float64(int32(rt.field(fieldCombatRating1+rating))) / divisor * scalar
}

// registerStatsAPI defines the calls the character sheet makes for the
// statistics of the player.
func registerStatsAPI(rt *Runtime) {
	L := rt.L
	reg := func(name string, fn lua.LGFunction) { L.SetGlobal(name, L.NewFunction(fn)) }
	number := func(L *lua.LState, values ...float64) int {
		for _, value := range values {
			L.Push(lua.LNumber(value))
		}
		return len(values)
	}
	unitArg := func(L *lua.LState) (string, bool) {
		unit := L.CheckString(1)
		return unit, rt.isSelf(unit)
	}
	reg("UnitStat", func(L *lua.LState) int {
		unit, self := unitArg(L)
		stat := L.CheckInt(2) - 1
		if stat < 0 || stat >= 5 {
			L.RaiseError("Invalid stat index in UnitStat")
			return 0
		}
		if !self {
			return number(L, 0, 0, 0, 0)
		}
		_ = unit
		return number(L, float64(int32(rt.field(fieldStat0+stat))), float64(rt.fieldCount(fieldStat0+stat)), float64(int32(rt.field(fieldPosStat0+stat))), float64(int32(rt.field(fieldNegStat0+stat))))
	})
	// resistance is (base, total, positive, negative) for one school.
	resistance := func(school int) (base, total, positive, negative int) {
		total = int(int32(rt.field(fieldResistances + school)))
		positive = int(int32(rt.field(fieldResistancePos + school)))
		negative = int(int32(rt.field(fieldResistanceNeg + school)))
		base = total - positive - negative
		if total < 0 {
			total = 0
		}
		return
	}
	reg("UnitResistance", func(L *lua.LState) int {
		_, self := unitArg(L)
		school := L.CheckInt(2)
		if school < 0 || school > 6 {
			L.RaiseError("Invalid resistance index in UnitResistance")
			return 0
		}
		if !self {
			return number(L, 0, 0, 0, 0)
		}
		base, total, positive, negative := resistance(school)
		return number(L, float64(base), float64(total), float64(positive), float64(negative))
	})
	reg("UnitArmor", func(L *lua.LState) int {
		_, self := unitArg(L)
		if !self {
			return number(L, 0, 0, 0, 0, 0)
		}
		base, total, positive, negative := resistance(0)
		return number(L, float64(base), float64(total), float64(total), float64(positive), float64(negative))
	})
	power := func(attack, mods, mult int) func(L *lua.LState) int {
		return func(L *lua.LState) int {
			if _, self := unitArg(L); !self {
				return number(L, 0, 0, 0)
			}
			multiplier := rt.fieldFloat(mult) + 1
			positive, negative := int16(rt.field(mods)), int16(rt.field(mods)>>16)
			return number(L, math.Round(multiplier*float64(int32(rt.field(attack)))), math.Round(multiplier*float64(positive)), math.Round(multiplier*float64(negative)))
		}
	}
	reg("UnitAttackPower", power(fieldAttackPower, fieldAttackPowerMods, fieldAttackPowerMult))
	reg("UnitRangedAttackPower", power(fieldRangedAttackPower, fieldRangedAttackPowerMods, fieldRangedAttackPowerMult))
	reg("UnitDamage", func(L *lua.LState) int {
		_, self := unitArg(L)
		if !self {
			return number(L, 0, 0, 0, 0, 0, 0, 0)
		}
		return number(L, rt.fieldFloat(fieldMinDamage), rt.fieldFloat(fieldMinDamage+1), rt.fieldFloat(fieldMinDamage+2), rt.fieldFloat(fieldMinDamage+3),
			float64(int32(rt.field(fieldModDamageDonePos))), float64(int32(rt.field(fieldModDamageDoneNeg))), rt.fieldFloat(fieldModDamageDonePct))
	})
	reg("UnitRangedDamage", func(L *lua.LState) int {
		_, self := unitArg(L)
		if !self {
			return number(L, 0, 0, 0, 0, 0, 1)
		}
		school := rt.rangedDamageSchool()
		return number(L, float64(rt.field(fieldRangedAttackTime))*0.001, rt.fieldFloat(fieldMinRangedDamage), rt.fieldFloat(fieldMinRangedDamage+1),
			float64(int32(rt.field(fieldModDamageDonePos+school))), float64(int32(rt.field(fieldModDamageDoneNeg+school))), rt.fieldFloat(fieldModDamageDonePct+school))
	})
	reg("UnitAttackSpeed", func(L *lua.LState) int {
		_, self := unitArg(L)
		if !self {
			L.Push(lua.LNumber(0))
			L.Push(lua.LNil)
			return 2
		}
		L.Push(lua.LNumber(float64(rt.field(fieldBaseAttackTime)) * 0.001))
		if rt.offhandWeaponEquipped() {
			L.Push(lua.LNumber(float64(rt.field(fieldBaseAttackTime+1)) * 0.001))
		} else {
			L.Push(lua.LNil)
		}
		return 2
	})
	reg("UnitAttackBothHands", func(L *lua.LState) int {
		if _, self := unitArg(L); !self {
			return number(L, 0, 0, 0, 0)
		}
		mainBase, mainMod := rt.weaponSkill(equipSlotMainHand)
		offBase, offMod := rt.weaponSkill(equipSlotOffHand)
		return number(L, float64(mainBase), float64(mainMod), float64(offBase), float64(offMod))
	})
	reg("UnitRangedAttack", func(L *lua.LState) int {
		if _, self := unitArg(L); !self {
			return number(L, 0, 0)
		}
		base, mod := rt.weaponSkill(equipSlotRanged)
		return number(L, float64(base), float64(mod))
	})
	reg("UnitDefense", func(L *lua.LState) int {
		if _, self := unitArg(L); !self {
			return number(L, 0, 0)
		}
		base, mod := rt.skillRankByID(skillDefense)
		return number(L, float64(base), float64(mod))
	})

	reg("GetCombatRating", func(L *lua.LState) int {
		rating := L.CheckInt(1) - 1
		if rating < 0 || rating >= ratingCount {
			L.RaiseError("ratingIndex is in the range 1 .. %d", ratingCount)
			return 0
		}
		return number(L, float64(int32(rt.field(fieldCombatRating1+rating))))
	})
	reg("GetCombatRatingBonus", func(L *lua.LState) int {
		rating := L.CheckInt(1) - 1
		if rating < 0 || rating >= ratingCount {
			L.RaiseError("ratingIndex is in the range 1 .. %d", ratingCount)
			return 0
		}
		return number(L, rt.ratingBonus(rating))
	})
	reg("GetMaxCombatRatingBonus", func(L *lua.LState) int {
		rating := L.CheckInt(1) - 1
		if rating < 0 || rating >= ratingCount {
			L.RaiseError("ratingIndex is in the range 1 .. %d", ratingCount)
			return 0
		}
		if rating > 13 && rating < 17 {
			return number(L, 33)
		}
		return number(L, -1)
	})
	reg("GetCritChance", func(L *lua.LState) int { return number(L, rt.fieldFloat(fieldCritPercentage)) })
	reg("GetRangedCritChance", func(L *lua.LState) int { return number(L, rt.fieldFloat(fieldRangedCritPercent)) })
	reg("GetDodgeChance", func(L *lua.LState) int { return number(L, rt.fieldFloat(fieldDodgePercentage)) })
	reg("GetParryChance", func(L *lua.LState) int { return number(L, rt.fieldFloat(fieldParryPercentage)) })
	reg("GetBlockChance", func(L *lua.LState) int { return number(L, rt.fieldFloat(fieldBlockPercentage)) })
	reg("GetShieldBlock", func(L *lua.LState) int { return number(L, float64(int32(rt.field(fieldShieldBlock)))) })
	reg("GetSpellCritChance", func(L *lua.LState) int {
		school := L.CheckInt(1)
		if school < 1 || school > 7 {
			L.RaiseError("Usage: GetSpellCritChance(school)")
			return 0
		}
		return number(L, rt.fieldFloat(fieldSpellCritPercentage+school-1))
	})
	reg("GetExpertise", func(L *lua.LState) int {
		return number(L, float64(int32(rt.field(fieldExpertise))), float64(int32(rt.field(fieldOffhandExpertise))))
	})
	reg("GetExpertisePercent", func(L *lua.LState) int {
		return number(L, float64(int32(rt.field(fieldExpertise)))*0.25, float64(int32(rt.field(fieldOffhandExpertise)))*0.25)
	})
	reg("GetSpellBonusDamage", func(L *lua.LState) int {
		school := L.CheckInt(1)
		if school < 1 || school > 7 {
			L.RaiseError("Usage: GetSpellBonusDamage(school)")
			return 0
		}
		return number(L, float64(int32(rt.field(fieldModDamageDonePos+school-1))+int32(rt.field(fieldModDamageDoneNeg+school-1))))
	})
	reg("GetSpellBonusHealing", func(L *lua.LState) int { return number(L, float64(int32(rt.field(fieldModHealingDonePos)))) })
	reg("GetPetSpellBonusDamage", func(L *lua.LState) int { return number(L, float64(int32(rt.field(fieldPetSpellPower)))) })
	reg("GetSpellPenetration", func(L *lua.LState) int { return number(L, -float64(int32(rt.field(fieldModTargetResistance)))) })
	reg("GetArmorPenetration", func(L *lua.LState) int {
		return number(L, math.Min(rt.ratingBonus(24), 100))
	})
	reg("GetAttackPowerForStat", func(L *lua.LState) int {
		if L.Get(1) != lua.LNil && L.Get(2) != lua.LNil {
			return number(L, float64(rt.attackPowerForStat(L.CheckInt(1)-1, L.CheckInt(2))))
		}
		L.RaiseError("Usage: Script_GetAttackPowerForStat(stat, value)")
		return 0
	})
	reg("GetCritChanceFromAgility", func(L *lua.LState) int {
		if _, self := unitArg(L); !self {
			return number(L, 0)
		}
		level, class := rt.playerLevelClass()
		base := rt.gameTable("ChanceToMeleeCritBase", 0, class-1)
		per := rt.gameTable("ChanceToMeleeCrit", level-1, class-1)
		if per == 0 {
			return number(L, 0)
		}
		return number(L, (per*float64(rt.fieldCount(fieldStat0+1))+base)*100)
	})
	reg("GetSpellCritChanceFromIntellect", func(L *lua.LState) int {
		if _, self := unitArg(L); !self {
			return number(L, 0)
		}
		level, class := rt.playerLevelClass()
		base := rt.gameTable("ChanceToSpellCritBase", 0, class-1)
		per := rt.gameTable("ChanceToSpellCrit", level-1, class-1)
		if per == 0 {
			return number(L, 0)
		}
		return number(L, (per*float64(rt.fieldCount(fieldStat0+3))+base)*100)
	})
	reg("GetUnitMaxHealthModifier", func(L *lua.LState) int {
		if _, self := unitArg(L); !self {
			return number(L, 0)
		}
		return number(L, rt.fieldFloat(fieldMaxHealthModifier)+1)
	})
	reg("GetUnitHealthModifier", func(L *lua.LState) int {
		unitArg(L)
		return number(L, 1)
	})
	reg("GetUnitPowerModifier", func(L *lua.LState) int {
		unitArg(L)
		return number(L, 1)
	})
	reg("GetUnitHealthRegenRateFromSpirit", func(L *lua.LState) int {
		if _, self := unitArg(L); !self {
			return number(L, 0)
		}
		level, class := rt.playerLevelClass()
		spirit := rt.fieldCount(fieldStat0 + 4)
		capped := min(spirit, 50)
		return number(L, float64(capped)*rt.gameTable("OCTRegenHP", level-1, class-1)+rt.gameTable("RegenHPPerSpt", level-1, class-1)*float64(spirit-capped))
	})
	reg("GetUnitManaRegenRateFromSpirit", func(L *lua.LState) int {
		if _, self := unitArg(L); !self {
			return number(L, 0)
		}
		level, class := rt.playerLevelClass()
		intellect, spirit := rt.fieldCount(fieldStat0+3), rt.fieldCount(fieldStat0+4)
		return number(L, math.Sqrt(float64(intellect))*rt.gameTable("RegenMPPerSpt", level-1, class-1)*float64(spirit)+0.001)
	})
	reg("GetManaRegen", func(L *lua.LState) int {
		if info := rt.unitInfo("player"); info != nil && info.PowerType == 0 {
			return number(L, rt.fieldFloat(fieldPowerRegenFlat)+0.001, rt.fieldFloat(fieldPowerRegenInterrupted)+0.001)
		}
		return number(L, 0, 0)
	})
	reg("GetXPExhaustion", func(L *lua.LState) int {
		if rested := rt.field(fieldRestStateExperience); rested > 0 {
			return number(L, float64(rested))
		}
		L.Push(lua.LNil)
		return 1
	})
	reg("GetPVPSessionStats", func(L *lua.LState) int {
		return number(L, float64(rt.field(fieldKills)&0xFFFF), float64(rt.field(fieldTodayContribution)))
	})
	reg("GetPVPYesterdayStats", func(L *lua.LState) int {
		return number(L, float64(rt.field(fieldKills)>>16), float64(rt.field(fieldYesterdayContrib)))
	})
	reg("GetPVPLifetimeStats", func(L *lua.LState) int {
		return number(L, float64(rt.field(fieldLifetimeKills)), 0, 0)
	})
	reg("GetPVPThisWeekStats", func(L *lua.LState) int { return number(L, 0, 0) })
	reg("GetPVPLastWeekStats", func(L *lua.LState) int { return number(L, 0, 0, 0, 0) })
}

// attackPowerForStat is the attack power a stat gives the class of the player:
// strength gives two for most classes and one for the classes ChrClasses marks
// with a 1 in its second column (hunter, rogue, shaman), agility gives one to
// those.
func (rt *Runtime) attackPowerForStat(stat, value int) int {
	_, class := rt.playerLevelClass()
	value -= 10
	if value < 0 {
		value = 0
	}
	single := class == classHunter || class == classRogue || class == classShaman
	switch stat {
	case 0:
		if single {
			return value
		}
		return value * 2
	case 1:
		if single {
			return value
		}
	}
	return 0
}

// Class ids.
const (
	classWarrior = 1
	classPaladin = 2
	classHunter  = 3
	classRogue   = 4
	classPriest  = 5
	classShaman  = 7
	classMage    = 8
	classWarlock = 9
	classDruid   = 11
)

// Equipment slots of the weapons.
const (
	equipSlotMainHand = 15
	equipSlotOffHand  = 16
	equipSlotRanged   = 17
)

// Skill lines the character sheet reads.
const (
	skillDefense = 95
	skillUnarmed = 162
)

// equippedTemplate is the template of the item worn in an equipment slot.
func (rt *Runtime) equippedTemplate(slot int) *world.ItemTemplate {
	item := rt.inventory.items[rt.inventory.slots[slot]]
	if item == nil {
		return nil
	}
	return rt.inventory.templates[item.Entry]
}

// weaponSkillLine is the skill line a weapon is used with, by item subclass.
var weaponSkillLine = map[uint32]uint32{
	0: 44, 1: 172, 2: 45, 3: 46, 4: 54, 5: 160, 6: 229, 7: 43, 8: 55,
	10: 136, 13: 473, 15: 173, 16: 176, 18: 226, 19: 228, 20: 356,
}

// skillRankByID is the rank and the bonus of a skill line the character has.
func (rt *Runtime) skillRankByID(id uint32) (int, int) {
	if value, ok := rt.skillValues[id]; ok {
		return value[0], value[2] + value[3]
	}
	for _, entry := range rt.skills.entries {
		if entry.ID == id {
			return entry.Rank, entry.TempBonus + entry.PermBonus
		}
	}
	return 0, 0
}

// weaponSkill is the skill of the weapon in a slot, the unarmed one when the
// slot holds none.
func (rt *Runtime) weaponSkill(slot int) (int, int) {
	template := rt.equippedTemplate(slot)
	if template == nil || template.Class != 2 {
		if slot == equipSlotMainHand {
			return rt.skillRankByID(skillUnarmed)
		}
		return 0, 0
	}
	return rt.skillRankByID(weaponSkillLine[template.SubClass])
}

func (rt *Runtime) offhandWeaponEquipped() bool {
	template := rt.equippedTemplate(equipSlotOffHand)
	return template != nil && template.Class == 2
}

// rangedDamageSchool is the damage school of the ranged weapon, physical when
// there is none.
func (rt *Runtime) rangedDamageSchool() int {
	if template := rt.equippedTemplate(equipSlotRanged); template != nil {
		for _, damage := range template.Damage {
			if damage.Max > 0 {
				return int(damage.Type)
			}
		}
	}
	return 0
}

// SetSkillValues keeps the value of every skill line the character has, the
// ones the skill window hides too: (rank, maximum, temporary, permanent).
func (rt *Runtime) SetSkillValues(values map[uint32][4]int) { rt.skillValues = values }
