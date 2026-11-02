package ui

import (
	"sort"

	lua "github.com/yuin/gopher-lua"
)

// Minimap tracking: the tracking spells the character knows, then the
// client's own list of things to show (Wow.exe's table at 0xA11C50: the
// vendors, trainers and the like, for the classes that may use each).
type trackingOther struct {
	// kind is 1 for an NPC flag, 2 for a game object type, 3 for no filter;
	// value is that flag or type.
	kind, value uint32
	key, icon   string
	classMask   uint32
}

var trackingOthers = []trackingOther{
	{1, 0x1000, "MINIMAP_TRACKING_REPAIR", "Repair", 0},
	{1, 0x200, "MINIMAP_TRACKING_VENDOR_FOOD", "Food", 0},
	{1, 0x400, "MINIMAP_TRACKING_VENDOR_POISON", "Poisons", 0x10},
	{1, 0x100, "MINIMAP_TRACKING_VENDOR_AMMO", "Ammunition", 0x1a},
	{1, 0x800, "MINIMAP_TRACKING_VENDOR_REAGENT", "Reagents", 0},
	{1, 0x10000, "MINIMAP_TRACKING_INNKEEPER", "Innkeeper", 0},
	{1, 0x2000, "MINIMAP_TRACKING_FLIGHTMASTER", "FlightMaster", 0},
	{1, 0x400000, "MINIMAP_TRACKING_STABLEMASTER", "StableMaster", 0x8},
	{1, 0x100000, "MINIMAP_TRACKING_BATTLEMASTER", "BattleMaster", 0},
	{1, 0x20, "MINIMAP_TRACKING_TRAINER_CLASS", "Class", 0},
	{1, 0x40, "MINIMAP_TRACKING_TRAINER_PROFESSION", "Profession", 0},
	{1, 0x200000, "MINIMAP_TRACKING_AUCTIONEER", "Auctioneer", 0},
	{1, 0x20000, "MINIMAP_TRACKING_BANKER", "Banker", 0},
	{2, 0x13, "MINIMAP_TRACKING_MAILBOX", "Mailbox", 0},
	{3, 0, "MINIMAP_TRACKING_TRIVIAL_QUESTS", "TrivialQuests", 0},
}

const trackingIconDir = `Interface\Minimap\Tracking\`

// trackingAuras are the auras that track: creatures, resources and the
// stealthed (Wow.exe tests a spell's three effects for these).
func spellTracks(info SpellInfo) bool {
	for _, effect := range info.Effects {
		if effect.Aura == 44 || effect.Aura == 45 || effect.Aura == 151 {
			return true
		}
	}
	return false
}

func (rt *Runtime) trackingSpells() []SpellInfo {
	var spells []SpellInfo
	for id := range rt.gameplay.Known {
		if info, ok := rt.gameplay.Catalog[id]; ok && spellTracks(info) {
			spells = append(spells, info)
		}
	}
	sort.Slice(spells, func(a, b int) bool {
		if spells[a].Name != spells[b].Name {
			return spells[a].Name < spells[b].Name
		}
		return spells[a].ID < spells[b].ID
	})
	return spells
}

func (rt *Runtime) trackingOthersForPlayer() []*trackingOther {
	class := uint32(0)
	if info := rt.unitInfo("player"); info != nil {
		class = uint32(info.ClassID)
	}
	var list []*trackingOther
	for index := range trackingOthers {
		entry := &trackingOthers[index]
		if entry.classMask == 0 || entry.classMask&(1<<(class&31)) != 0 {
			list = append(list, entry)
		}
	}
	return list
}

// trackingAura is the tracking spell the player has an aura of.
func (rt *Runtime) trackingAura() uint32 {
	info := rt.unitInfo("player")
	if info == nil {
		return 0
	}
	for _, aura := range info.Auras {
		if spell, ok := rt.gameplay.Catalog[aura.SpellID]; ok && spellTracks(spell) {
			return aura.SpellID
		}
	}
	return 0
}

func (rt *Runtime) setTrackingOther(entry *trackingOther) {
	rt.trackedOther = entry
	key := ""
	if entry != nil {
		key = entry.key
	}
	setCVarValue(rt, "minimapTrackedInfo", key)
	rt.FireEvent("MINIMAP_UPDATE_TRACKING")
}

func registerTrackingAPI(rt *Runtime) {
	L := rt.L
	reg := func(name string, fn lua.LGFunction) { L.SetGlobal(name, L.NewFunction(fn)) }
	reg("GetNumTrackingTypes", func(L *lua.LState) int {
		L.Push(lua.LNumber(len(rt.trackingSpells()) + len(rt.trackingOthersForPlayer())))
		return 1
	})
	reg("GetTrackingInfo", func(L *lua.LState) int {
		index := L.CheckInt(1) - 1
		spells := rt.trackingSpells()
		if index >= 0 && index < len(spells) {
			spell := spells[index]
			L.Push(lua.LString(spell.Name))
			L.Push(lua.LString(spell.Icon))
			if rt.trackingAura() == spell.ID && rt.trackedOther == nil {
				L.Push(lua.LNumber(1))
			} else {
				L.Push(lua.LNil)
			}
			L.Push(lua.LString("spell"))
			return 4
		}
		others := rt.trackingOthersForPlayer()
		if index -= len(spells); index >= 0 && index < len(others) {
			entry := others[index]
			L.Push(lua.LString(rt.globalString(entry.key, entry.key)))
			L.Push(lua.LString(trackingIconDir + entry.icon))
			if rt.trackedOther == entry {
				L.Push(lua.LNumber(1))
			} else {
				L.Push(lua.LNil)
			}
			L.Push(lua.LString("other"))
			return 4
		}
		return 0
	})
	reg("GetTrackingTexture", func(L *lua.LState) int {
		if rt.trackedOther != nil {
			L.Push(lua.LString(trackingIconDir + rt.trackedOther.icon))
			return 1
		}
		if id := rt.trackingAura(); id != 0 {
			L.Push(lua.LString(rt.gameplay.Catalog[id].Icon))
			return 1
		}
		L.Push(lua.LString(trackingIconDir + "None"))
		return 1
	})
	reg("SetTracking", func(L *lua.LState) int {
		if L.Get(1).Type() != lua.LTNumber {
			// Nothing tracked: cancel the tracking aura and clear the choice.
			if id := rt.trackingAura(); id != 0 {
				if host, ok := rt.Host.(WorldAuraHost); ok {
					_ = host.CancelAura(id)
				}
			}
			rt.setTrackingOther(nil)
			return 0
		}
		index := L.CheckInt(1) - 1
		spells := rt.trackingSpells()
		if index >= 0 && index < len(spells) {
			_ = rt.cast(spells[index].ID, "")
			return 0
		}
		others := rt.trackingOthersForPlayer()
		if index -= len(spells); index >= 0 && index < len(others) {
			if id := rt.trackingAura(); id != 0 {
				if host, ok := rt.Host.(WorldAuraHost); ok {
					_ = host.CancelAura(id)
				}
			}
			rt.setTrackingOther(others[index])
		}
		return 0
	})
}
