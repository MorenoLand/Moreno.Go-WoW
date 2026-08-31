package ui

import (
	"runtime"
	"strconv"
	"strings"

	lua "github.com/yuin/gopher-lua"
)

// Build constants reported by GetBuildInfo, matching the reference client
// release this runtime targets.
const (
	buildVersion = "3.3.5"
	buildNumber  = "12340"
	buildDate    = "Jun 24 2010"
	buildTOC     = 30300
)

type createRaceInfo struct {
	key     string
	file    string
	scene   string
	faction string
}

type createClassInfo struct {
	key                  string
	file                 string
	tank, healer, damage bool
}

var createRaces = []createRaceInfo{{"RACE_HUMAN", "Human", "Human", "Alliance"}, {"RACE_DWARF", "Dwarf", "Dwarf", "Alliance"}, {"RACE_GNOME", "Gnome", "Dwarf", "Alliance"}, {"RACE_NIGHTELF", "NightElf", "NightElf", "Alliance"}, {"RACE_TAUREN", "Tauren", "Tauren", "Horde"}, {"RACE_SCOURGE", "Scourge", "Scourge", "Horde"}, {"RACE_TROLL", "Troll", "Orc", "Horde"}, {"RACE_ORC", "Orc", "Orc", "Horde"}, {"RACE_BLOODELF", "BloodElf", "BloodElf", "Horde"}, {"RACE_DRAENEI", "Draenei", "Draenei", "Alliance"}}

var createClasses = []createClassInfo{{"WARRIOR", "WARRIOR", true, false, true}, {"PALADIN", "PALADIN", true, true, true}, {"HUNTER", "HUNTER", false, false, true}, {"ROGUE", "ROGUE", false, false, true}, {"PRIEST", "PRIEST", false, true, true}, {"DEATHKNIGHT", "DEATHKNIGHT", true, false, true}, {"SHAMAN", "SHAMAN", false, true, true}, {"MAGE", "MAGE", false, false, true}, {"WARLOCK", "WARLOCK", false, false, true}, {"DRUID", "DRUID", true, true, true}}

// GlueState tracks connection-flow state the glue API surfaces to scripts.
type GlueState struct {
	Connected         bool
	ServerName        string
	PendingRealmList  bool
	SelectedRealm     int
	Realms            []RealmInfo
	SelectedCharacter int
	Characters        []CharacterEntry
	AddOns            []AddonInfo
}

// RealmInfo describes one realm entry returned by the realm list.
type RealmInfo struct {
	Name       string
	Address    string
	Population string
	RealmType  string
	Locale     string
	ID         int
	Characters int
	Invalid    bool
	Down       bool
	Current    bool
	PVP        bool
	RP         bool
	Load       float64
	Locked     bool
	Major      int
	Minor      int
	Revision   int
	Build      string
}

type AddonInfo struct {
	Name       string
	Title      string
	Notes      string
	URL        string
	Loadable   bool
	Reason     string
	Security   string
	NewVersion bool
	Enabled    bool
}

// CharacterEntry describes one character in the character list.
type CharacterEntry struct {
	Name              string
	Race              string
	RaceID            int
	Class             string
	ClassID           int
	Gender            int
	Level             int
	Zone              string
	ZoneID            uint32
	MapID             uint32
	Flags             uint32
	CustomizeFlags    uint32
	Ghost             bool
	PaidCustomization bool
	PaidRaceChange    bool
	PaidFactionChange bool
	BackgroundModel   string
}

