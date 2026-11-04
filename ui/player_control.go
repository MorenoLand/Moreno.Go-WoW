package ui

// SetPlayerControl says whether the player can move the character, which the
// server changes when the character is stunned, charmed or on a vehicle. The
// interface hears of it only when it changes.
func (rt *Runtime) SetPlayerControl(allowed bool) {
	if allowed == rt.hasControl {
		return
	}
	rt.hasControl = allowed
	if allowed {
		rt.FireEvent("PLAYER_CONTROL_GAINED")
		return
	}
	rt.FireEvent("PLAYER_CONTROL_LOST")
}
