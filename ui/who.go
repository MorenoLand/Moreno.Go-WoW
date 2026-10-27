package ui

import (
	"encoding/binary"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"unicode"

	"github.com/MorenoLand/Moreno.WoW/world"
	lua "github.com/yuin/gopher-lua"
)

// Who packets.
const (
	clientWho  world.ClientOpcode = 0x0062
	packetWho  uint16             = 0x0063
	whoMaxRows                    = 50
	// whoMaxWords is how many free words a query carries.
	whoMaxWords = 4
	// whoMaxZones is how many zones a query can name.
	whoMaxZones = 10
	// whoChatLimit is how many results are written to the chat instead of
	// shown in the who list.
	whoChatLimit = 3
)

// WhoZone is a zone of AreaTable.dbc (a row with no parent).
type WhoZone struct {
	ID   uint32
	Name string
}

// WhoRace is a row of ChrRaces.dbc with its names for each sex.
type WhoRace struct {
	ID                 uint32
	Name, Female, Male string
	// SicknessSpell is the spell that weakens the race after it comes back to life.
	SicknessSpell uint32
}

// WhoClass is a row of ChrClasses.dbc with its names for each sex.
type WhoClass struct {
	ID                 uint32
	Name, Female, Male string
	File               string
}

// WhoTables are the tables a who query and its results need.
type WhoTables struct {
	Zones   []WhoZone
	Races   []WhoRace
	Classes []WhoClass
	// Areas names every area, for the zone a result stands in.
	Areas map[uint32]string
}

// whoEntry is a player a who query found.
type whoEntry struct {
	name, guild              string
	level, class, race, area uint32
	female                   bool
}

// whoSortKey is the column a who list is ordered by.
type whoSortKey int

const (
	whoByZone whoSortKey = iota
	whoByLevel
	whoByClass
	whoByGroup
	whoByName
	whoByRace
	whoByGuild
)

type whoSort struct {
	key       whoSortKey
	ascending bool
}

type whoState struct {
	tables  WhoTables
	rows    []whoEntry
	total   int
	toUI    bool
	sorting [7]whoSort
}

// SetWhoTables gives the interface the tables who queries use.
func (rt *Runtime) SetWhoTables(tables WhoTables) {
	rt.who.tables = tables
	if rt.who.sorting[0].key == 0 && !rt.who.sorting[0].ascending && rt.who.sorting[1].key == 0 {
		rt.resetWhoSorting()
	}
}

func (rt *Runtime) resetWhoSorting() {
	order := []whoSortKey{whoByName, whoByZone, whoByLevel, whoByClass, whoByGroup, whoByRace, whoByGuild}
	for index, key := range order {
		rt.who.sorting[index] = whoSort{key: key, ascending: true}
	}
}

// whoQuery is a parsed filter.
type whoQuery struct {
	minLevel, maxLevel uint32
	name, guild        string
	races, classes     uint32
	zones              []uint32
	words              []string
}

// containsFold reports whether a name holds a text, ignoring case.
func containsFold(name, text string) bool {
	return strings.Contains(strings.ToLower(name), strings.ToLower(text))
}

// parseWho reads a filter: words, "quoted words", numbers for levels and the
// tags n- g- z- r- c-.
func (rt *Runtime) parseWho(filter string) whoQuery {
	query := whoQuery{maxLevel: 100, races: 0xFFFFFFFF, classes: 0xFFFFFFFF}
	tag := func(name, fallback string) string { return strings.ToLower(rt.globalString(name, fallback)) }
	tags := struct{ name, guild, zone, race, class string }{
		tag("WHO_TAG_NAME", "n-"), tag("WHO_TAG_GUILD", "g-"), tag("WHO_TAG_ZONE", "z-"), tag("WHO_TAG_RACE", "r-"), tag("WHO_TAG_CLASS", "c-"),
	}
	var tokens []string
	var current strings.Builder
	quoted := false
	flush := func() {
		if current.Len() > 0 {
			tokens = append(tokens, current.String())
			current.Reset()
		}
	}
	for _, char := range filter {
		switch {
		case char == '"':
			quoted = !quoted
		case !quoted && unicode.IsSpace(char):
			flush()
		default:
			current.WriteRune(char)
		}
	}
	flush()
	zonesMatched := false
	for _, token := range tokens {
		lower := strings.ToLower(token)
		switch {
		case strings.HasPrefix(lower, tags.name):
			query.name = token[len(tags.name):]
		case strings.HasPrefix(lower, tags.guild):
			query.guild = token[len(tags.guild):]
		case strings.HasPrefix(lower, tags.zone):
			text := token[len(tags.zone):]
			zonesMatched = true
			for _, zone := range rt.who.tables.Zones {
				if len(query.zones) < whoMaxZones && containsFold(zone.Name, text) {
					query.zones = append(query.zones, zone.ID)
				}
			}
			if len(query.zones) == 0 {
				query.zones = []uint32{0}
			}
		case strings.HasPrefix(lower, tags.race):
			text := token[len(tags.race):]
			if query.races == 0xFFFFFFFF {
				query.races = 0
			}
			for _, race := range rt.who.tables.Races {
				if containsFold(race.Name, text) && race.ID < 32 {
					query.races |= 1 << race.ID
				}
			}
		case strings.HasPrefix(lower, tags.class):
			text := token[len(tags.class):]
			if query.classes == 0xFFFFFFFF {
				query.classes = 0
			}
			for _, class := range rt.who.tables.Classes {
				if containsFold(class.Name, text) && class.ID < 32 {
					query.classes |= 1 << class.ID
				}
			}
		case isLevelToken(token):
			low, high := parseLevelRange(token)
			query.minLevel, query.maxLevel = low, high
		default:
			if len(query.words) < whoMaxWords {
				query.words = append(query.words, token)
			}
		}
	}
	_ = zonesMatched
	return query
}

