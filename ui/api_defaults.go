package ui

import (
	"strings"

	lua "github.com/yuin/gopher-lua"
)

// registerWorldAPIDefaults defines the C-API functions FrameXML calls for game
// systems this client does not replicate yet (guild roster, talents, skills,
// reputation, mail, trade, battlegrounds, ...). Each returns what the client
// returns when the character has none of that: no guild, no talent tabs, empty
// lists, zero money. A function that already has a real implementation is left
// alone, so the real systems replace these one by one.
func registerWorldAPIDefaults(rt *Runtime) {
	L := rt.L
	define := func(name string, fn lua.LGFunction) {
		if L.GetGlobal(name) == lua.LNil {
			L.SetGlobal(name, L.NewFunction(fn))
			rt.stubAPI[name] = true
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
	nothing := func(names ...string) {
		for _, name := range names {
			returns(name)
		}
	}
	number := func(value float64) lua.LValue { return lua.LNumber(value) }
	text := func(value string) lua.LValue { return lua.LString(value) }

	// Guild and social lists: the character is in no guild and has no friends
	// beyond what GetNumFriends reports.
	returns("IsInGuild", lua.LNil)
	returns("GetGuildRosterMOTD", text(""))
	returns("GetGuildRosterSelection", number(0))
	returns("GetGuildRosterShowOffline", lua.LFalse)
	returns("GetNumGuildMembers", number(0), number(0))
	returns("GetGuildInfoText", text(""))
	returns("CanEditGuildInfo", lua.LFalse)
	returns("CanEditMOTD", lua.LFalse)
	returns("CanGuildInvite", lua.LFalse)
	returns("CanGuildPromote", lua.LFalse)
	returns("CanGuildDemote", lua.LFalse)
	returns("CanGuildRemove", lua.LFalse)
	returns("GetFriendInfo", lua.LNil)
	returns("GetIgnoreName", lua.LNil)
	returns("GetNumIgnores", number(0))
	returns("GetNumMutes", number(0))
	returns("GetNumWhoResults", number(0), number(0))
	returns("GetWhoInfo", lua.LNil)
	returns("GetNumGuildBankTabs", number(0))
	returns("GetGuildBankMoney", number(0))
	returns("GetGuildBankWithdrawMoney", number(0))
	nothing("SetGuildRosterSelection", "SetGuildRosterShowOffline", "SortGuildRoster", "GuildRoster", "SetGuildInfoText",
		"GuildDemote", "GuildPromote", "InviteUnit", "AddFriend", "AddIgnore", "AddMute", "SetWhoToUI", "SortWho",
		"GuildControlSetRankFlag", "PickupGuildBankMoney", "SetFriendNotes", "RemoveFriend", "DelIgnore", "ShowFriends", "SendWho")

	// Talents and skills.
	returns("GetNumTalentTabs", number(0))
	returns("GetNumTalents", number(0))
	returns("GetTalentTabInfo", lua.LNil)
	returns("GetTalentInfo", lua.LNil)
	returns("GetTalentPrereqs", lua.LNil)
	returns("GetUnspentTalentPoints", number(0))
	returns("GetGroupPreviewTalentPointsSpent", number(0))
	returns("GetActiveTalentGroup", number(1))
	returns("GetNumTalentGroups", number(1))
	returns("GetNumSkillLines", number(0))
	returns("GetSkillLineInfo", text(""), lua.LNil, lua.LNil, number(0), number(0), number(0), number(0), lua.LNil, number(0), number(0), number(0), number(0), text(""))
	returns("GetSelectedSkill", number(0))
	nothing("SetSelectedSkill", "ExpandSkillHeader", "CollapseSkillHeader", "AddSkillUp", "AcceptSkillUps", "CancelSkillUps", "RemoveSkillUp",
		"BuySkillTier", "SkillSortButton_OnClick", "SkillSortButton_OnShow")

	// Reputation.
	returns("GetNumFactions", number(0))
	returns("GetFactionInfo", lua.LNil)
	returns("GetSelectedFaction", number(0))
	returns("GetWatchedFactionInfo", lua.LNil)
	nothing("SetWatchedFactionIndex", "SetFactionActive", "SetFactionInactive", "FactionToggleAtWar", "ExpandFactionHeader", "CollapseFactionHeader")

	// Battlegrounds, arenas and the dungeon finder.
	returns("GetNumBattlegroundTypes", number(0))
	returns("GetBattlegroundInfo", lua.LNil)
	returns("GetBattlefieldStatus", text("none"))
	returns("GetWorldPVPQueueStatus", text("none"))
	returns("GetBattlefieldWinner", lua.LNil)
	returns("GetNumArenaOpponents", number(0))
	returns("GetExpansionLevel", number(2))
	nothing("CloseBattlefield", "RequestBattlefieldPositions", "SortBattlefieldScoreData", "RequestRaidInfo", "RequestLFDPartyLockInfo",
		"RequestLFDPlayerLockInfo", "LeaveLFG", "RefreshLFGList", "SearchLFGSort", "SetLFGComment", "AcceptProposal", "RejectProposal",
		"PartyLFGStartBackfill", "ConfirmReadyCheck")

	// Mail, trade, merchants and the tabard designer.
	returns("GetTargetTradeMoney", number(0))
	returns("GetSendMailMoney", number(0))
	returns("GetSendMailCOD", number(0))
	returns("GetSendMailPrice", number(30))
	returns("GetTabardCreationCost", number(0))
	returns("GetTabardInfo", lua.LNil)
	returns("GetNumBuybackItems", number(0))
	returns("GetRepairAllCost", number(0), lua.LFalse)
	returns("InRepairMode", lua.LFalse)
	returns("HasNewMail", lua.LFalse)
	returns("GetInboxNumItems", number(0), number(0))
	nothing("AddTradeMoney", "AddSendMailMoney", "AddSendMailCOD", "SetSendMailShowing", "PickupTradeMoney", "PickupSendMailMoney",
		"PickupSendMailCOD", "TakeInboxMoney", "TakeInboxTextItem", "CloseMail", "CloseTabardCreation", "ClosePetStables", "CloseGossip",
		"CloseGuildRegistrar", "ClosePetition", "ClosePetitionVendor", "CloseItemText", "CloseTaxiMap", "ClickTradeButton",
		"ClickTargetTradeButton", "ClickStablePet", "PickupStablePet", "PickupMerchantItem", "RepairAllItems", "ShowRepairCursor",
		"HideRepairCursor", "ShowMerchantSellCursor", "BuybackItem", "BuyGuildCharter", "BuyPetition", "AcceptTrade", "DropCursorMoney",
		"ItemTextNextPage", "ItemTextPrevPage", "TakeTaxiNode", "QuestLogPushQuest", "RollOnLoot")

	// Pets, companions, runes and totems.
	returns("GetCompanionInfo", lua.LNil)
	returns("GetNumCompanions", number(0))
	returns("GetTotemInfo", lua.LFalse, text(""), number(0), number(0), text(""))
	returns("GetMasterLootCandidate", lua.LNil)
	returns("IsPossessBarVisible", lua.LFalse)
	define("GetRuneType", func(L *lua.LState) int {
		// Runes 1-2 are blood, 3-4 unholy, 5-6 frost; type ids are 1 blood,
		// 2 unholy, 3 frost, 4 death.
		rune := L.CheckInt(1)
		L.Push(lua.LNumber((rune-1)/2 + 1))
		return 1
	})
	returns("GetRuneCooldown", number(0), number(10), lua.LTrue)
	returns("GetRuneCount", number(0))
	nothing("UpdateArenaEnemyBackground", "ArenaEnemyFrame_UpdatePet", "TriggerTutorial", "ResetTutorials")
	returns("CanResetTutorials", lua.LFalse)

	// Chat channels and voice.
	returns("GetSelectedDisplayChannel", number(0))
	returns("GetChannelList")
	returns("GetNumDisplayChannels", number(0))
	returns("GetChannelDisplayInfo", lua.LNil)
	returns("Sound_ChatSystem_GetNumInputDrivers", number(0))
	returns("Sound_ChatSystem_GetNumOutputDrivers", number(0))
	returns("Sound_ChatSystem_GetInputDriverNameByIndex", text(""))
	returns("Sound_ChatSystem_GetOutputDriverNameByIndex", text(""))
	nothing("RegisterForSave", "RegisterForSavePerCharacter", "SetActiveVoiceChannel", "VoiceChat_PlayLoopbackSound",
		"VoiceChat_RecordLoopbackSound", "VoiceChat_StopRecordingLoopbackSound", "SetLanguage")
	returns("GetLanguageByIndex", text(""))
	returns("GetNumLanguages", number(0))

	// Misc state queries.
	returns("UnitAffectingCombat", lua.LFalse)
	returns("IsMouseButtonDown", lua.LFalse)
	returns("GetMouseFocus", lua.LNil)
	returns("DetectWowMouse", lua.LFalse)
	returns("GetActionBarToggles", lua.LFalse, lua.LFalse, lua.LFalse, lua.LFalse)
	returns("CanUseEquipmentSets", lua.LFalse)
	returns("GetNumEquipmentSets", number(0))
	returns("GetEquipmentSetInfo", lua.LNil)
	returns("EquipmentSetContainsLockedItems", lua.LFalse)
	returns("ShowingHelm", lua.LTrue)
	returns("ShowingCloak", lua.LTrue)
	returns("GetSpellDescription", text(""))
	returns("GetQuestLogTitle", lua.LNil)
	returns("GetAbandonQuestName", text(""))
	returns("GetAbandonQuestItems", lua.LNil)
	returns("HasKey", lua.LFalse)
	nothing("ShowHelm", "ShowCloak", "SetCursor", "ResetCursor", "StopCinematic", "UseEquipmentSet", "PickupEquipmentSetByName",
		"SetMapZoom", "SendAddonMessage", "RunBinding", "ScriptErrorsFrame_OnError", "MovieRecording_Cancel", "MovieRecording_DeleteMovie",
		"MovieRecording_QueueMovieToCompress", "FrameXML_Debug")
	returns("MovieRecording_IsSupported", lua.LFalse)
	returns("MovieRecording_IsRecording", lua.LFalse)
	returns("MovieRecording_SearchUncompressedMovie", lua.LNil)
	returns("MovieRecording_GetMovieFullPath", text(""))
}

// registerSurveyedAPIDefaults defines the remaining surveyed C-API functions:
// counts report zero, state queries report false and everything else (actions
// and lookups with nothing to find) returns nothing.
func registerSurveyedAPIDefaults(rt *Runtime) {
	L := rt.L
	define := func(name string, fn lua.LGFunction) {
		if L.GetGlobal(name) == lua.LNil {
			L.SetGlobal(name, L.NewFunction(fn))
			rt.stubAPI[name] = true
		}
	}
	for _, name := range emptyCountAPI {
		define(name, func(L *lua.LState) int { L.Push(lua.LNumber(0)); return 1 })
	}
	for _, name := range emptyFlagAPI {
		define(name, func(L *lua.LState) int { L.Push(lua.LFalse); return 1 })
	}
	for _, name := range emptyActionAPI {
		if (strings.HasPrefix(name, "Get") && hasNumericSuffix(name)) || strings.HasPrefix(name, "BNGetNum") {
			define(name, func(L *lua.LState) int { L.Push(lua.LNumber(0)); return 1 })
			continue
		}
		define(name, func(L *lua.LState) int { return 0 })
	}
	// Movement and camera commands the bindings call; the world camera controller
	// reads the movement keys itself until movement moves behind these.
	for _, name := range []string{"AscendStop", "AttackTarget", "CameraOrSelectOrMoveStart", "CameraOrSelectOrMoveStop", "CameraZoomIn", "CameraZoomOut",
		"DescendStop", "FlipCameraYaw", "InteractUnit", "JumpOrAscendStart", "MoveAndSteerStart", "MoveAndSteerStop", "MoveBackwardStart", "MoveBackwardStop",
		"MoveForwardStart", "MoveForwardStop", "NextView", "PitchDownStart", "PitchDownStop", "PitchUpStart", "PitchUpStop", "PrevView", "ResetView", "SaveView", "SetView",
		"SitStandOrDescendStart", "StrafeLeftStart", "StrafeLeftStop", "StrafeRightStart", "StrafeRightStop", "ToggleAutoRun", "ToggleRun", "ToggleSheath",
		"TurnLeftStart", "TurnLeftStop", "TurnOrActionStart", "TurnOrActionStop", "TurnRightStart", "TurnRightStop", "VehicleAimDecrement", "VehicleAimDownStart",
		"VehicleAimDownStop", "VehicleAimIncrement", "VehicleAimUpStart", "VehicleAimUpStop", "VehicleCameraZoomIn", "VehicleCameraZoomOut", "VehicleNextSeat",
		"VehiclePrevSeat", "ToggleCollision", "ToggleCollisionDisplay", "ToggleCommentatorMode", "TogglePerformanceDisplay", "TogglePerformancePause",
		"TogglePerformanceValues", "TogglePlayerBounds", "TogglePortals", "ToggleTris", "ResetPerformanceValues", "MovieRecording_Toggle", "MovieRecording_ToggleGUI",
		"MusicPlayer_BackTrack", "MusicPlayer_NextTrack", "MusicPlayer_PlayPause", "MusicPlayer_VolumeDown", "MusicPlayer_VolumeUp"} {
		define(name, func(L *lua.LState) int { return 0 })
	}
	define("MovieRecording_IsCompressing", func(L *lua.LState) int { L.Push(lua.LFalse); return 1 })
	zeros := func(count int) lua.LGFunction {
		return func(L *lua.LState) int {
			for range count {
				L.Push(lua.LNumber(0))
			}
			return count
		}
	}
	// Character sheet statistics: the values the PaperDoll frame computes its
	// panels from. Until the server's PLAYER_FIELD values are mapped they are 0.
	for name, count := range map[string]int{
		"GetCombatRating": 1, "GetCombatRatingBonus": 1, "GetMaxCombatRatingBonus": 1, "GetCritChance": 1,
		"GetCritChanceFromAgility": 1, "GetRangedCritChance": 1, "GetSpellCritChance": 1, "GetSpellCritChanceFromIntellect": 1,
		"GetBlockChance": 1, "GetDodgeChance": 1, "GetParryChance": 1, "GetShieldBlock": 1, "GetSpellBonusDamage": 1,
		"GetSpellBonusHealing": 1, "GetArmorPenetration": 1, "GetSpellPenetration": 1, "GetAttackPowerForStat": 1,
		"GetUnitHealthModifier": 1, "GetUnitMaxHealthModifier": 1, "GetUnitPowerModifier": 1,
		"GetUnitHealthRegenRateFromSpirit": 1, "GetUnitManaRegenRateFromSpirit": 1, "GetPetSpellBonusDamage": 1,
		"UnitStat": 4, "UnitArmor": 4, "UnitAttackPower": 3, "UnitRangedAttackPower": 3, "UnitDefense": 2,
		"UnitAttackSpeed": 2, "UnitRangedAttack": 4, "UnitAttackBothHands": 4, "UnitResistance": 4,
		"GetExpertise": 2, "GetManaRegen": 2, "GetExpertisePercent": 2, "GetAdjustedSkillPoints": 1,
		"GetHonorCurrency": 2, "GetArenaCurrency": 1, "GetPVPTimer": 1, "GetLFGDeserterExpiration": 0,
		"GetLFGRandomCooldownExpiration": 0, "GetTimeToWellRested": 0, "GetPossessInfo": 0,
	} {
		define(name, zeros(count))
	}
	define("UnitDamage", func(L *lua.LState) int {
		for _, value := range []float64{0, 0, 0, 0, 0, 0, 1} {
			L.Push(lua.LNumber(value))
		}
		return 7
	})
	define("UnitRangedDamage", func(L *lua.LState) int {
		for _, value := range []float64{0, 0, 0, 0, 1} {
			L.Push(lua.LNumber(value))
		}
		return 5
	})
	define("HasFullControl", func(L *lua.LState) int { L.Push(lua.LTrue); return 1 })
	define("UnitReaction", func(L *lua.LState) int { L.Push(lua.LNumber(4)); return 1 })
	define("GetRestState", func(L *lua.LState) int { L.Push(lua.LNumber(1)); return 1 })
	define("GetFramerate", func(L *lua.LState) int { L.Push(lua.LNumber(60)); return 1 })
	define("GetBindingAction", func(L *lua.LState) int { L.Push(lua.LString("")); return 1 })
	define("GetBindingByKey", func(L *lua.LState) int { L.Push(lua.LString("")); return 1 })
	define("GetRealmName", func(L *lua.LState) int {
		L.Push(L.GetGlobal("GetServerName"))
		L.Call(0, 1)
		if L.Get(-1) == lua.LNil {
			L.Pop(1)
			L.Push(lua.LString(""))
		}
		return 1
	})
	define("GetRealZoneText", func(L *lua.LState) int {
		L.Push(L.GetGlobal("GetZoneText"))
		L.Call(0, 1)
		return 1
	})
}

// registerLuaExtensions adds the table helpers the client defines on top of
// the standard library.
func registerLuaExtensions(rt *Runtime) {
	L := rt.L
	wipe := L.NewFunction(func(L *lua.LState) int {
		table := L.CheckTable(1)
		var keys []lua.LValue
		table.ForEach(func(key, _ lua.LValue) { keys = append(keys, key) })
		for _, key := range keys {
			table.RawSet(key, lua.LNil)
		}
		L.Push(table)
		return 1
	})
	L.SetGlobal("wipe", wipe)
	if tableLib, ok := L.GetGlobal("table").(*lua.LTable); ok {
		tableLib.RawSetString("wipe", wipe)
	}
}

// registerWorldMapDefaults defines the map functions for the parts of the world
// map this client does not draw yet: landmarks, battlefield markers and the
// player arrow model.
func registerWorldMapDefaults(rt *Runtime) {
	L := rt.L
	define := func(name string, fn lua.LGFunction) {
		if L.GetGlobal(name) == lua.LNil {
			L.SetGlobal(name, L.NewFunction(fn))
			rt.stubAPI[name] = true
		}
	}
	for _, name := range []string{"CreateWorldMapArrowFrame", "ShowWorldMapArrowFrame", "PositionWorldMapArrowFrame", "UpdateWorldMapArrowFrames",
		"InitWorldMapPing", "ClickLandmark", "TeleportToDebugObject", "QuestMapUpdateAllQuests", "QuestPOIUpdateIcons",
		"GetMapDebugObjectInfo"} {
		define(name, func(L *lua.LState) int { return 0 })
	}
	for _, name := range []string{"HasDebugZoneMap"} {
		define(name, func(L *lua.LState) int { L.Push(lua.LFalse); return 1 })
	}
	for _, name := range []string{"GetNumMapDebugObjects", "GetNumBattlefieldVehicles", "GetNumBattlefieldPositions", "GetNumBattlefieldFlagPositions",
		"GetNumQuestWatches", "GetNumQuestLeaderBoards", "GetNumQuestLogEntries", "GetWintergraspWaitTime"} {
		define(name, func(L *lua.LState) int { L.Push(lua.LNumber(0)); return 1 })
	}
	define("GetBattlefieldPosition", func(L *lua.LState) int { L.Push(lua.LNumber(0)); L.Push(lua.LNumber(0)); L.Push(lua.LString("")); return 3 })
	define("GetBattlefieldFlagPosition", func(L *lua.LState) int { L.Push(lua.LNumber(0)); L.Push(lua.LNumber(0)); L.Push(lua.LString("")); return 3 })
	define("GetBattlefieldVehicleInfo", func(L *lua.LState) int { L.Push(lua.LNumber(0)); L.Push(lua.LNumber(0)); L.Push(lua.LString("")); L.Push(lua.LFalse); L.Push(lua.LString("")); L.Push(lua.LNumber(0)); return 6 })
	define("GetQuestWorldMapAreaID", func(L *lua.LState) int { L.Push(lua.LNumber(0)); return 1 })
}

// hasNumericSuffix reports names of query functions that answer with a
// quantity, which scripts do arithmetic on.
func hasNumericSuffix(name string) bool {
	for _, suffix := range []string{"Size", "Count", "Cost", "Money", "Delay", "Rating", "Chance", "Points", "Level", "Remaining", "TimeLeft"} {
		if strings.HasSuffix(name, suffix) {
			return true
		}
	}
	return false
}

// registerAddOnAPIDefaults defines the add-on C-API functions that have no
// implementation yet, the same way registerSurveyedAPIDefaults does for
// FrameXML.
func registerAddOnAPIDefaults(rt *Runtime) {
	L := rt.L
	define := func(name string, fn lua.LGFunction) {
		if L.GetGlobal(name) == lua.LNil {
			L.SetGlobal(name, L.NewFunction(fn))
			rt.stubAPI[name] = true
		}
	}
	for _, name := range emptyAddonCountAPI {
		define(name, func(L *lua.LState) int { L.Push(lua.LNumber(0)); return 1 })
	}
	for _, name := range emptyAddonFlagAPI {
		define(name, func(L *lua.LState) int { L.Push(lua.LFalse); return 1 })
	}
	for _, name := range emptyAddonActionAPI {
		if strings.HasPrefix(name, "Get") && hasNumericSuffix(name) {
			define(name, func(L *lua.LState) int { L.Push(lua.LNumber(0)); return 1 })
			continue
		}
		define(name, func(L *lua.LState) int { return 0 })
	}
	define("GetBackpackCurrencyInfo", func(L *lua.LState) int { return 0 })
	// The small map's player arrow is made in code too: frames the battlefield
	// minimap sizes and fades, with nothing to draw.
	define("CreateMiniWorldMapArrowFrame", func(L *lua.LState) int {
		parent := rt.widgetArg(L, 1)
		for _, name := range []string{"PlayerMiniArrowFrame", "PlayerMiniArrowEffectFrame"} {
			if rt.widgets[name] != nil {
				continue
			}
			frame := newWidget(kindFrame, name)
			frame.typeName = "Model"
			frame.parent = parent
			frame.shown = false
			frame.width, frame.height = 32, 32
			if parent != nil {
				frame.frameStrata = parent.frameStrata
				frame.frameLevel = parent.frameLevel + 1
				addWidgetChild(parent, frame)
			}
			rt.register(frame)
		}
		return 0
	})
	for _, name := range []string{"PositionMiniWorldMapArrowFrame", "ShowMiniWorldMapArrowFrame"} {
		define(name, func(L *lua.LState) int { return 0 })
	}
	// What the calendar filters guild members by until the player chooses.
	L.SetGlobal("CalendarDefaultGuildFilter", L.NewFunction(func(L *lua.LState) int {
		L.Push(lua.LNumber(1))
		L.Push(lua.LNumber(80))
		L.Push(lua.LNumber(1))
		return 3
	}))
	define("UnitInRange", func(L *lua.LState) int { L.Push(lua.LFalse); L.Push(lua.LFalse); return 2 })
}
