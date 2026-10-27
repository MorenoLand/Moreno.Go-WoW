package ui

import (
	"fmt"
	"time"

	"github.com/MorenoLand/Moreno.WoW/world"
	lua "github.com/yuin/gopher-lua"
)

// attackSpellID is the spell the Attack action button casts: the client turns
// it into CMSG_ATTACKSWING instead of a cast.
const attackSpellID = 6603

// WorldCombatHost starts and stops melee and cycles the target.
type WorldCombatHost interface {
	StartAttack(guid uint64) error
	StopAttack() error
	CycleEnemy(reverse bool) error
}

// CombatUnit names one side of a combat log event.
type CombatUnit struct {
	GUID  uint64
	Name  string
	Flags uint32
}

const combatLogLimit = 300

type combatState struct {
	attacking bool
	entries   [][]lua.LValue
	cursor    int // index into entries, newest last
}

// Attacking reports whether auto attack is on.
func (rt *Runtime) Attacking() bool { return rt.combat.attacking }

// SetAttacking records the server's word on auto attack and tells the
// interface, which flashes the Attack button and shows combat state.
func (rt *Runtime) SetAttacking(attacking bool) {
	if rt.combat.attacking == attacking {
		return
	}
	rt.combat.attacking = attacking
	if attacking {
		rt.FireEvent("PLAYER_ENTER_COMBAT")
	} else {
		rt.FireEvent("PLAYER_LEAVE_COMBAT")
	}
	rt.FireEvent("ACTIONBAR_UPDATE_STATE")
}

func (rt *Runtime) toggleAttack() error {
	host, ok := rt.Host.(WorldCombatHost)
	if !ok {
		return fmt.Errorf("world combat transport is unavailable")
	}
	if rt.combat.attacking {
		return host.StopAttack()
	}
	return rt.startAttack()
}

func (rt *Runtime) startAttack() error {
	host, ok := rt.Host.(WorldCombatHost)
	if !ok {
		return fmt.Errorf("world combat transport is unavailable")
	}
	target := rt.unitInfo("target")
	if target == nil || !target.Exists || target.GUID == 0 {
		return fmt.Errorf("no target")
	}
	return host.StartAttack(target.GUID)
}

func combatGUIDText(guid uint64) string { return fmt.Sprintf("0x%016X", guid) }

// CombatEvent records and announces one combat log event in the form the
// interface reads: timestamp, event, source and destination, then the
// event's own fields.
func (rt *Runtime) CombatEvent(subevent string, source, dest CombatUnit, extra ...lua.LValue) {
	name := func(unit CombatUnit) lua.LValue {
		if unit.Name == "" {
			return lua.LNil
		}
		return lua.LString(unit.Name)
	}
	args := []lua.LValue{
		lua.LNumber(float64(time.Now().UnixNano()) / 1e9), lua.LString(subevent),
		lua.LString(combatGUIDText(source.GUID)), name(source), lua.LNumber(source.Flags),
		lua.LString(combatGUIDText(dest.GUID)), name(dest), lua.LNumber(dest.Flags),
	}
	args = append(args, extra...)
	rt.combat.entries = append(rt.combat.entries, args)
	if len(rt.combat.entries) > combatLogLimit {
		rt.combat.entries = rt.combat.entries[len(rt.combat.entries)-combatLogLimit:]
	}
	rt.FireEvent("COMBAT_LOG_EVENT_UNFILTERED", args...)
	rt.FireEvent("COMBAT_LOG_EVENT", args...)
}