func isLevelToken(token string) bool {
	if token == "" {
		return false
	}
	if token[0] >= '0' && token[0] <= '9' {
		return true
	}
	return token[0] == '-' && len(token) > 1 && token[1] >= '0' && token[1] <= '9'
}

// parseLevelRange reads "20", "20-30" and "-30".
func parseLevelRange(token string) (uint32, uint32) {
	low, high := uint32(0), uint32(100)
	digits := func(text string) (uint32, string) {
		end := 0
		for end < len(text) && text[end] >= '0' && text[end] <= '9' {
			end++
		}
		value, _ := strconv.Atoi(text[:end])
		return uint32(value), text[end:]
	}
	rest := token
	if token[0] != '-' {
		low, rest = digits(token)
		high = low
	}
	if strings.HasPrefix(rest, "-") {
		high = 100
		if value, _ := digits(rest[1:]); len(rest) > 1 && rest[1] >= '0' && rest[1] <= '9' {
			high = value
		}
	}
	return low, high
}

// buildWhoRequest is the body of CMSG_WHO.
func buildWhoRequest(query whoQuery) []byte {
	body := binary.LittleEndian.AppendUint32(nil, query.minLevel)
	body = binary.LittleEndian.AppendUint32(body, query.maxLevel)
	body = append(append(body, query.name...), 0)
	body = append(append(body, query.guild...), 0)
	body = binary.LittleEndian.AppendUint32(body, query.races)
	body = binary.LittleEndian.AppendUint32(body, query.classes)
	body = binary.LittleEndian.AppendUint32(body, uint32(len(query.zones)))
	for _, zone := range query.zones {
		body = binary.LittleEndian.AppendUint32(body, zone)
	}
	body = binary.LittleEndian.AppendUint32(body, uint32(len(query.words)))
	for _, word := range query.words {
		body = append(append(body, word...), 0)
	}
	return body
}

func (rt *Runtime) raceName(id uint32, female bool) string {
	for _, race := range rt.who.tables.Races {
		if race.ID == id {
			if female && race.Female != "" {
				return race.Female
			}
			if !female && race.Male != "" {
				return race.Male
			}
			return race.Name
		}
	}
	return rt.globalString("UNKNOWN", "Unknown")
}

func (rt *Runtime) className(id uint32, female bool) string {
	for _, class := range rt.who.tables.Classes {
		if class.ID == id {
			if female && class.Female != "" {
				return class.Female
			}
			if !female && class.Male != "" {
				return class.Male
			}
			return class.Name
		}
	}
	return rt.globalString("UNKNOWN", "Unknown")
}

func (rt *Runtime) classFile(id uint32) string {
	for _, class := range rt.who.tables.Classes {
		if class.ID == id {
			return class.File
		}
	}
	return ""
}

func (rt *Runtime) whoArea(id uint32) string {
	if name := rt.who.tables.Areas[id]; name != "" {
		return name
	}
	return rt.globalString("UNKNOWN", "Unknown")
}

// applyWho handles SMSG_WHO.
func (rt *Runtime) applyWho(body []byte) {
	reader := world.NewReader(body, "SMSG_WHO")
	shown, err := reader.U32()
	if err != nil {
		return
	}
	total, err := reader.U32()
	if err != nil {
		return
	}
	rt.who.rows = rt.who.rows[:0]
	for index := uint32(0); index < shown; index++ {
		var entry whoEntry
		if entry.name, err = reader.CString(); err != nil {
			break
		}
		if entry.guild, err = reader.CString(); err != nil {
			break
		}
		if entry.level, err = reader.U32(); err != nil {
			break
		}
		if entry.class, err = reader.U32(); err != nil {
			break
		}
		if entry.race, err = reader.U32(); err != nil {
			break
		}
		gender, gerr := reader.U8()
		if gerr != nil {
			break
		}
		entry.female = gender == 1
		if entry.area, err = reader.U32(); err != nil {
			break
		}
		if len(rt.who.rows) < whoMaxRows {
			rt.who.rows = append(rt.who.rows, entry)
		}
	}
	rt.who.total = int(total)
	rt.sortWho()
	toChat := !rt.who.toUI && (int(shown) <= whoChatLimit)
	if !toChat {
		rt.FireEvent("WHO_LIST_UPDATE")
		return
	}
	for _, entry := range rt.who.rows {
		rt.whoLine(entry)
	}
	rt.ChatLine("CHAT_MSG_SYSTEM", fmt.Sprintf(rt.globalString("WHO_NUM_RESULTS", "%d players total"), total))
}

