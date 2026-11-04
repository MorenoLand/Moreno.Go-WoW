package ui

import (
	"math"
	"time"

	lua "github.com/yuin/gopher-lua"
)

const (
	// playerMaxLevelField is PLAYER_FIELD_MAX_LEVEL.
	playerMaxLevelField = 1279
	// auraModQuestXP is the aura that changes the experience quests give.
	auraModQuestXP = 0x123
	// questXPShare is how far a friend must be to share in a quest's bonus.
	questXPShare = 100.0
	// questXPBonusLevel is the level up to which a friend triples quest XP.
	questXPBonusLevel = 60
	// spellAttrTradeSpell marks a spell of a profession.
	spellAttrTradeSpell      = 0x20
	spellEffectLearnPetSpell = 0x39
)

// questGreyRange is how many levels below itself a character still gets
// experience for a quest.
var questGreyRange = [...]int{4, 4, 5, 5, 6, 6, 7, 7, 8, 8}

func greyRange(level int) int {
	if index := level / 5; index < len(questGreyRange) {
		return questGreyRange[index]
	}
	return 8
}

// questByCall is the quest a call names by its place in the log, or the
// selected one.
func (rt *Runtime) questByCall(L *lua.LState) (QuestLogSlot, bool) {
	rows := rt.questRows()
	index := rt.quests.selected
	if L.Get(1).Type() == lua.LTNumber {
		index = L.CheckInt(1)
	}
	if index < 1 || index > len(rows) || rows[index-1].slot < 0 {
		return QuestLogSlot{}, false
	}
	return rt.quests.slots[rows[index-1].slot], true
}

// playerAtMaxLevel is whether the character has reached the level cap.
func (rt *Runtime) playerAtMaxLevel() bool {
	player := rt.unitInfo("player")
	return player != nil && player.Level >= int(rt.field(playerMaxLevelField))
}

// questXPBonus is the share of extra experience the auras of the character
// give.
func (rt *Runtime) questXPBonus() float64 {
	bonus := 1.0
	player := rt.unitInfo("player")
	if player == nil {
		return bonus
	}
	for _, aura := range player.Auras {
		spell, ok := rt.gameplay.Catalog[aura.SpellID]
		if !ok {
			continue
		}
		for _, effect := range spell.Effects {
			if effect.Aura == auraModQuestXP {
				bonus += float64(effect.BasePoints+1) * 0.01
			}
		}
	}
	return bonus
}

func registerQuestLogRewardsAPI(rt *Runtime) {
	L := rt.L
	reg := func(name string, fn lua.LGFunction) { L.SetGlobal(name, L.NewFunction(fn)) }
	reg("GetQuestLogRewardMoney", func(L *lua.LState) int {
		money := 0
		if slot, ok := rt.questByCall(L); ok {
			info := rt.quests.info[slot.ID]
			if reward := int32(info.RewardMoney); reward > 0 {
				money = int(reward)
			}
			if rt.playerAtMaxLevel() && money <= int(info.RewardMoneyMax) {
				money = int(info.RewardMoneyMax)
			}
		}
		L.Push(lua.LNumber(money))
		return 1
	})
	reg("GetQuestLogRequiredMoney", func(L *lua.LState) int {
		money := 0
		if slot, ok := rt.questByCall(L); ok {
			if reward := int32(rt.quests.info[slot.ID].RewardMoney); reward < 0 {
				money = int(-reward)
			}
		}
		L.Push(lua.LNumber(money))
		return 1
	})
	reg("GetQuestLogRewardXP", func(L *lua.LState) int {
		xp := 0
		slot, ok := rt.questByCall(L)
		player := rt.unitInfo("player")
		if ok && player != nil && !rt.playerAtMaxLevel() {
			info := rt.quests.info[slot.ID]
			xp = int(rt.questXPFor(info.Level, info.XPRow))
			for party := 1; party <= 4; party++ {
				member := rt.unitInfo("party" + string(rune('0'+party)))
				if member == nil || !member.Exists || !rafLinked(member) {
					continue
				}
				if distance, known := rt.unitDistance(member.GUID); rt.unitDistance == nil || !known || distance >= questXPShare {
					continue
				}
				if info.Level != -1 {
					level := int(info.Level)
					if player.Level > greyRange(player.Level)+level || member.Level > greyRange(member.Level)+level ||
						int(info.MinLevel) > player.Level || int(info.MinLevel) > member.Level {
						continue
					}
				}
				if player.Level < questXPBonusLevel {
					xp *= 3
				}
			}
			xp = int(math.Round(rt.questXPBonus() * float64(xp)))
		}
		L.Push(lua.LNumber(xp))
		return 1
	})
	reg("GetQuestLogRewardSpell", func(L *lua.LState) int {
		slot, ok := rt.questByCall(L)
		if ok {
			if info := rt.quests.info[slot.ID]; info.RewardSpell != 0 {
				spell, known := rt.gameplay.Catalog[info.RewardSpell]
				if known {
					L.Push(lua.LString(spell.Icon))
					L.Push(lua.LString(spell.Name))
					if spell.Attributes&spellAttrTradeSpell != 0 {
						L.Push(lua.LNumber(1))
					} else {
						L.Push(lua.LNil)
					}
					learned := false
					if cast, found := rt.gameplay.Catalog[info.RewardSpellCast]; info.RewardSpellCast != 0 && found {
						for _, effect := range cast.Effects {
							if effect.Effect == spellEffectLearnSpell || effect.Effect == spellEffectLearnPetSpell {
								learned = true
							}
						}
					}
					if learned {
						L.Push(lua.LNumber(1))
					} else {
						L.Push(lua.LNil)
					}
					return 4
				}
			}
		}
		for i := 0; i < 4; i++ {
			L.Push(lua.LNil)
		}
		return 4
	})
	reg("GetQuestLogTimeLeft", func(L *lua.LState) int {
		rows := rt.questRows()
		index := rt.quests.selected
		if index >= 1 && index <= len(rows) && rows[index-1].slot >= 0 {
			slot := rt.quests.slots[rows[index-1].slot]
			if slot.Timer != 0 && slot.State&2 == 0 {
				left := int64(slot.Timer) - time.Now().Unix() - 1
				L.Push(lua.LNumber(max(left, 0)))
				return 1
			}
		}
		L.Push(lua.LNil)
		return 1
	})
}
