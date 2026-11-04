package ui

import (
	"github.com/MorenoLand/Moreno.WoW/world"
	lua "github.com/yuin/gopher-lua"
)

// Threat: for each unit the server sends the threat of whoever is on its
// list. The client keeps the list, scores each entry against the one with the
// most threat, and says whether the character is tanking.
const (
	threatHighestUpdate = 0x0482
	threatUpdate        = 0x0483
	threatRemove        = 0x0484
	threatClear         = 0x0485

	// threatPercentCap is the highest percentage kept.
	threatPercentCap = 250
	// threatUnknown is the percentage of an entry that was not scored yet.
	threatUnknown = 0xff
)

// Entry statuses: 1 is on the list below the top, 2 is level with the top,
// 3 is the top while another is level, 4 is the top.
type threatEntry struct {
	guid   uint64
	status uint8
	pct    uint8
	value  int32
}

type threatMob struct {
	highest uint64
	entries []*threatEntry
}

type threatState struct {
	mobs map[uint64]*threatMob
	// UnitReach gives the combat reach of a unit, for the melee range test.
	reach func(guid uint64) (float64, bool)
}

// SetUnitReach gives the interface the combat reach of the units the world
// has loaded.
func (rt *Runtime) SetUnitReach(lookup func(uint64) (float64, bool)) { rt.threat.reach = lookup }

func (s *threatState) mob(guid uint64, create bool) *threatMob {
	if s.mobs == nil {
		if !create {
			return nil
		}
		s.mobs = map[uint64]*threatMob{}
	}
	mob := s.mobs[guid]
	if mob == nil && create {
		mob = &threatMob{}
		s.mobs[guid] = mob
	}
	return mob
}

func (m *threatMob) entry(guid uint64) *threatEntry {
	for _, e := range m.entries {
		if e.guid == guid {
			return e
		}
	}
	return nil
}

func (rt *Runtime) tokenOfGUID(guid uint64) string {
	if token := rt.unitTokenOf(guid); token != "" {
		return token
	}
	for token, info := range rt.units {
		if info != nil && info.Exists && info.GUID == guid {
			return token
		}
	}
	return ""
}

func (rt *Runtime) threatSituationEvent(guid uint64) {
	if token := rt.tokenOfGUID(guid); token != "" {
		rt.FireEvent("UNIT_THREAT_SITUATION_UPDATE", lua.LString(token))
	}
}

func (rt *Runtime) threatListEvent(mob uint64) {
	if token := rt.tokenOfGUID(mob); token != "" {
		rt.FireEvent("UNIT_THREAT_LIST_UPDATE", lua.LString(token))
	}
}

// scoreThreat scores an entry against the most threat on the list; it reports
// whether the status of the entry changed.
func (rt *Runtime) scoreThreat(e *threatEntry, top int32) bool {
	var pct uint8
	switch {
	case top == 0:
		pct = 100
	case e.value < 1:
		pct = 0
	default:
		scored := int64(e.value) * 100 / int64(top)
		if scored > threatPercentCap {
			scored = threatPercentCap
		}
		pct = uint8(scored)
	}
	if pct == e.pct {
		return false
	}
	e.pct = pct
	status := uint8(2)
	if pct < 100 {
		status = 1
	}
	if status == e.status {
		return false
	}
	e.status = status
	rt.threatSituationEvent(e.guid)
	return true
}

// threatTop sets the status of the unit with the most threat: it holds the
// aggro securely unless another is level with it.
func (rt *Runtime) threatTop(m *threatMob) {
	top := m.entry(m.highest)
	if top == nil {
		return
	}
	status := uint8(4)
	for _, e := range m.entries {
		if e.guid != m.highest && e.status == 2 {
			status = 3
		}
	}
	if top.status != status {
		top.status = status
		rt.threatSituationEvent(top.guid)
	}
}

func (rt *Runtime) handleThreatPacket(packet world.Packet) bool {
	switch packet.Opcode {
	case threatHighestUpdate, threatUpdate, threatRemove, threatClear:
	default:
		return false
	}
	r := world.NewReader(packet.Body, "SMSG_THREAT")
	mobGUID, err := world.ReadPackedGUID(r)
	if err != nil {
		return true
	}
	switch packet.Opcode {
	case threatClear:
		if m := rt.threat.mob(mobGUID, false); m != nil {
			for _, e := range m.entries {
				rt.threatSituationEvent(e.guid)
			}
			m.highest, m.entries = 0, nil
			rt.threatListEvent(mobGUID)
		}
	case threatRemove:
		guid, err := world.ReadPackedGUID(r)
		m := rt.threat.mob(mobGUID, false)
		if err != nil || m == nil {
			return true
		}
		for i, e := range m.entries {
			if e.guid != guid {
				continue
			}
			rescore := false
			if guid == m.highest {
				m.highest = 0
			} else if e.status > 1 {
				rescore = true
			}
			m.entries = append(m.entries[:i], m.entries[i+1:]...)
			rt.threatSituationEvent(guid)
			if rescore {
				rt.threatTop(m)
			}
			rt.threatListEvent(mobGUID)
			break
		}
	default:
		m := rt.threat.mob(mobGUID, true)
		withHighest := packet.Opcode == threatHighestUpdate
		ensure := func(guid uint64) *threatEntry {
			if e := m.entry(guid); e != nil {
				return e
			}
			e := &threatEntry{guid: guid, status: 1, pct: threatUnknown}
			m.entries = append(m.entries, e)
			return e
		}
		if withHighest {
			if m.highest, err = world.ReadPackedGUID(r); err != nil {
				return true
			}
			if m.highest != 0 {
				ensure(m.highest)
			}
		}
		count, err := r.U32()
		if err != nil {
			return true
		}
		var updated []*threatEntry
		for i := uint32(0); i < count && i < 512; i++ {
			guid, err := world.ReadPackedGUID(r)
			if err != nil {
				break
			}
			value, err := r.U32()
			if err != nil {
				break
			}
			e := ensure(guid)
			e.value = int32(value)
			updated = append(updated, e)
		}
		changed := false
		if top := m.entry(m.highest); m.highest != 0 && top != nil {
			scored := updated
			if withHighest {
				scored = m.entries
			}
			for _, e := range scored {
				if e.guid != m.highest && rt.scoreThreat(e, top.value) {
					changed = true
				}
			}
		}
		if withHighest || changed {
			rt.threatTop(m)
		}
		rt.threatListEvent(mobGUID)
	}
	return true
}

