package ui

import (
	"strings"

	lua "github.com/yuin/gopher-lua"
)

// ReputationChanged tells the chat and the combat text that the standing with
// a faction went up or down.
func (rt *Runtime) ReputationChanged(faction string, delta int) {
	key, fallback := "FACTION_STANDING_INCREASED", "Your reputation with %s has increased by %d."
	amount := delta
	if delta < 0 {
		key, fallback = "FACTION_STANDING_DECREASED", "Your reputation with %s has decreased by %d."
		amount = -delta
	}
	text := strings.Replace(rt.globalString(key, fallback), "%s", faction, 1)
	text = strings.Replace(text, "%d", itoa(amount), 1)
	rt.say("CHAT_MSG_COMBAT_FACTION_CHANGE", text)
	rt.FireEvent("COMBAT_TEXT_UPDATE", lua.LString("FACTION"), lua.LString(faction), lua.LNumber(delta))
}

// HonorGained tells the chat and the combat text of the honor a kill gave.
func (rt *Runtime) HonorGained(victim string, honor int) {
	text := rt.globalString("COMBAT_HONOR_GAIN_NO_RANK", "%s dies, honorable kill (Estimated Honor Points: %d)")
	text = strings.Replace(text, "%s", victim, 1)
	text = strings.Replace(text, "%d", itoa(honor), 1)
	rt.say("CHAT_MSG_COMBAT_HONOR_GAIN", text)
	rt.FireEvent("COMBAT_TEXT_UPDATE", lua.LString("HONOR_GAINED"), lua.LNumber(honor))
}