func registerGlueAPI(rt *Runtime) {
	L := rt.L
	reg := func(name string, fn func(L *lua.LState) int) {
		L.SetGlobal(name, L.NewFunction(func(L *lua.LState) int {
			defer func() {
				if r := recover(); r != nil {
					L.RaiseError("go panic in %s: %v", name, r)
				}
			}()
			return fn(L)
		}))
	}

	// Global helpers.
	reg("getglobal", func(L *lua.LState) int {
		L.Push(L.GetGlobal(L.CheckString(1)))
		return 1
	})
	reg("setglobal", func(L *lua.LState) int {
		L.SetGlobal(L.CheckString(1), L.Get(2))
		return 0
	})

	// Build and platform info.
	reg("GetBuildInfo", func(L *lua.LState) int {
		versionType := L.GetGlobal("NORMAL_BUILD")
		if versionType.Type() != lua.LTString {
			versionType = lua.LString("")
		}
		buildType := L.GetGlobal("RELEASE_BUILD")
		if buildType.Type() != lua.LTString {
			buildType = lua.LString("")
		}
		L.Push(versionType)
		L.Push(buildType)
		L.Push(lua.LString(buildVersion))
		L.Push(lua.LString(buildNumber))
		L.Push(lua.LString(buildDate))
		return 5
	})
	reg("IsWindowsClient", func(L *lua.LState) int { L.Push(lua.LBool(runtime.GOOS == "windows")); return 1 })
	reg("IsMacClient", func(L *lua.LState) int { L.Push(lua.LBool(false)); return 1 })
	reg("IsLinuxClient", func(L *lua.LState) int { L.Push(lua.LBool(runtime.GOOS == "linux")); return 1 })
	reg("GetLocale", func(L *lua.LState) int { L.Push(lua.LString("enUS")); return 1 })
	reg("GetClientExpansionLevel", func(L *lua.LState) int { L.Push(lua.LNumber(3)); return 1 })
	reg("GetAccountExpansionLevel", func(L *lua.LState) int { L.Push(lua.LNumber(2)); return 1 })
	reg("IsShiftKeyDown", func(L *lua.LState) int { L.Push(lua.LBool(false)); return 1 })

	// Screen state transitions.
	reg("SetCurrentScreen", func(L *lua.LState) int {
		// SetCurrentScreen is called by the interface layer to notify the C engine
		// that the screen changed. It should NOT fire SET_GLUE_SCREEN because that
		// would cause an infinite loop.
		return 0
	})

	// Screen geometry.
	screen := func() (float64, float64) {
		if rt.Host != nil {
			return rt.Host.ScreenSize()
		}
		return 1280, 960
	}
	reg("GetScreenWidth", func(L *lua.LState) int {
		w, _ := screen()
		L.Push(lua.LNumber(w))
		return 1
	})
	reg("GetScreenHeight", func(L *lua.LState) int {
		_, h := screen()
		L.Push(lua.LNumber(h))
		return 1
	})
	reg("GetCursorPosition", func(L *lua.LState) int {
		L.Push(lua.LNumber(0))
		L.Push(lua.LNumber(0))
		return 2
	})
	reg("ShowCursor", func(L *lua.LState) int { return 0 })
	reg("HideCursor", func(L *lua.LState) int { return 0 })

	// Console variables.
	reg("GetCVar", func(L *lua.LState) int {
		if v, ok := rt.GetCVar(L.CheckString(1)); ok {
			L.Push(lua.LString(v))
		} else if value, ok := defaultCVarValue(L.CheckString(1)); ok {
			L.Push(lua.LString(value))
		} else {
			L.Push(lua.LString("0"))
		}
		return 1
	})
	reg("GetCVarBool", func(L *lua.LState) int {
		if v, ok := rt.GetCVar(L.CheckString(1)); ok {
			L.Push(lua.LBool(v == "1" || strings.EqualFold(v, "true")))
		} else {
			L.Push(lua.LFalse)
		}
		return 1
	})
	reg("SetCVar", func(L *lua.LState) int {
		name := L.CheckString(1)
		value := ""
		switch L.Get(2).Type() {
		case lua.LTString:
			value = L.CheckString(2)
		case lua.LTBool:
			value = map[bool]string{true: "1", false: "0"}[L.CheckBool(2)]
		default:
			value = L.Get(2).String()
		}
		setCVarValue(rt, name, value)
		return 0
	})
	reg("GetCVarDefault", func(L *lua.LState) int {
		if v, ok := rt.cvarDefaults[strings.ToLower(L.CheckString(1))]; ok {
			L.Push(lua.LString(v))
		} else if value, ok := defaultCVarValue(L.CheckString(1)); ok {
			L.Push(lua.LString(value))
		} else {
			L.Push(lua.LString("0"))
		}
		return 1
	})
	reg("GetCVarMin", func(L *lua.LState) int { L.Push(lua.LNil); return 1 })
	reg("GetCVarMax", func(L *lua.LState) int { L.Push(lua.LNil); return 1 })

	// Audio.
	reg("PlaySound", func(L *lua.LState) int {
		if rt.Host != nil {
			rt.Host.PlaySound(L.CheckString(1))
		}
		return 0
	})
	reg("PlaySoundFile", func(L *lua.LState) int {
		if rt.Host != nil {
			rt.Host.PlaySound(L.CheckString(1))
		}
		return 0
	})
	reg("PlayGlueMusic", func(L *lua.LState) int {
		if rt.Host != nil {
			rt.Host.PlayMusic(L.CheckString(1))
		}
		return 0
	})
	reg("StopGlueMusic", func(L *lua.LState) int {
		if rt.Host != nil {
			rt.Host.StopMusic()
		}
		return 0
	})
	reg("PlayGlueAmbience", func(L *lua.LState) int {
		if rt.Host != nil {
			rt.Host.PlayAmbience(L.CheckString(1))
		}
		return 0
	})
	reg("StopGlueAmbience", func(L *lua.LState) int {
		if rt.Host != nil {
			rt.Host.StopAmbience()
		}
		return 0
	})
	reg("StopAllSFX", func(L *lua.LState) int {
		if rt.Host != nil {
			rt.Host.StopAllSFX()
		}
		return 0
	})
	reg("PlayCreditsMusic", func(L *lua.LState) int {
		if rt.Host != nil {
			rt.Host.PlayMusic(L.CheckString(1))
		}
		return 0
	})
	reg("ActionStatus_DisplayMessage", func(L *lua.LState) int { return 0 })
	reg("Sound_ToggleMusic", func(L *lua.LState) int {
		if cvarValue(rt, "Sound_EnableAllSound") == "0" {
			return 0
		}
		if cvarValue(rt, "Sound_EnableMusic") == "0" {
			setCVarValue(rt, "Sound_EnableMusic", "1")
		} else {
			setCVarValue(rt, "Sound_EnableMusic", "0")
		}
		return 0
	})
	reg("Sound_ToggleSound", func(L *lua.LState) int {
		if cvarValue(rt, "Sound_EnableAllSound") == "0" {
			return 0
		}
		if cvarValue(rt, "Sound_EnableSFX") == "0" {
			setCVarValue(rt, "Sound_EnableSFX", "1")
			setCVarValue(rt, "Sound_EnableAmbience", "1")
		} else {
			setCVarValue(rt, "Sound_EnableSFX", "0")
			setCVarValue(rt, "Sound_EnableAmbience", "0")
		}
		return 0
	})

	// Platform actions.
	reg("LaunchURL", func(L *lua.LState) int {
		if rt.Host != nil {
			rt.Host.LaunchURL(L.CheckString(1))
		}
		return 0
	})
	reg("QuitGame", func(L *lua.LState) int {
		if rt.Host != nil {
			rt.Host.Quit(false)
		}
		return 0
	})
	reg("QuitGameAndRunLauncher", func(L *lua.LState) int {
		if rt.Host != nil {
			rt.Host.Quit(true)
		}
		return 0
	})
	reg("Screenshot", func(L *lua.LState) int {
		if rt.Host != nil {
			rt.Host.Screenshot()
		}
		return 0
	})
	reg("ConsoleExec", func(L *lua.LState) int {
		if rt.Host != nil {
			rt.Host.ConsoleExec(L.CheckString(1))
		}
		L.Push(lua.LBool(true))
		return 1
	})

	// Saved account handling.
	reg("GetSavedAccountName", func(L *lua.LState) int {
		if v, ok := rt.GetCVar("accountName"); ok {
			L.Push(lua.LString(v))
		} else {
			L.Push(lua.LString(""))
		}
		return 1
	})
	reg("SetSavedAccountName", func(L *lua.LState) int {
		rt.SetCVar("accountName", L.CheckString(1))
		return 0
	})
	reg("GetSavedAccountList", func(L *lua.LState) int {
		if v, ok := rt.GetCVar("accountList"); ok {
			L.Push(lua.LString(v))
		} else {
			L.Push(lua.LNil)
		}
		return 1
	})
	reg("SetSavedAccountList", func(L *lua.LState) int {
		rt.SetCVar("accountList", L.CheckString(1))
		return 0
	})
	reg("SetUsesToken", func(L *lua.LState) int {
		rt.SetCVar("accountUsesToken", map[bool]string{true: "1", false: "0"}[L.CheckBool(1)])
		return 0
	})
	reg("GetUsesToken", func(L *lua.LState) int {
		if v, ok := rt.GetCVar("accountUsesToken"); ok {
			L.Push(lua.LBool(v == "1"))
		} else {
			L.Push(lua.LBool(false))
		}
		return 1
	})
	reg("DefaultServerLogin", func(L *lua.LState) int {
		if host, ok := rt.Host.(LoginHost); ok {
			host.DefaultServerLogin(L.CheckString(1), L.CheckString(2))
		}
		return 0
	})
	reg("EULAAccepted", func(L *lua.LState) int { L.Push(lua.LBool(true)); return 1 })
	reg("TOSAccepted", func(L *lua.LState) int { L.Push(lua.LBool(true)); return 1 })
	reg("TerminationWithoutNoticeAccepted", func(L *lua.LState) int { L.Push(lua.LBool(true)); return 1 })
	reg("ScanningAccepted", func(L *lua.LState) int { L.Push(lua.LBool(true)); return 1 })
	reg("ContestAccepted", func(L *lua.LState) int { L.Push(lua.LBool(true)); return 1 })

	// Connection flow queries. The login shell drives actual network actions;
	// these return the current glue state the scripts render from.
	reg("IsConnectedToServer", func(L *lua.LState) int {
		L.Push(lua.LBool(rt.Glue.Connected))
		return 1
	})
	reg("GetServerName", func(L *lua.LState) int {
		if rt.Glue.ServerName == "" {
			L.Push(lua.LNil)
		} else {
			L.Push(lua.LString(rt.Glue.ServerName))
		}
		return 1
	})
	reg("CancelLogin", func(L *lua.LState) int { return 0 })
	reg("DisconnectFromServer", func(L *lua.LState) int {
		rt.Glue.Connected = false
		return 0
	})

	// Realm list.
	reg("GetNumRealms", func(L *lua.LState) int {
		L.Push(lua.LNumber(len(rt.Glue.Realms)))
		return 1
	})
	reg("GetRealmInfo", func(L *lua.LState) int {
		idx := L.CheckInt(1)
		if L.GetTop() >= 2 {
			idx = L.CheckInt(2)
		}
		if idx < 1 || idx > len(rt.Glue.Realms) {
			L.Push(lua.LNil)
			return 1
		}
		r := rt.Glue.Realms[idx-1]
		L.Push(lua.LString(r.Name))
		L.Push(lua.LNumber(r.Characters))
		L.Push(lua.LBool(r.Invalid))
		L.Push(lua.LBool(r.Down))
		L.Push(lua.LBool(r.Current || (r.ID != 0 && r.ID == rt.Glue.SelectedRealm)))
		L.Push(lua.LBool(r.PVP))
		L.Push(lua.LBool(r.RP))
		L.Push(lua.LNumber(r.Load))
		L.Push(lua.LBool(r.Locked))
		if r.Major == 0 {
			L.Push(lua.LNil)
		} else {
			L.Push(lua.LNumber(r.Major))
		}
		if r.Minor == 0 {
			L.Push(lua.LNil)
		} else {
			L.Push(lua.LNumber(r.Minor))
		}
		if r.Revision == 0 {
			L.Push(lua.LNil)
		} else {
			L.Push(lua.LNumber(r.Revision))
		}
		if r.Build == "" {
			L.Push(lua.LNil)
		} else {
			L.Push(lua.LString(r.Build))
		}
		L.Push(lua.LString(r.RealmType))
		return 14
	})
	reg("RequestRealmList", func(L *lua.LState) int {
		rt.Glue.PendingRealmList = true
		rt.FireEvent("OPEN_REALM_LIST")
		return 0
	})
	reg("CancelRealmListQuery", func(L *lua.LState) int { rt.Glue.PendingRealmList = false; return 0 })
	reg("SortRealms", func(L *lua.LState) int { return 0 })
	reg("ChangeRealm", func(L *lua.LState) int {
		idx := L.CheckInt(2)
		if idx >= 1 && idx <= len(rt.Glue.Realms) {
			rt.Glue.SelectedRealm = rt.Glue.Realms[idx-1].ID
			rt.Glue.ServerName = rt.Glue.Realms[idx-1].Name
		}
		rt.Glue.PendingRealmList = false
		return 0
	})
	reg("RealmListUpdateRate", func(L *lua.LState) int { L.Push(lua.LNumber(5)); return 1 })
	reg("RealmListDialogCancelled", func(L *lua.LState) int { return 0 })

	// Character list.
	reg("GetNumCharacters", func(L *lua.LState) int {
		L.Push(lua.LNumber(len(rt.Glue.Characters)))
		return 1
	})
	reg("GetCharacterInfo", func(L *lua.LState) int {
		idx := L.CheckInt(1)
		if idx < 1 || idx > len(rt.Glue.Characters) {
			L.Push(lua.LNil)
			return 1
		}
		c := rt.Glue.Characters[idx-1]
		L.Push(lua.LString(c.Name))
		L.Push(lua.LString(localizedValue(L, c.Race)))
		L.Push(lua.LString(localizedValue(L, c.Class)))
		L.Push(lua.LNumber(c.Level))
		L.Push(lua.LString(c.Zone))
		L.Push(lua.LNumber(c.Gender))
		L.Push(lua.LBool(c.Ghost))
		L.Push(lua.LBool(c.PaidCustomization))
		L.Push(lua.LBool(c.PaidRaceChange))
		L.Push(lua.LBool(c.PaidFactionChange))
		return 10
	})
	reg("SelectCharacter", func(L *lua.LState) int {
		index := L.CheckInt(1)
		rt.Glue.SelectedCharacter = index
		rt.FireEvent("UPDATE_SELECTED_CHARACTER", lua.LNumber(index))
		return 0
	})
	reg("EnterWorld", func(L *lua.LState) int {
		if host, ok := rt.Host.(WorldHost); ok {
			host.EnterWorld(rt.Glue.SelectedCharacter - 1)
		}
		return 0
	})
	reg("GetSelectBackgroundModel", func(L *lua.LState) int {
		idx := L.CheckInt(1)
		if idx < 1 || idx > len(rt.Glue.Characters) {
			L.Push(lua.LNil)
			return 1
		}
		model := rt.Glue.Characters[idx-1].BackgroundModel
		if model == "" {
			model = strings.TrimPrefix(strings.ToUpper(rt.Glue.Characters[idx-1].Race), "RACE_")
		}
		L.Push(lua.LString(model))
		return 1
	})
	reg("GetCharacterSelectFacing", func(L *lua.LState) int { L.Push(lua.LNumber(0)); return 1 })
	reg("SetCharacterSelectFacing", func(L *lua.LState) int { return 0 })
	reg("GetCharacterCreateFacing", func(L *lua.LState) int { L.Push(lua.LNumber(0)); return 1 })
	reg("SetCharacterCreateFacing", func(L *lua.LState) int { return 0 })
	reg("RandomizeCharCustomization", func(L *lua.LState) int { return 0 })
	reg("CycleCharCustomization", func(L *lua.LState) int { return 0 })
	reg("UpdateCustomizationScene", func(L *lua.LState) int { return 0 })
	reg("UpdateSelectionCustomizationScene", func(L *lua.LState) int { return 0 })
	reg("SetCharCustomizeBackground", func(L *lua.LState) int { rt.SetCVar("charCustomizeBackground", L.CheckString(1)); return 0 })
	reg("SetCharSelectBackground", func(L *lua.LState) int { rt.SetCVar("charSelectBackground", L.CheckString(1)); return 0 })
	reg("ReadyForAccountDataTimes", func(L *lua.LState) int { return 0 })
	reg("RequestRealmSplitInfo", func(L *lua.LState) int { return 0 })

	// Notices and agreements.
	for _, name := range []string{
		"ShowEULANotice", "ShowTOSNotice", "ShowTerminationWithoutNoticeNotice",
		"ShowScanningNotice", "ShowContestNotice", "ShowChangedOptionWarnings",
		"AcceptChangedOptionWarnings",
	} {
		reg(name, func(L *lua.LState) int { return 0 })
	}
	reg("AcceptEULA", func(L *lua.LState) int { rt.SetCVar("showEULA", "0"); return 0 })
	reg("AcceptTOS", func(L *lua.LState) int { rt.SetCVar("showTOS", "0"); return 0 })
	reg("AcceptTerminationWithoutNotice", func(L *lua.LState) int { return 0 })
	reg("AcceptScanning", func(L *lua.LState) int { return 0 })
	reg("AcceptContest", func(L *lua.LState) int { return 0 })
	reg("GetCreditsText", func(L *lua.LState) int { L.Push(lua.LString("")); return 1 })

	// Patching and scanning (no-op in a client that ships complete data).
	reg("PatchDownloadProgress", func(L *lua.LState) int { L.Push(lua.LNumber(0)); return 1 })
	reg("PatchDownloadCancel", func(L *lua.LState) int { return 0 })
	reg("PatchDownloadApply", func(L *lua.LState) int { return 0 })
	reg("IsScanDLLFinished", func(L *lua.LState) int { L.Push(lua.LBool(true)); return 1 })
	reg("ScanDLLStart", func(L *lua.LState) int { return 0 })
	reg("ScanDLLContinueAnyway", func(L *lua.LState) int { return 0 })

	// Status dialog interaction from scripts.
	reg("StatusDialogClick", func(L *lua.LState) int { return 0 })

	// Movie playback.
	reg("GetMovieResolution", func(L *lua.LState) int {
		L.Push(lua.LNumber(1024))
		L.Push(lua.LNumber(768))
		return 2
	})

	// AddOn management (the glue addon list screen queries these).
	reg("GetNumAddOns", func(L *lua.LState) int { L.Push(lua.LNumber(len(rt.Glue.AddOns))); return 1 })
	reg("GetAddOnInfo", func(L *lua.LState) int {
		index := L.CheckInt(1)
		if index < 1 || index > len(rt.Glue.AddOns) {
			for i := 0; i < 8; i++ {
				L.Push(lua.LNil)
			}
			return 8
		}
		addon := rt.Glue.AddOns[index-1]
		L.Push(lua.LString(addon.Name))
		L.Push(lua.LString(addon.Title))
		L.Push(lua.LString(addon.Notes))
		L.Push(lua.LString(addon.URL))
		L.Push(lua.LBool(addon.Loadable))
		if addon.Reason == "" {
			L.Push(lua.LNil)
		} else {
			L.Push(lua.LString(addon.Reason))
		}
		if addon.Security == "" {
			L.Push(lua.LNil)
		} else {
			L.Push(lua.LString(addon.Security))
		}
		L.Push(lua.LBool(addon.NewVersion))
		return 8
	})
	reg("GetAddOnEnableState", func(L *lua.LState) int {
		index := addonIndexArg(L)
		if index >= 1 && index <= len(rt.Glue.AddOns) && rt.Glue.AddOns[index-1].Enabled {
			L.Push(lua.LNumber(2))
		} else {
			L.Push(lua.LNumber(0))
		}
		return 1
	})
	reg("EnableAddOn", func(L *lua.LState) int {
		index := addonIndexArg(L)
		if index >= 1 && index <= len(rt.Glue.AddOns) {
			rt.Glue.AddOns[index-1].Enabled = true
		}
		return 0
	})
	reg("DisableAddOn", func(L *lua.LState) int {
		index := addonIndexArg(L)
		if index >= 1 && index <= len(rt.Glue.AddOns) {
			rt.Glue.AddOns[index-1].Enabled = false
		}
		return 0
	})
	reg("EnableAllAddOns", func(L *lua.LState) int {
		for index := range rt.Glue.AddOns {
			rt.Glue.AddOns[index].Enabled = true
		}
		return 0
	})
	reg("DisableAllAddOns", func(L *lua.LState) int {
		for index := range rt.Glue.AddOns {
			rt.Glue.AddOns[index].Enabled = false
		}
		return 0
	})
	reg("ResetAddOns", func(L *lua.LState) int { return 0 })
	reg("SaveAddOns", func(L *lua.LState) int { return 0 })
	reg("SetAddonVersionCheck", func(L *lua.LState) int {
		if L.GetTop() > 0 {
			rt.addonVersionCheck = L.Get(1) != lua.LFalse && L.Get(1) != lua.LNil && L.Get(1).String() != "0"
		}
		return 0
	})
	reg("IsAddonVersionCheckEnabled", func(L *lua.LState) int { L.Push(lua.LBool(rt.addonVersionCheck)); return 1 })
	reg("GetAddOnDependencies", func(L *lua.LState) int { return 0 })
	reg("LaunchAddOnURL", func(L *lua.LState) int { return 0 })

	// Billing.
	reg("GetBillingPlan", func(L *lua.LState) int { L.Push(lua.LNumber(0)); return 1 })
	reg("GetBillingTimeRemaining", func(L *lua.LState) int { L.Push(lua.LNumber(0)); return 1 })
	reg("GetBillingTimeRested", func(L *lua.LState) int { L.Push(lua.LNumber(0)); return 1 })

	// Security matrix (unused with no authenticator traffic).
	reg("GetMatrixCoordinates", func(L *lua.LState) int { L.Push(lua.LNumber(0)); L.Push(lua.LNumber(0)); return 2 })
	reg("MatrixCommit", func(L *lua.LState) int { return 0 })
	reg("MatrixRevert", func(L *lua.LState) int { return 0 })

	// Error reporting hooks the glue scripts install.
	reg("seterrorhandler", func(L *lua.LState) int {
		if L.Get(1).Type() == lua.LTFunction {
			rt.scriptErrorHandler = L.Get(1).(*lua.LFunction)
		}
		return 0
	})
	reg("debuginfo", func(L *lua.LState) int { return 0 })
	reg("message", func(L *lua.LState) int { return 0 })

	// Format globals (string library aliases).
	reg("format", func(L *lua.LState) int {
		n := L.GetTop()
		fmtFn := L.GetGlobal("string").(*lua.LTable).RawGetString("format")
		L.Push(fmtFn)
		for i := 1; i <= n; i++ {
			L.Push(L.Get(i))
		}
		L.Call(n, 1)
		return 1
	})
	L.SetGlobal("gsub", L.GetGlobal("string").(*lua.LTable).RawGetString("gsub"))

	// Video system queries. The original client answers from the graphics
	// device; an unconnected host reports one fixed mode.
	reg("GetScreenResolutions", func(L *lua.LState) int {
		L.Push(lua.LString("1280x960"))
		return 1
	})
	reg("GetCurrentResolution", func(L *lua.LState) int { L.Push(lua.LNumber(1)); return 1 })
	reg("GetRefreshRates", func(L *lua.LState) int { L.Push(lua.LNumber(60)); return 1 })
	reg("GetMultisampleFormats", func(L *lua.LState) int { L.Push(lua.LNumber(1)); return 1 })
	reg("GetCurrentMultisampleFormat", func(L *lua.LState) int { L.Push(lua.LNumber(1)); return 1 })
	reg("SetMultisampleFormat", func(L *lua.LState) int { return 0 })
	reg("SetScreenResolution", func(L *lua.LState) int { return 0 })
	reg("GetGamma", func(L *lua.LState) int {
		if value, ok := rt.GetCVar("gamma"); ok {
			if number, err := strconv.ParseFloat(value, 64); err == nil {
				L.Push(lua.LNumber(number))
				return 1
			}
		}
		L.Push(lua.LNumber(0))
		return 1
	})
	reg("SetGamma", func(L *lua.LState) int {
		rt.SetCVar("gamma", L.Get(1).String())
		return 0
	})
	reg("GetTerrainMip", func(L *lua.LState) int { L.Push(lua.LNumber(0)); return 1 })
	reg("SetTerrainMip", func(L *lua.LState) int { return 0 })
	reg("IsPlayerResolutionAvailable", func(L *lua.LState) int { L.Push(lua.LBool(true)); return 1 })
	reg("IsStereoVideoAvailable", func(L *lua.LState) int { L.Push(lua.LBool(false)); return 1 })
	reg("GetVideoCaps", func(L *lua.LState) int { L.Push(lua.LNumber(0)); return 1 })

	// Account and trial state.
	reg("IsStreamingTrial", func(L *lua.LState) int { L.Push(lua.LBool(false)); return 1 })
	reg("IsTrialAccount", func(L *lua.LState) int { L.Push(lua.LBool(false)); return 1 })
	reg("IsInvalidLocale", func(L *lua.LState) int { L.Push(lua.LBool(false)); return 1 })
	reg("IsTournamentRealmCategory", func(L *lua.LState) int { L.Push(lua.LBool(false)); return 1 })
	reg("IsInvalidTournamentRealmCategory", func(L *lua.LState) int { L.Push(lua.LBool(false)); return 1 })
	reg("GetRealmCategories", func(L *lua.LState) int { L.Push(lua.LNumber(0)); return 1 })

	// Character creation and selection scene control.
	reg("SetCharSelectModelFrame", func(L *lua.LState) int { rt.SetCVar("charSelectModel", L.CheckString(1)); return 0 })
	reg("SetCharCustomizeFrame", func(L *lua.LState) int { rt.SetCVar("charCustomizeModel", L.CheckString(1)); return 0 })
	reg("SetRaceSelectFrame", func(L *lua.LState) int { rt.SetCVar("raceSelectModel", L.CheckString(1)); return 0 })
	reg("GetSelectedRace", func(L *lua.LState) int { L.Push(lua.LNumber(rt.selectedRace)); return 1 })
	reg("SetSelectedRace", func(L *lua.LState) int { rt.selectedRace = clampCreateIndex(L.CheckInt(1), len(createRaces)); return 0 })
	reg("GetSelectedSex", func(L *lua.LState) int { L.Push(lua.LNumber(rt.selectedSex)); return 1 })
	reg("SetSelectedSex", func(L *lua.LState) int {
		sex := L.CheckInt(1)
		if sex == 2 || sex == 3 {
			rt.selectedSex = sex
		}
		return 0
	})
	reg("GetSelectedClass", func(L *lua.LState) int {
		index := rt.selectedClass
		if !validCreateClass(rt.selectedRace, index) {
			for candidate := range createClasses {
				if validCreateClass(rt.selectedRace, candidate+1) {
					index = candidate + 1
					break
				}
			}
			rt.selectedClass = index
		}
		class := createClasses[index-1]
		L.Push(lua.LString(localizedValue(L, class.key)))
		L.Push(lua.LString(class.file))
		L.Push(lua.LNumber(index))
		L.Push(lua.LBool(class.tank))
		L.Push(lua.LBool(class.healer))
		L.Push(lua.LBool(class.damage))
		return 6
	})
	reg("SetSelectedClass", func(L *lua.LState) int {
		index := L.CheckInt(1)
		if validCreateClass(rt.selectedRace, index) {
			rt.selectedClass = index
		}
		return 0
	})
	reg("GetSelectedCategory", func(L *lua.LState) int { L.Push(lua.LNumber(1)); return 1 })
	reg("GetNameForRace", func(L *lua.LState) int {
		index := clampCreateIndex(rt.selectedRace, len(createRaces))
		race := createRaces[index-1]
		L.Push(lua.LString(localizedValue(L, race.key)))
		L.Push(lua.LString(race.file))
		return 2
	})
	reg("GetFactionForRace", func(L *lua.LState) int {
		index := rt.selectedRace
		if L.GetTop() > 0 {
			index = L.CheckInt(1)
		}
		index = clampCreateIndex(index, len(createRaces))
		race := createRaces[index-1]
		L.Push(lua.LString(localizedValue(L, race.key)))
		L.Push(lua.LString(race.faction))
		return 2
	})
	reg("GetAvailableRaces", func(L *lua.LState) int {
		for _, race := range createRaces {
			L.Push(lua.LString(localizedValue(L, race.key)))
			L.Push(lua.LString(race.file))
			L.Push(lua.LNumber(1))
		}
		return len(createRaces) * 3
	})
	reg("GetAvailableClasses", func(L *lua.LState) int {
		for index, class := range createClasses {
			L.Push(lua.LString(localizedValue(L, class.key)))
			L.Push(lua.LString(class.file))
			if validCreateClass(rt.selectedRace, index+1) {
				L.Push(lua.LNumber(1))
			} else {
				L.Push(lua.LNumber(0))
			}
		}
		return len(createClasses) * 3
	})
	reg("GetClassesForRace", func(L *lua.LState) int {
		for index, class := range createClasses {
			L.Push(lua.LString(localizedValue(L, class.key)))
			L.Push(lua.LString(class.file))
			if validCreateClass(L.CheckInt(1), index+1) {
				L.Push(lua.LNumber(1))
			} else {
				L.Push(lua.LNumber(0))
			}
		}
		return len(createClasses) * 3
	})
	reg("IsRaceClassValid", func(L *lua.LState) int { L.Push(lua.LBool(validCreateClass(L.CheckInt(1), L.CheckInt(2)))); return 1 })
	reg("GetFacialHairCustomization", func(L *lua.LState) int { L.Push(lua.LString("NORMAL")); L.Push(lua.LString("NORMAL")); return 2 })
	reg("GetHairCustomization", func(L *lua.LState) int { L.Push(lua.LString("NORMAL")); L.Push(lua.LString("NORMAL")); return 2 })
	reg("GetCreateBackgroundModel", func(L *lua.LState) int {
		index := clampCreateIndex(rt.selectedRace, len(createRaces))
		L.Push(lua.LString(createRaces[index-1].scene))
		return 1
	})
	reg("ResetCharCustomize", func(L *lua.LState) int { return 0 })
	reg("RandomName", func(L *lua.LState) int { L.Push(lua.LString("")); return 1 })
	reg("CreateCharacter", func(L *lua.LState) int { return 0 })
	reg("DeleteCharacter", func(L *lua.LState) int { return 0 })
	reg("RenameCharacter", func(L *lua.LState) int { return 0 })
	reg("CustomizeExistingCharacter", func(L *lua.LState) int { return 0 })
	reg("GetCharacterListUpdate", func(L *lua.LState) int { L.Push(lua.LBool(false)); return 1 })

	// Frame creation for script-driven widgets (dropdown menus). The fourth
	// argument names a virtual template the new frame inherits.
	reg("CreateFrame", func(L *lua.LState) int {
		frameType := L.CheckString(1)
		name := ""
		if L.Get(2).Type() == lua.LTString {
			name = L.CheckString(2)
		}
		parent := (*widget)(nil)
		if ud, ok := L.Get(3).(*lua.LUserData); ok {
			if p, ok := ud.Value.(*widget); ok {
				parent = p
			}
		}
		template := ""
		if L.Get(4).Type() == lua.LTString {
			template = L.CheckString(4)
		}
		if parent != nil {
			name = resolveParentName(name, parent.name)
		}
		w := newWidget(kindFromObjectType(frameType), name)
		w.parent = parent
		addWidgetChild(parent, w)
		if template != "" && rt.instantiateTemplate != nil {
			rt.instantiateTemplate(w, template)
		}
		rt.register(w)
		rt.fireHandler(w, "OnLoad")
		L.Push(w.luaValue(L))
		return 1
	})
}

