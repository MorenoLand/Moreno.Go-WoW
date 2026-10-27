package ui

import (
	lua "github.com/yuin/gopher-lua"
)

// registerGapAPI defines the calls the stock interface makes for features this
// client does not carry: each answers as the client does with none of it.
func registerGapAPI(rt *Runtime) {
	L := rt.L
	define := func(name string, fn lua.LGFunction) {
		if L.GetGlobal(name) == lua.LNil {
			L.SetGlobal(name, L.NewFunction(fn))
		}
	}
	returns := func(name string, values ...lua.LValue) {
		define(name, func(L *lua.LState) int {
			for _, value := range values {
				L.Push(value)
			}
			return len(values)
		})
	}
	// The knowledge base the customer support window reads: none is loaded.
	returns("KBSetup_IsLoaded", lua.LFalse)
	for _, name := range []string{"KBSetup_GetCategoryCount", "KBSetup_GetSubCategoryCount", "KBSetup_GetTotalArticleCount",
		"KBSetup_GetArticleHeaderCount", "KBQuery_GetTotalArticleCount", "KBQuery_GetArticleHeaderCount"} {
		returns(name, lua.LNumber(0))
	}
	for _, name := range []string{"KBSetup_GetCategoryData", "KBSetup_GetSubCategoryData", "KBArticle_GetData", "KBSystem_GetMOTD", "KBSystem_GetServerNotice"} {
		returns(name)
	}
	// Movie recording is a Mac feature.
	for _, name := range []string{"MovieRecording_IsCodecSupported", "MovieRecording_IsCursorRecordingSupported"} {
		returns(name, lua.LFalse)
	}
	for _, name := range []string{"MovieRecording_DataRate", "MovieRecording_GetAspectRatio", "MovieRecording_GetProgress",
		"MovieRecording_GetViewportWidth", "MovieRecording_MaxLength"} {
		returns(name, lua.LNumber(0))
	}
	// Voice chat devices: there are none.
	for _, name := range []string{"VoiceChat_IsPlayingLoopbackSound", "VoiceChat_IsRecordingLoopbackSound"} {
		returns(name, lua.LFalse)
	}
	returns("VoiceChat_GetCurrentMicrophoneSignalLevel", lua.LNumber(0))
	// Every player is on this realm.
	returns("UnitIsSameServer", lua.LTrue)
	// Container items bought with honor or arena points, gems that trade, and
	// calendar event pictures.
	returns("GetContainerItemPurchaseInfo")
	returns("GetContainerItemPurchaseItem")
	returns("GetSocketItemBoundTradeable", lua.LFalse)
	returns("GetSocketItemRefundable", lua.LFalse)
	returns("CalendarEventGetTextures")
}

// PlayerMotion is what the world tells of how the player moves and where.
type PlayerMotion struct {
	Swimming, Flying, Indoors, FlyableArea bool
}

// SetPlayerMotion records how the player moves and whether they are indoors.
func (rt *Runtime) SetPlayerMotion(motion PlayerMotion) { rt.motion = motion }

// spellNamed finds a spell of the catalog by name or id.
func (rt *Runtime) spellNamed(L *lua.LState, index int) (SpellInfo, bool) {
	switch value := L.Get(index).(type) {
	case lua.LNumber:
		info, ok := rt.gameplay.Catalog[uint32(value)]
		return info, ok
	case lua.LString:
		for _, info := range rt.gameplay.Catalog {
			if info.Name == string(value) {
				return info, true
			}
		}
	}
	return SpellInfo{}, false
}

// registerPlayerStateAPI defines the questions about the player's own state.
func registerPlayerStateAPI(rt *Runtime) {
	L := rt.L
	reg := func(name string, fn lua.LGFunction) { L.SetGlobal(name, L.NewFunction(fn)) }
	reg("IsSwimming", func(L *lua.LState) int { L.Push(lua.LBool(rt.motion.Swimming)); return 1 })
	reg("IsFlying", func(L *lua.LState) int { L.Push(lua.LBool(rt.motion.Flying)); return 1 })
	reg("IsIndoors", func(L *lua.LState) int { L.Push(lua.LBool(rt.motion.Indoors)); return 1 })
	reg("IsOutdoors", func(L *lua.LState) int { L.Push(lua.LBool(!rt.motion.Indoors)); return 1 })
	reg("IsFlyableArea", func(L *lua.LState) int { L.Push(lua.LBool(rt.motion.FlyableArea)); return 1 })
	reg("IsMounted", func(L *lua.LState) int {
		info := rt.unitInfo("player")
		L.Push(lua.LBool(info != nil && info.Mounted))
		return 1
	})
	reg("IsStealthed", func(L *lua.LState) int {
		stealthed := false
		if info := rt.unitInfo("player"); info != nil {
			for _, aura := range info.Auras {
				for _, effect := range rt.gameplay.Catalog[aura.SpellID].Effects {
					if effect.Aura == spellAuraStealth {
						stealthed = true
					}
				}
			}
		}
		L.Push(lua.LBool(stealthed))
		return 1
	})
	reg("GetShapeshiftForm", func(L *lua.LState) int { L.Push(lua.LNumber(0)); return 1 })
	reg("GetMouseButtonClicked", func(L *lua.LState) int {
		if rt.lastButton == "" {
			L.Push(lua.LString("LeftButton"))
		} else {
			L.Push(lua.LString(rt.lastButton))
		}
		return 1
	})
	spellKind := func(helpful bool) lua.LGFunction {
		return func(L *lua.LState) int {
			if info, ok := rt.spellNamed(L, 1); ok {
				L.Push(lua.LBool(spellIsHelpful(info) == helpful))
				return 1
			}
			L.Push(lua.LNil)
			return 1
		}
	}
	reg("IsHelpfulSpell", spellKind(true))
	reg("IsHarmfulSpell", spellKind(false))
	reg("IsHelpfulItem", func(L *lua.LState) int { L.Push(lua.LNil); return 1 })
	reg("IsHarmfulItem", func(L *lua.LState) int { L.Push(lua.LNil); return 1 })
}

// spellAuraStealth is the aura type of stealth.
const spellAuraStealth = 16
