package ui

// Bits of the flags of a player that raise an event when they change.
const (
	playerFlagRested       = 0x20
	playerFlagPlayTime     = 0x3000
	playerFlagTaxiBench    = 0x20000
	playerFlagNoXP         = 0x2000000
	playerFlagLowLevelRaid = 0x10000
)

// playerFlagEvents raises the events a change in the player's flags deserves.
func (rt *Runtime) playerFlagEvents(old, now uint32, known bool) {
	if !known {
		return
	}
	changed := old ^ now
	if changed&playerFlagRested != 0 {
		rt.FireEvent("PLAYER_UPDATE_RESTING")
	}
	if changed&playerFlagPlayTime != 0 {
		rt.FireEvent("PLAYTIME_CHANGED")
	}
	if changed&playerFlagTaxiBench != 0 {
		if now&playerFlagTaxiBench != 0 {
			rt.FireEvent("ENABLE_TAXI_BENCHMARK")
		} else {
			rt.FireEvent("DISABLE_TAXI_BENCHMARK")
		}
	}
	if changed&playerFlagLowLevelRaid != 0 {
		if now&playerFlagLowLevelRaid != 0 {
			rt.FireEvent("ENABLE_LOW_LEVEL_RAID")
		} else {
			rt.FireEvent("DISABLE_LOW_LEVEL_RAID")
		}
	}
	if changed&playerFlagNoXP != 0 {
		if now&playerFlagNoXP != 0 {
			rt.FireEvent("DISABLE_XP_GAIN")
		} else {
			rt.FireEvent("ENABLE_XP_GAIN")
		}
	}
}