func kindFromObjectType(objectType string) widgetKind {
	switch objectType {
	case "Button":
		return kindButton
	case "CheckButton":
		return kindCheckButton
	case "EditBox":
		return kindEditBox
	case "Slider":
		return kindSlider
	case "ScrollFrame":
		return kindScrollFrame
	case "SimpleHTML":
		return kindSimpleHTML
	case "Model":
		return kindModel
	case "ModelFFX":
		return kindModelFFX
	case "MovieFrame":
		return kindMovieFrame
	default:
		return kindFrame
	}
}

func cvarValue(rt *Runtime, name string) string {
	if value, ok := rt.GetCVar(name); ok {
		return value
	}
	value, _ := defaultCVarValue(name)
	return value
}

func setCVarValue(rt *Runtime, name, value string) {
	rt.SetCVar(name, value)
	if host, ok := rt.Host.(AudioHost); ok {
		host.SetAudioCVar(name, value)
	}
}

func defaultCVarValue(name string) (string, bool) {
	switch strings.ToLower(name) {
	case "sound_enableallsound", "sound_enablemusic", "sound_enablesfx", "sound_enableambience", "sound_mastervolume", "sound_musicvolume", "sound_sfxvolume", "sound_ambiencevolume":
		return "1", true
	default:
		return "", false
	}
}