// whoLine writes one result to the chat windows.
func (rt *Runtime) whoLine(entry whoEntry) {
	race, class, zone := rt.raceName(entry.race, entry.female), rt.className(entry.class, entry.female), rt.whoArea(entry.area)
	var line string
	if entry.guild == "" {
		line = fmt.Sprintf(rt.globalString("WHO_LIST_FORMAT", "%s %s %d %s %s %s"), entry.name, entry.name, entry.level, race, class, zone)
	} else {
		line = fmt.Sprintf(rt.globalString("WHO_LIST_GUILD_FORMAT", "%s %s %d %s %s %s %s"), entry.name, entry.name, entry.level, race, class, entry.guild, zone)
	}
	rt.ChatLine("CHAT_MSG_SYSTEM", line)
}

// sortWho orders the results by the columns in turn.
func (rt *Runtime) sortWho() {
	compare := func(a, b whoEntry, key whoSortKey) int {
		switch key {
		case whoByZone:
			return strings.Compare(rt.whoArea(a.area), rt.whoArea(b.area))
		case whoByLevel:
			return int(a.level) - int(b.level)
		case whoByClass:
			return strings.Compare(rt.className(a.class, a.female), rt.className(b.class, b.female))
		case whoByGroup:
			return 0
		case whoByName:
			return strings.Compare(strings.ToLower(a.name), strings.ToLower(b.name))
		case whoByRace:
			return strings.Compare(rt.raceName(a.race, a.female), rt.raceName(b.race, b.female))
		case whoByGuild:
			return strings.Compare(a.guild, b.guild)
		}
		return 0
	}
	sort.SliceStable(rt.who.rows, func(i, j int) bool {
		for _, order := range rt.who.sorting {
			result := compare(rt.who.rows[i], rt.who.rows[j], order.key)
			if result == 0 {
				continue
			}
			if order.ascending {
				return result < 0
			}
			return result > 0
		}
		return false
	})
}

// registerWhoAPI defines the calls of the who list.
func registerWhoAPI(rt *Runtime) {
	L := rt.L
	reg := func(name string, fn lua.LGFunction) { L.SetGlobal(name, L.NewFunction(fn)) }
	reg("SendWho", func(L *lua.LState) int {
		rt.send(clientWho, buildWhoRequest(rt.parseWho(L.CheckString(1))))
		return 0
	})
	reg("GetNumWhoResults", func(L *lua.LState) int {
		L.Push(lua.LNumber(len(rt.who.rows)))
		L.Push(lua.LNumber(rt.who.total))
		return 2
	})
	reg("GetWhoInfo", func(L *lua.LState) int {
		index := L.CheckInt(1)
		if index < 1 || index > len(rt.who.rows) {
			L.Push(lua.LNil)
			L.Push(lua.LNil)
			L.Push(lua.LNumber(0))
			for range 4 {
				L.Push(lua.LNil)
			}
			return 7
		}
		entry := rt.who.rows[index-1]
		L.Push(lua.LString(entry.name))
		L.Push(lua.LString(entry.guild))
		L.Push(lua.LNumber(entry.level))
		L.Push(lua.LString(rt.raceName(entry.race, entry.female)))
		L.Push(lua.LString(rt.className(entry.class, entry.female)))
		L.Push(lua.LString(rt.whoArea(entry.area)))
		if file := rt.classFile(entry.class); file != "" {
			L.Push(lua.LString(file))
		} else {
			L.Push(lua.LNil)
		}
		return 7
	})
	reg("SetWhoToUI", func(L *lua.LState) int {
		rt.who.toUI = L.GetTop() >= 1 && lua.LVAsBool(L.Get(1))
		return 0
	})
	reg("SortWho", func(L *lua.LState) int {
		name := strings.ToLower(L.CheckString(1))
		key := whoByName
		switch name {
		case "level":
			key = whoByLevel
		case "class":
			key = whoByClass
		case "group":
			key = whoByGroup
		case "race":
			key = whoByRace
		case "zone":
			key = whoByZone
		case "guild":
			key = whoByGuild
		}
		for position := range rt.who.sorting {
			if rt.who.sorting[position].key != key {
				continue
			}
			chosen := rt.who.sorting[position]
			if position == 0 {
				chosen.ascending = !chosen.ascending
			} else {
				copy(rt.who.sorting[1:position+1], rt.who.sorting[:position])
			}
			rt.who.sorting[0] = chosen
			break
		}
		rt.sortWho()
		rt.FireEvent("WHO_LIST_UPDATE")
		return 0
	})
}
