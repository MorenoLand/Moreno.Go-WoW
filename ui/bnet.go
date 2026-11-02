package ui

import lua "github.com/yuin/gopher-lua"

// Battle.net. The client only talks to Battle.net for an account that logged
// in through it; Wow.exe checks that first in every one of these calls and
// without it answers a count with zero and everything else with nothing, as
// this client does for a realm account.
func registerBattleNetAPI(rt *Runtime) {
	L := rt.L
	for _, name := range []string{
		"BNGetNumBlocked",
		"BNGetNumBlockedToons",
		"BNGetNumConversationMembers",
		"BNGetNumFriendInvites",
		"BNGetNumFriendToons",
		"BNGetSelectedBlock",
		"BNGetSelectedFriend",
		"BNGetSelectedToonBlock",
	} {
		L.SetGlobal(name, L.NewFunction(func(L *lua.LState) int {
			L.Push(lua.LNumber(0))
			return 1
		}))
	}
	for _, name := range []string{
		"BNAcceptFriendInvite",
		"BNCreateConversation",
		"BNDeclineFriendInvite",
		"BNGetBlockedInfo",
		"BNGetBlockedToonInfo",
		"BNGetConversationInfo",
		"BNGetConversationMemberInfo",
		"BNGetCustomMessageTable",
		"BNGetFOFInfo",
		"BNGetFriendInfo",
		"BNGetFriendInfoByID",
		"BNGetFriendInviteInfo",
		"BNGetFriendToonInfo",
		"BNGetInfo",
		"BNGetMatureLanguageFilter",
		"BNGetNumFOF",
		"BNGetToonInfo",
		"BNInviteToConversation",
		"BNIsFriend",
		"BNIsSelf",
		"BNIsToonBlocked",
		"BNLeaveConversation",
		"BNListConversation",
		"BNRemoveFriend",
		"BNReportPlayer",
		"BNRequestFOFInfo",
		"BNSendConversationMessage",
		"BNSendFriendInvite",
		"BNSendFriendInviteByID",
		"BNSendWhisper",
		"BNSetAFK",
		"BNSetBlocked",
		"BNSetCustomMessage",
		"BNSetDND",
		"BNSetFriendNote",
		"BNSetMatureLanguageFilter",
		"BNSetSelectedBlock",
		"BNSetSelectedFriend",
		"BNSetSelectedToonBlock",
		"BNSetToonBlocked",
	} {
		L.SetGlobal(name, L.NewFunction(func(L *lua.LState) int { return 0 }))
	}
}