func localizedValue(L *lua.LState, key string) string {
	if value := L.GetGlobal(key); value.Type() == lua.LTString {
		return value.String()
	}
	return key
}

func clampCreateIndex(index, max int) int {
	if max == 0 {
		return 0
	}
	if index < 1 || index > max {
		return 1
	}
	return index
}

func validCreateClass(raceIndex, classIndex int) bool {
	if raceIndex < 1 || raceIndex > len(createRaces) || classIndex < 1 || classIndex > len(createClasses) {
		return false
	}
	valid := map[int]map[int]bool{
		1:  {1: true, 2: true, 3: true, 4: true, 5: true, 6: true, 8: true, 9: true},
		2:  {1: true, 2: true, 3: true, 4: true, 5: true, 6: true, 7: true},
		3:  {1: true, 4: true, 6: true, 8: true, 9: true},
		4:  {1: true, 3: true, 4: true, 5: true, 6: true, 10: true},
		5:  {1: true, 3: true, 5: true, 6: true, 7: true, 10: true},
		6:  {1: true, 4: true, 5: true, 6: true, 8: true, 9: true},
		7:  {1: true, 3: true, 4: true, 6: true, 8: true, 9: true, 10: true},
		8:  {1: true, 3: true, 4: true, 5: true, 6: true, 7: true, 8: true, 9: true, 10: true},
		9:  {1: true, 2: true, 3: true, 4: true, 5: true, 6: true, 8: true, 9: true},
		10: {1: true, 2: true, 3: true, 4: true, 5: true, 6: true, 7: true, 8: true},
	}
	return valid[raceIndex][classIndex]
}

