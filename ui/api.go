package ui

import (
	"runtime"
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

// GlueState tracks connection-flow state the glue API surfaces to scripts.
type GlueState struct {
	Connected         bool
	ServerName        string
	PendingRealmList  bool
	SelectedRealm     int
	Realms            []RealmInfo
	SelectedCharacter int
	Characters        []CharacterEntry
}

// RealmInfo describes one realm entry returned by the realm list.
type RealmInfo struct {
	Name       string
	Address    string
	Population string
	RealmType  string
	Locale     string
}

// CharacterEntry describes one character in the character list.
type CharacterEntry struct {
	Name   string
	Race   string
	Class  string
	Gender int
	Level  int
	Zone   string
}

func registerGlueAPI(rt *Runtime) {
	L := rt.L
	reg := func(name string, fn func(L *lua.LState) int) {
		L.SetGlobal(name, L.NewFunction(fn))
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
		L.Push(lua.LString(buildVersion))
		L.Push(lua.LString(buildNumber))
		L.Push(lua.LString(buildDate))
		L.Push(lua.LNumber(buildTOC))
		return 4
	})
	reg("IsWindowsClient", func(L *lua.LState) int { L.Push(lua.LBool(runtime.GOOS == "windows")); return 1 })
	reg("IsMacClient", func(L *lua.LState) int { L.Push(lua.LBool(false)); return 1 })
	reg("IsLinuxClient", func(L *lua.LState) int { L.Push(lua.LBool(runtime.GOOS == "linux")); return 1 })
	reg("GetLocale", func(L *lua.LState) int { L.Push(lua.LString("enUS")); return 1 })
	reg("GetClientExpansionLevel", func(L *lua.LState) int { L.Push(lua.LNumber(2)); return 1 })
	reg("GetAccountExpansionLevel", func(L *lua.LState) int { L.Push(lua.LNumber(2)); return 1 })
	reg("IsShiftKeyDown", func(L *lua.LState) int { L.Push(lua.LBool(false)); return 1 })

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
		} else {
			L.Push(lua.LNil)
		}
		return 1
	})
	reg("GetCVarBool", func(L *lua.LState) int {
		if v, ok := rt.GetCVar(L.CheckString(1)); ok {
			L.Push(lua.LBool(v == "1" || strings.EqualFold(v, "true")))
		} else {
			L.Push(lua.LNil)
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
		rt.SetCVar(name, value)
		return 0
	})
	reg("GetCVarDefault", func(L *lua.LState) int {
		if v, ok := rt.cvarDefaults[strings.ToLower(L.CheckString(1))]; ok {
			L.Push(lua.LString(v))
		} else {
			L.Push(lua.LNil)
		}
		return 1
	})

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
		if idx < 1 || idx > len(rt.Glue.Realms) {
			L.Push(lua.LNil)
			return 1
		}
		r := rt.Glue.Realms[idx-1]
		L.Push(lua.LString(r.Name))
		L.Push(lua.LNumber(0)) // queue status: unavailable flag
		L.Push(lua.LBool(false))
		L.Push(lua.LNumber(0)) // current population
		L.Push(lua.LString(r.RealmType))
		return 5
	})
	reg("RequestRealmList", func(L *lua.LState) int {
		rt.Glue.PendingRealmList = true
		return 0
	})
	reg("CancelRealmListQuery", func(L *lua.LState) int { return 0 })
	reg("SortRealms", func(L *lua.LState) int { return 0 })
	reg("ChangeRealm", func(L *lua.LState) int { return 0 })

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
		L.Push(lua.LString(c.Race))
		L.Push(lua.LString(c.Class))
		L.Push(lua.LNumber(c.Gender))
		L.Push(lua.LNumber(c.Level))
		L.Push(lua.LNumber(0)) // localized class id placeholder is not used by glue scripts
		L.Push(lua.LNumber(0)) // zone id
		L.Push(lua.LString(c.Zone))
		return 8
	})
	reg("SelectCharacter", func(L *lua.LState) int {
		rt.Glue.SelectedCharacter = L.CheckInt(1)
		return 0
	})
	reg("GetCharacterSelectFacing", func(L *lua.LState) int { L.Push(lua.LNumber(0)); return 1 })
	reg("SetCharacterSelectFacing", func(L *lua.LState) int { return 0 })
	reg("GetCharacterCreateFacing", func(L *lua.LState) int { L.Push(lua.LNumber(0)); return 1 })
	reg("SetCharacterCreateFacing", func(L *lua.LState) int { return 0 })
	reg("RandomizeCharCustomization", func(L *lua.LState) int { return 0 })
	reg("CycleCharCustomization", func(L *lua.LState) int { return 0 })
	reg("UpdateCustomizationScene", func(L *lua.LState) int { return 0 })
	reg("UpdateSelectionCustomizationScene", func(L *lua.LState) int { return 0 })
	reg("SetCharCustomizeBackground", func(L *lua.LState) int { return 0 })
	reg("SetCharSelectBackground", func(L *lua.LState) int { return 0 })

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
	reg("GetNumAddOns", func(L *lua.LState) int { L.Push(lua.LNumber(0)); return 1 })
	reg("GetAddOnInfo", func(L *lua.LState) int {
		L.Push(lua.LNil)
		return 6
	})
	reg("GetAddOnEnableState", func(L *lua.LState) int { L.Push(lua.LNumber(0)); return 1 })
	reg("EnableAddOn", func(L *lua.LState) int { return 0 })
	reg("DisableAddOn", func(L *lua.LState) int { return 0 })
	reg("EnableAllAddOns", func(L *lua.LState) int { return 0 })
	reg("DisableAllAddOns", func(L *lua.LState) int { return 0 })
	reg("ResetAddOns", func(L *lua.LState) int { return 0 })
	reg("SaveAddOns", func(L *lua.LState) int { return 0 })
	reg("SetAddonVersionCheck", func(L *lua.LState) int { return 0 })
	reg("IsAddonVersionCheckEnabled", func(L *lua.LState) int { L.Push(lua.LBool(false)); return 1 })

	// Billing.
	reg("GetBillingPlan", func(L *lua.LState) int { L.Push(lua.LNumber(0)); return 1 })
	reg("GetBillingTimeRemaining", func(L *lua.LState) int { L.Push(lua.LNumber(0)); return 1 })
	reg("GetBillingTimeRested", func(L *lua.LState) int { L.Push(lua.LNumber(0)); return 1 })

	// Security matrix (unused with no authenticator traffic).
	reg("GetMatrixCoordinates", func(L *lua.LState) int { L.Push(lua.LNumber(0)); L.Push(lua.LNumber(0)); return 2 })
	reg("MatrixCommit", func(L *lua.LState) int { return 0 })
	reg("MatrixRevert", func(L *lua.LState) int { return 0 })

	// Frame creation for script-driven widgets (dropdown menus).
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
		w := newWidget(kindFromObjectType(frameType), name)
		w.parent = parent
		if parent != nil {
			parent.children = append(parent.children, w)
		}
		rt.register(w)
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
}
