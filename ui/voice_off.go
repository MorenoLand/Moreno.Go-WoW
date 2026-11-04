package ui

import lua "github.com/yuin/gopher-lua"

// registerVoiceOffAPI defines the calls that look at the chat sound system and
// the list of searches for a group. Voice chat is not running and no list of
// groups arrives, so they report nothing.
func registerVoiceOffAPI(rt *Runtime) {
	L := rt.L
	reg := func(name string, fn lua.LGFunction) { L.SetGlobal(name, L.NewFunction(fn)) }
	zero := func(L *lua.LState) int {
		L.Push(lua.LNumber(0))
		return 1
	}
	reg("Sound_ChatSystem_GetNumInputDrivers", zero)
	reg("Sound_ChatSystem_GetNumOutputDrivers", zero)
	reg("GetNumMapDebugObjects", zero)
	driverName := func(usage string) lua.LGFunction {
		return func(L *lua.LState) int {
			if L.Get(1).Type() != lua.LTNumber {
				L.RaiseError("Usage: %s", usage)
				return 0
			}
			L.Push(lua.LString(""))
			return 1
		}
	}
	reg("Sound_ChatSystem_GetInputDriverNameByIndex", driverName("Sound_ChatSystem_GetInputDriverNameByIndex(InputDriverIndex)"))
	reg("Sound_ChatSystem_GetOutputDriverNameByIndex", driverName("Sound_ChatSystem_GetOutputDriverNameByIndex(OutputDriverIndex)"))
	reg("SearchLFGGetResults", func(L *lua.LState) int {
		if L.Get(1).Type() != lua.LTNumber {
			L.RaiseError("Usage: SearchLFGGetResults(index)")
		}
		return 0
	})
	reg("SearchLFGGetPartyResults", func(L *lua.LState) int {
		if L.Get(1).Type() != lua.LTNumber || L.Get(2).Type() != lua.LTNumber {
			L.RaiseError("Usage: SearchLFGGetPartyResults(index, partyIndex)")
		}
		return 0
	})
	reg("SearchLFGGetEncounterResults", func(L *lua.LState) int {
		if L.Get(1).Type() != lua.LTNumber || L.Get(2).Type() != lua.LTNumber {
			L.RaiseError("Usage: SearchLFGGetEncounterResults(index, encounterIndex)")
		}
		return 0
	})
	// Nothing is on a voice channel: no channel is active, nobody is silenced
	// and no session has members.
	nothing := func(L *lua.LState) int { return 0 }
	for _, name := range []string{
		"GetActiveVoiceChannel", "VoiceIsDisabledByClient", "IsSilenced", "UnitIsSilenced",
		"GetVoiceStatus", "GetNumVoiceSessionMembersBySessionID", "GetVoiceSessionMemberInfoBySessionID",
		"ChannelSilenceVoice", "ChannelUnSilenceVoice", "VoiceSelectCaptureDevice", "VoiceSelectOutputDevice",
		"VoiceChat_PlayLoopbackSound", "VoiceChat_RecordLoopbackSound", "VoiceChat_StopRecordingLoopbackSound",
	} {
		reg(name, nothing)
	}
	reg("SetActiveVoiceChannel", func(L *lua.LState) int {
		if L.Get(1).Type() != lua.LTNumber {
			return 0
		}
		if L.CheckInt(1) == 0 {
			rt.FireEvent("CHANNEL_VOICE_UPDATE", lua.LNumber(1))
		}
		return 0
	})
}