func addonIndexArg(L *lua.LState) int {
	for index := L.GetTop(); index >= 1; index-- {
		if value, ok := L.Get(index).(lua.LNumber); ok {
			return int(value)
		}
	}
	return 0
}

// registerStringHelpers installs the string-function globals the interface
// scripts use; the original client provides these from its extended string
// library.
func registerStringHelpers(L *lua.LState) {
	L.SetGlobal("strupper", L.NewFunction(func(L *lua.LState) int {
		L.Push(lua.LString(strings.ToUpper(L.CheckString(1))))
		return 1
	}))
	L.SetGlobal("strlower", L.NewFunction(func(L *lua.LState) int {
		L.Push(lua.LString(strings.ToLower(L.CheckString(1))))
		return 1
	}))
	L.SetGlobal("strlen", L.NewFunction(func(L *lua.LState) int {
		L.Push(lua.LNumber(len(L.CheckString(1))))
		return 1
	}))
	L.SetGlobal("strsub", L.NewFunction(func(L *lua.LState) int {
		s := L.CheckString(1)
		start := L.CheckInt(2)
		if start < 0 {
			start = len(s) + start + 1
		}
		if start < 1 {
			start = 1
		}
		if L.GetTop() >= 3 {
			end := L.CheckInt(3)
			if end < 0 {
				end = len(s) + end + 1
			}
			if end > len(s) {
				end = len(s)
			}
			if end < start {
				L.Push(lua.LString(""))
				return 1
			}
			L.Push(lua.LString(s[start-1 : end]))
			return 1
		}
		if start > len(s) {
			L.Push(lua.LString(""))
			return 1
		}
		L.Push(lua.LString(s[start-1:]))
		return 1
	}))
	L.SetGlobal("strfind", L.GetGlobal("string").(*lua.LTable).RawGetString("find"))
	L.SetGlobal("strtrim", L.NewFunction(func(L *lua.LState) int {
		s := L.CheckString(1)
		cut := " \t\n\r"
		if L.GetTop() >= 2 {
			cut = L.CheckString(2)
		}
		L.Push(lua.LString(strings.Trim(s, cut)))
		return 1
	}))
	L.SetGlobal("strsplit", L.NewFunction(func(L *lua.LState) int {
		sep := L.CheckString(1)
		s := L.CheckString(2)
		pieces := -1
		if L.GetTop() >= 3 {
			pieces = L.CheckInt(3)
		}
		parts := strings.Split(s, sep)
		if pieces > 0 && len(parts) > pieces {
			head := parts[:pieces-1]
			tail := strings.Join(parts[pieces-1:], sep)
			parts = append(head, tail)
		}
		for _, p := range parts {
			L.Push(lua.LString(p))
		}
		return len(parts)
	}))
	L.SetGlobal("strjoin", L.NewFunction(func(L *lua.LState) int {
		sep := L.CheckString(1)
		parts := make([]string, 0, L.GetTop()-1)
		for i := 2; i <= L.GetTop(); i++ {
			parts = append(parts, L.Get(i).String())
		}
		L.Push(lua.LString(strings.Join(parts, sep)))
		return 1
	}))
	// Math library aliases, the set the embedded compat layer provides.
	math := L.GetGlobal("math").(*lua.LTable)
	for _, name := range []string{"floor", "ceil", "abs", "min", "max", "sqrt",
		"sin", "cos", "tan", "asin", "acos", "atan", "deg", "rad", "exp",
		"log", "log10", "fmod", "modf"} {
		if fn := math.RawGetString(name); fn.Type() == lua.LTFunction {
			L.SetGlobal(name, fn)
		}
	}

	// Sound device enumeration.
	L.SetGlobal("Sound_GameSystem_GetNumOutputDrivers", L.NewFunction(func(L *lua.LState) int { L.Push(lua.LNumber(0)); return 1 }))
	L.SetGlobal("Sound_GameSystem_GetOutputDriverNameByIndex", L.NewFunction(func(L *lua.LState) int { L.Push(lua.LString("")); return 1 }))
	L.SetGlobal("Sound_RestartSoundEngine", L.NewFunction(func(L *lua.LState) int { return 0 }))

	// Table library aliases the interface scripts rely on.
	L.SetGlobal("tinsert", L.GetGlobal("table").(*lua.LTable).RawGetString("insert"))
	L.SetGlobal("tremove", L.GetGlobal("table").(*lua.LTable).RawGetString("remove"))
	// SecureNext pairs with next for secure iteration; the glue scripts use
	// it as a plain iterator.
	safeNext := L.NewFunction(func(L *lua.LState) int {
		table := L.CheckTable(1)
		key := lua.LValue(lua.LNil)
		if L.GetTop() >= 2 {
			key = L.Get(2)
			switch key.Type() {
			case lua.LTUserData, lua.LTTable, lua.LTFunction:
				key = lua.LNil
			case lua.LTNumber:
				if key != lua.LNumber(0) && table.RawGet(key) == lua.LNil {
					key = lua.LNil
				}
			default:
				if key != lua.LNil && table.RawGet(key) == lua.LNil {
					key = lua.LNil
				}
			}
		}
		nextKey, value := table.Next(key)
		if nextKey == lua.LNil {
			L.Push(lua.LNil)
			return 1
		}
		L.Push(nextKey)
		L.Push(value)
		return 2
	})
	L.SetGlobal("next", safeNext)
	L.SetGlobal("SecureNext", safeNext)
	L.SetGlobal("issecure", L.NewFunction(func(L *lua.LState) int {
		L.Push(lua.LBool(true))
		return 1
	}))
	// securecall invokes a function with its arguments and propagates the
	// results; the first argument may also be a global function name.
	L.SetGlobal("securecall", L.NewFunction(func(L *lua.LState) int {
		n := L.GetTop()
		if n == 0 {
			return 0
		}
		fn := L.Get(1)
		if fn.Type() == lua.LTString {
			fn = L.GetGlobal(fn.String())
		}
		if fn.Type() != lua.LTFunction {
			return 0
		}
		args := make([]lua.LValue, n-1)
		for i := 2; i <= n; i++ {
			args[i-2] = L.Get(i)
		}
		L.SetTop(0)
		L.Push(fn)
		for _, arg := range args {
			L.Push(arg)
		}
		if err := L.PCall(len(args), lua.MultRet, nil); err != nil {
			return 0
		}
		return L.GetTop()
	}))
}
