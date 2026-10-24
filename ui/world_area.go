package ui

// areaState is the place the player stands in, as the world reports it.
type areaState struct {
	known         bool
	zone, subZone string
	// pvp is what GetZonePVPInfo answers: "sanctuary", "friendly", "contested",
	// "hostile" or "arena", empty for none.
	pvp string
	// faction is the name of the side that holds the area.
	faction string
	// indoors is set while the place is inside a map object.
	indoors bool
}

// SetArea records the zone and sub zone the player is in and tells the
// interface when they change.
func (rt *Runtime) SetArea(zone, subZone, pvp, faction string, indoors bool) {
	if rt == nil {
		return
	}
	previous := rt.area
	rt.area = areaState{known: true, zone: zone, subZone: subZone, pvp: pvp, faction: faction, indoors: indoors}
	switch {
	case !previous.known || previous.zone != zone:
		rt.FireEvent("ZONE_CHANGED_NEW_AREA")
		rt.FireEvent("ZONE_CHANGED")
	case previous.subZone != subZone && indoors:
		rt.FireEvent("ZONE_CHANGED_INDOORS")
	case previous.subZone != subZone:
		rt.FireEvent("ZONE_CHANGED")
	case previous.pvp != pvp:
		rt.FireEvent("ZONE_CHANGED")
	}
}