// threatScaled is the percentage the unit needs to pull the aggro: 110% of the
// top threat in melee range, 130% outside it.
func (rt *Runtime) threatScaled(m *threatMob, e *threatEntry, mob uint64) float64 {
	if e.guid == m.highest {
		return 100
	}
	melee := false
	if e.guid == rt.playerGUID() && rt.unitDistance != nil {
		if distance, ok := rt.unitDistance(mob); ok {
			reach := 0.0
			if rt.threat.reach != nil {
				a, _ := rt.threat.reach(e.guid)
				b, _ := rt.threat.reach(mob)
				reach = a + b
			}
			limit := reach + 1.3333333730697632
			if limit < 5 {
				limit = 5
			}
			melee = distance <= limit
		}
	}
	if melee {
		return float64(e.pct) * 0.9090908765792847
	}
	return float64(e.pct) * 0.7692307829856873
}

func registerThreatAPI(rt *Runtime) {
	L := rt.L
	guidArg := func(L *lua.LState, index int) (uint64, bool) {
		token, ok := L.Get(index).(lua.LString)
		if !ok {
			return 0, false
		}
		info := rt.unitByNameOrToken(string(token))
		if info == nil {
			return 0, true
		}
		return info.GUID, true
	}
	L.SetGlobal("UnitThreatSituation", L.NewFunction(func(L *lua.LState) int {
		guid, ok := guidArg(L, 1)
		if !ok {
			L.RaiseError("Usage: UnitThreatSituation(\"unit\" [, \"mob\"])")
			return 0
		}
		if guid == 0 {
			return 0
		}
		var status uint8
		if L.Get(2).Type() == lua.LTString {
			mobGUID, _ := guidArg(L, 2)
			if m := rt.threat.mob(mobGUID, false); m != nil && mobGUID != 0 {
				if e := m.entry(guid); e != nil {
					status = e.status
				}
			}
		} else {
			for _, m := range rt.threat.mobs {
				if e := m.entry(guid); e != nil {
					if status < 3 {
						status = max(status, e.status)
					} else if status == 4 && e.status == 3 {
						status = 3
					}
				}
			}
		}
		if status == 0 {
			return 0
		}
		L.Push(lua.LNumber(status - 1))
		return 1
	}))
	L.SetGlobal("UnitDetailedThreatSituation", L.NewFunction(func(L *lua.LState) int {
		guid, ok1 := guidArg(L, 1)
		mobGUID, ok2 := guidArg(L, 2)
		if !ok1 || !ok2 {
			L.RaiseError("Usage: UnitDetailedThreatSituation(\"unit\", \"mob\")")
			return 0
		}
		m := rt.threat.mob(mobGUID, false)
		if m == nil || guid == 0 || mobGUID == 0 {
			return 0
		}
		e := m.entry(guid)
		if e == nil {
			return 0
		}
		if m.highest == guid {
			L.Push(lua.LNumber(1))
		} else {
			L.Push(lua.LNil)
		}
		L.Push(lua.LNumber(e.status - 1))
		L.Push(lua.LNumber(rt.threatScaled(m, e, mobGUID)))
		L.Push(lua.LNumber(e.pct))
		L.Push(lua.LNumber(e.value))
		return 5
	}))
}

// threatColors are the colors of the threat statuses 0 to 3.
var threatColors = [...][3]float64{
	{0xb0 / 255.0, 0xb0 / 255.0, 0xb0 / 255.0}, {1, 1, 0x77 / 255.0}, {1, 0x99 / 255.0, 0}, {1, 0, 0},
}

func registerThreatColorsAPI(rt *Runtime) {
	rt.L.SetGlobal("GetThreatStatusColor", rt.L.NewFunction(func(L *lua.LState) int {
		index := int(L.OptNumber(1, 0))
		if index < 0 || index >= len(threatColors) {
			index = 0
		}
		color := threatColors[index]
		L.Push(lua.LNumber(color[0]))
		L.Push(lua.LNumber(color[1]))
		L.Push(lua.LNumber(color[2]))
		return 3
	}))
}
