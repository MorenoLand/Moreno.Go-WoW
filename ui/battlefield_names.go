package ui

// bfCurrentMap is the map the character stands in.
func (rt *Runtime) bfCurrentMap() uint32 {
	if rt.worldMap.hasPlayer {
		return rt.worldMap.playerMap
	}
	return 0
}

// localizedRaceName is the name of a race for a sex.
func (rt *Runtime) localizedRaceName(race, gender uint8) string {
	for _, row := range rt.who.tables.Races {
		if row.ID == uint32(race) {
			switch {
			case gender == 1 && row.Female != "":
				return row.Female
			case gender == 0 && row.Male != "":
				return row.Male
			}
			return row.Name
		}
	}
	return ""
}

// localizedClassName is the name of a class for a sex and the token the
// interface keys its colours by.
func (rt *Runtime) localizedClassName(class, gender uint8) (string, string) {
	for _, row := range rt.who.tables.Classes {
		if row.ID == uint32(class) {
			name := row.Name
			switch {
			case gender == 1 && row.Female != "":
				name = row.Female
			case gender == 0 && row.Male != "":
				name = row.Male
			}
			return name, row.File
		}
	}
	return "", ""
}