// registerCombatAPI defines the combat log accessors and the attack functions.
func registerCombatAPI(rt *Runtime) {
	L := rt.L
	reg := func(name string, fn lua.LGFunction) { L.SetGlobal(name, L.NewFunction(fn)) }
	// The log is read newest first: entry 0 is the newest, and advancing by -1
	// walks towards older ones.
	position := func() int { return len(rt.combat.entries) - 1 - rt.combat.cursor }
	reg("CombatLogGetNumEntries", func(L *lua.LState) int { L.Push(lua.LNumber(len(rt.combat.entries))); return 1 })
	reg("CombatLogSetCurrentEntry", func(L *lua.LState) int {
		rt.combat.cursor = L.OptInt(1, 0)
		return 0
	})
	reg("CombatLogGetCurrentEntry", func(L *lua.LState) int {
		index := position()
		if index < 0 || index >= len(rt.combat.entries) {
			return 0
		}
		for _, value := range rt.combat.entries[index] {
			L.Push(value)
		}
		return len(rt.combat.entries[index])
	})
	reg("CombatLogAdvanceEntry", func(L *lua.LState) int {
		// -1 steps towards older entries, which are further from the newest.
		rt.combat.cursor += -L.OptInt(1, 1)
		if rt.combat.cursor < 0 {
			rt.combat.cursor = 0
		}
		index := position()
		L.Push(lua.LBool(index >= 0 && index < len(rt.combat.entries) && rt.combat.cursor >= 0))
		return 1
	})
	reg("CombatLogClearEntries", func(L *lua.LState) int { rt.combat.entries, rt.combat.cursor = nil, 0; return 0 })
	reg("CombatLogResetFilter", func(L *lua.LState) int { return 0 })
	reg("CombatLogAddFilter", func(L *lua.LState) int { L.Push(lua.LTrue); return 1 })
	reg("CombatLogSetRetentionTime", func(L *lua.LState) int { return 0 })
	reg("CombatLogGetRetentionTime", func(L *lua.LState) int { L.Push(lua.LNumber(300)); return 1 })

	reg("StartAttack", func(L *lua.LState) int {
		if !rt.combat.attacking {
			_ = rt.startAttack()
		}
		return 0
	})
	reg("AttackTarget", func(L *lua.LState) int {
		if host, ok := rt.Host.(WorldCombatHost); ok {
			if rt.combat.attacking {
				_ = host.StopAttack()
			} else {
				_ = rt.startAttack()
			}
		}
		return 0
	})
	reg("StopAttack", func(L *lua.LState) int {
		if host, ok := rt.Host.(WorldCombatHost); ok && rt.combat.attacking {
			_ = host.StopAttack()
		}
		return 0
	})
	reg("IsAttackAction", func(L *lua.LState) int {
		action := rt.action(L.CheckInt(1))
		L.Push(lua.LBool(action.Type == 0 && action.ID == attackSpellID))
		return 1
	})
	reg("IsAutoRepeatAction", func(L *lua.LState) int { L.Push(lua.LFalse); return 1 })
	reg("IsCurrentAction", func(L *lua.LState) int {
		action := rt.action(L.CheckInt(1))
		current := action.Type == 0 && action.ID != 0 && rt.gameplay.Cast.ID == action.ID
		if action.Type == 0 && action.ID == attackSpellID {
			current = rt.combat.attacking
		}
		L.Push(lua.LBool(current))
		return 1
	})
	cycle := func(reverse bool) {
		if host, ok := rt.Host.(WorldCombatHost); ok {
			_ = host.CycleEnemy(reverse)
		}
	}
	reg("TargetNearestEnemy", func(L *lua.LState) int { cycle(luaTruthyOpt(L, 1, false)); return 0 })
	reg("TargetNearest", func(L *lua.LState) int { cycle(luaTruthyOpt(L, 1, false)); return 0 })
}

// SpellName is the catalog name of a spell, or "" if the catalog lacks it.
func (rt *Runtime) SpellName(id uint32) string {
	return rt.gameplay.Catalog[id].Name
}

// ShowError puts a line in the on-screen error display.
func (rt *Runtime) ShowError(text string) {
	if text != "" {
		rt.FireEvent("UI_ERROR_MESSAGE", lua.LString(text))
	}
}

// ShowGlobalError shows the text of a global string such as ERR_BADATTACKPOS.
func (rt *Runtime) ShowGlobalError(name string) {
	if index, ok := gameErrorIndex(name); ok {
		rt.ShowGameError(index)
		return
	}
	if text, ok := rt.L.GetGlobal(name).(lua.LString); ok {
		rt.ShowError(string(text))
	}
}

// ShowSpellFailure shows why a cast failed, from the SpellCastResult code.
func (rt *Runtime) ShowSpellFailure(code uint8, powerToken string) {
	name := world.SpellFailedName(code)
	if name == "" {
		return
	}
	if code == world.SpellFailureNoPower {
		rt.ShowGlobalError("ERR_OUT_OF_" + powerToken)
		return
	}
	rt.ShowGlobalError("SPELL_FAILED_" + name)
}

// UnitName is the name stored for a unit token.
func (rt *Runtime) UnitName(unit string) string {
	if info := rt.unitInfo(unit); info != nil {
		return info.Name
	}
	return ""
}

// UnitPowerToken is the power token (MANA, RAGE, ...) of a unit token.
func (rt *Runtime) UnitPowerToken(unit string) string {
	if info := rt.unitInfo(unit); info != nil && info.PowerToken != "" {
		return info.PowerToken
	}
	return "MANA"
}

// WorldAuraHost cancels one of the player's own auras.
type WorldAuraHost interface {
	CancelAura(spellID uint32) error
	CancelMount() error
}

// registerAuraCancelAPI defines CancelUnitBuff, which right-clicking a buff
// calls (dismounting is cancelling the mount's aura).
func registerAuraCancelAPI(rt *Runtime) {
	L := rt.L
	L.SetGlobal("CancelUnitBuff", L.NewFunction(func(L *lua.LState) int {
		info := rt.unitInfo(L.OptString(1, "player"))
		host, ok := rt.Host.(WorldAuraHost)
		if info == nil || !ok {
			return 0
		}
		index := 0
		for _, aura := range info.Auras {
			if aura.Harmful {
				continue
			}
			index++
			match := false
			switch value := L.Get(2).(type) {
			case lua.LNumber:
				match = int(value) == index
			case lua.LString:
				match = aura.Name == string(value)
			}
			if match && aura.SpellID != 0 {
				_ = host.CancelAura(aura.SpellID)
				break
			}
		}
		return 0
	}))
}

// spell effect types that help the unit they land on.
var helpfulSpellEffects = map[int]bool{10: true, 18: true, 30: true, 35: true, 65: true, 67: true, 113: true}

const (
	spellAttributeCastableWhileMounted = 0x01000000
	spellAuraMounted                   = 78
)

func spellIsHelpful(info SpellInfo) bool {
	for _, effect := range info.Effects {
		if helpfulSpellEffects[effect.Effect] {
			return true
		}
	}
	return false
}

func spellIsMount(info SpellInfo) bool {
	for _, effect := range info.Effects {
		if effect.Aura == spellAuraMounted {
			return true
		}
	}
	return false
}
