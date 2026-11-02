package ui

import (
	"sort"
	"strings"

	"github.com/MorenoLand/Moreno.WoW/world"
	lua "github.com/yuin/gopher-lua"
)

// Dungeon finder dungeon types of LFGDungeons.dbc.
const (
	lfgTypeDungeon   = 1
	lfgTypeRaid      = 2
	lfgTypeZone      = 4
	lfgTypeHeroic    = 5
	lfgTypeRandom    = 6
	lfgFlagFinder    = 2 // listed by the dungeon finder
	lfgFlagHoliday   = 4
	lfgRoleLeader    = 1
	lfgRoleTank      = 2
	lfgRoleHealer    = 4
	lfgRoleDamage    = 8
	lfgSpellDeserter = 71041
	lfgSpellCooldown = 71328
)

// lfgClassRoles are the roles each class can take (indexed by class id).
var lfgClassRoles = [...]uint8{0, 0xB, 0xF, 9, 9, 0xD, 0xB, 0xD, 9, 9, 0, 0xF}

// LFGDungeonRow is a row of LFGDungeons.dbc.
type LFGDungeonRow struct {
	ID                           uint32
	Name                         string
	MinLevel, MaxLevel, Target   uint32
	TargetMin, TargetMax         uint32
	Map, Difficulty, Flags, Type uint32
	Faction                      uint32
	Texture                      string
	Expansion, Order, Group      uint32
	Description                  string
}

// LFGGroupRow is a row of LFGDungeonGroup.dbc.
type LFGGroupRow struct {
	ID     uint32
	Name   string
	Order  uint32
	Parent uint32
	Type   uint32
}

// LFGExpansionRow is a row of LFGDungeonExpansion.dbc: the levels a dungeon
// takes for an expansion.
type LFGExpansionRow struct {
	Dungeon, Expansion                     uint32
	HardMin, HardMax, TargetMin, TargetMax uint32
}

// LFGEncounterRow is a row of DungeonEncounter.dbc.
type LFGEncounterRow struct {
	Map, Difficulty, Order, Bit uint32
	Name                        string
	Icon                        string
}

// LFGTables are the tables the finder reads.
type LFGTables struct {
	Dungeons   []LFGDungeonRow
	Groups     []LFGGroupRow
	Expansions []LFGExpansionRow
	Encounters []LFGEncounterRow
	// MapPlayers is MapDifficulty.dbc: how many players a map takes at a
	// difficulty, keyed by map and difficulty.
	MapPlayers map[[2]uint32]uint32
}

// lfgChoice is a place in the lists of dungeons the finder shows: a dungeon
// (a positive id) or a heading (a negative one) with the heading it is under.
type lfgChoice struct {
	id    int32
	group uint32
}

// lfgNode is what the finder knows of one dungeon for the character: how a
// random dungeon pays and whether the character may enter.
type lfgNode struct {
	entry    uint32
	done     bool
	locked   bool
	status   uint32
	money    uint32
	xp       uint32
	moneyVar uint32
	xpVar    uint32
	items    []world.LFGRewardItem
}

type lfgPartyPlayer struct {
	guid  uint64
	has   bool
	locks []*lfgNode
}

// lfgStatus is a place in the finder: the character's own wish, the character's
// entry or the party's.
type lfgStatus struct {
	joined, lfgJoined, queued, dirty bool
	noPartial, achievements          bool
	needs                            [3]uint8
	dungeons                         []uint32
	comment                          string
}

type lfgProposalState struct {
	entry      uint32
	state      uint8
	id         uint32
	encounters uint32
	silent     bool
	players    [5]world.LFGProposalPlayer
	lastID     uint32
}

// lfgThrottle holds back calls the exe lets through only a few times in a
// while.
type lfgThrottle struct {
	count int
	last  float64
}

type lfgState struct {
	tables     LFGTables
	dungeons   map[uint32]*LFGDungeonRow
	groups     map[uint32]*LFGGroupRow
	expansions map[[2]uint32]LFGExpansionRow
	level      uint32 // the expansion the account has

	choice []lfgChoice // dungeons the finder lists
	raid   []lfgChoice // raids it lists
	random []uint32    // entries of the random dungeons on offer
	best   uint32      // the random dungeon it suggests

	enabled   map[uint32]bool // dungeons ticked in the list
	collapsed map[uint32]bool // headings folded in the list
	roles     uint8           // the roles the character chose

	local     lfgStatus // what the character asks to queue for
	solo      lfgStatus // the character's own entry the server confirmed
	party     lfgStatus // the party's entry
	hasPlayer bool
	player    []*lfgNode
	hasParty  bool
	partyList []*lfgPartyPlayer

	proposal  lfgProposalState
	roleCheck world.LFGRoleCheck
	checkIn   bool // a role check is on
	queue     world.LFGQueueStatus
	queueFrom float64 // when the character joined the queue, in seconds
	boot      world.LFGBootVote
	reward    world.LFGReward
	rewardSet bool
	searched  uint32 // the entry joined in the raid browser

	groupDungeon uint32 // the dungeon the party was formed for
	restricted   bool

	throttles map[string]*lfgThrottle
}

func (s *lfgState) init() {
	s.dungeons = map[uint32]*LFGDungeonRow{}
	s.groups = map[uint32]*LFGGroupRow{}
	s.expansions = map[[2]uint32]LFGExpansionRow{}
	s.enabled = map[uint32]bool{}
	s.collapsed = map[uint32]bool{}
	s.throttles = map[string]*lfgThrottle{}
	s.level = 2
}

// SetLFGTables gives the finder the tables it reads.
func (rt *Runtime) SetLFGTables(tables LFGTables) {
	s := &rt.lfg
	s.tables = tables
	s.dungeons = make(map[uint32]*LFGDungeonRow, len(tables.Dungeons))
	for index := range s.tables.Dungeons {
		row := &s.tables.Dungeons[index]
		s.dungeons[row.ID] = row
	}
	s.groups = make(map[uint32]*LFGGroupRow, len(tables.Groups))
	for index := range s.tables.Groups {
		row := &s.tables.Groups[index]
		s.groups[row.ID] = row
	}
	s.expansions = make(map[[2]uint32]LFGExpansionRow, len(tables.Expansions))
	for _, row := range tables.Expansions {
		key := [2]uint32{row.Dungeon, row.Expansion}
		if _, found := s.expansions[key]; !found {
			s.expansions[key] = row
		}
	}
	rt.rebuildLFGChoices()
	rt.rebuildLFGRandom()
}

// SetExpansionLevel tells the finder which expansion the account has.
func (rt *Runtime) SetExpansionLevel(level uint32) { rt.lfg.level = level }

func (s *lfgState) dungeon(id uint32) *LFGDungeonRow { return s.dungeons[id&0xFFFFFF] }

// levels are the levels of a dungeon for the account's expansion.
func (s *lfgState) levels(row *LFGDungeonRow) (min, max, minRec, maxRec uint32) {
	if expansion, found := s.expansions[[2]uint32{row.ID, s.level}]; found {
		return expansion.HardMin, expansion.HardMax, expansion.TargetMin, expansion.TargetMax
	}
	return row.MinLevel, row.MaxLevel, row.TargetMin, row.TargetMax
}

// average is the middle of the levels a dungeon is fit for, by which the lists
// sort.
func (s *lfgState) average(row *LFGDungeonRow) int {
	if expansion, found := s.expansions[[2]uint32{row.ID, s.level}]; found {
		return int(expansion.HardMax+expansion.HardMin) / 2
	}
	return int(row.MaxLevel+row.MinLevel) / 2
}

// compare orders two places of the lists: by heading, headings before the
// dungeons under them, then by the order the table gives, the middle level and
// the name.
func (s *lfgState) compare(a, b lfgChoice) int {
	keyOf := func(c lfgChoice) uint32 {
		if c.id < 0 {
			return uint32(-c.id)
		}
		return c.group
	}
	orderOf := func(key uint32) int {
		if key == 0 {
			return -1
		}
		if row := s.groups[key]; row != nil {
			return int(row.Order)
		}
		return 0
	}
	ka, kb := keyOf(a), keyOf(b)
	if ka != kb {
		if difference := orderOf(ka) - orderOf(kb); difference != 0 {
			return difference
		}
	}
	if (a.id < 0) != (b.id < 0) {
		if a.id < 0 {
			return -1
		}
		return 1
	}
	da, db := s.dungeons[uint32(a.id)], s.dungeons[uint32(b.id)]
	if da == nil || db == nil {
		return 0
	}
	if da.Order != db.Order {
		return int(da.Order) - int(db.Order)
	}
	if difference := s.average(da) - s.average(db); difference != 0 {
		return difference
	}
	return strings.Compare(strings.ToLower(da.Name), strings.ToLower(db.Name))
}

func (rt *Runtime) rebuildLFGChoices() {
	s := &rt.lfg
	s.choice, s.raid = nil, nil
	for index := range s.tables.Dungeons {
		row := &s.tables.Dungeons[index]
		if row.Flags&lfgFlagFinder != 0 && row.Type != lfgTypeRandom && row.Flags&lfgFlagHoliday == 0 {
			s.choice = append(s.choice, lfgChoice{int32(row.ID), row.Group})
		}
		if row.Type == lfgTypeRaid {
			s.raid = append(s.raid, lfgChoice{int32(row.ID), row.Group})
		}
	}
	for _, group := range s.tables.Groups {
		if group.Type != 0 {
			s.choice = append(s.choice, lfgChoice{-int32(group.ID), group.Parent})
		}
		s.raid = append(s.raid, lfgChoice{-int32(group.ID), group.Parent})
	}
	sort.SliceStable(s.choice, func(i, j int) bool { return s.compare(s.choice[i], s.choice[j]) < 0 })
	sort.SliceStable(s.raid, func(i, j int) bool { return s.compare(s.raid[i], s.raid[j]) < 0 })
}

func (s *lfgState) playerNode(entry uint32) *lfgNode {
	if !s.hasPlayer {
		return nil
	}
	for _, node := range s.player {
		if node.entry == entry {
			return node
		}
	}
	return nil
}

func lfgNodeOf(list *[]*lfgNode, entry uint32) *lfgNode {
	for _, node := range *list {
		if node.entry == entry {
			return node
		}
	}
	node := &lfgNode{entry: entry}
	*list = append(*list, node)
	return node
}

// holidayOffered says whether a holiday dungeon is on offer: the server named
// it and the character may enter.
func (s *lfgState) holidayOffered(row *LFGDungeonRow) bool {
	if row.Flags&lfgFlagHoliday == 0 {
		return false
	}
	node := s.playerNode(row.Type<<24 | row.ID)
	return node != nil && !node.locked
}

// randomBetter reports whether a random dungeon is a better suggestion than the
// one already picked.
func (s *lfgState) randomBetter(candidate, best *LFGDungeonRow) bool {
	if candidate == nil || candidate.Type != lfgTypeRandom || !s.joinable(candidate.Type<<24|candidate.ID) {
		return false
	}
	if best == nil {
		return true
	}
	if candidate.Expansion != best.Expansion {
		return best.Expansion < candidate.Expansion
	}
	levelOf := func(row *LFGDungeonRow) int {
		if expansion, found := s.expansions[[2]uint32{row.ID, s.level}]; found {
			return int(expansion.TargetMax + expansion.TargetMin)
		}
		return int(row.TargetMax + row.TargetMin)
	}
	if a, b := levelOf(candidate)/2, levelOf(best)/2; a != b {
		return b < a
	}
	minOf := func(row *LFGDungeonRow) int {
		if expansion, found := s.expansions[[2]uint32{row.ID, s.level}]; found {
			return int(expansion.HardMin)
		}
		return int(row.MinLevel)
	}
	return minOf(best) < minOf(candidate)
}

// rebuildLFGRandom lists the random dungeons on offer and picks the one the
// finder suggests.
func (rt *Runtime) rebuildLFGRandom() {
	s := &rt.lfg
	var random []uint32
	for index := range s.tables.Dungeons {
		row := &s.tables.Dungeons[index]
		if row.Type == lfgTypeRandom || s.holidayOffered(row) {
			random = append(random, row.Type<<24|row.ID)
		}
	}
	s.random = random
	var best *LFGDungeonRow
	for _, entry := range s.random {
		if candidate := s.dungeon(entry); s.randomBetter(candidate, best) {
			best = candidate
		}
	}
	s.best = 0
	if best != nil {
		s.best = best.Type<<24 | best.ID
	}
}

// joinable reports whether the character and the party may enter a dungeon.
func (s *lfgState) joinable(entry uint32) bool {
	if node := s.playerNode(entry); node != nil && node.locked {
		return false
	}
	if entry&0xFF000000 != lfgTypeRaid<<24 && s.hasParty {
		for _, player := range s.partyList {
			for _, node := range player.locks {
				if node.entry == entry && node.locked {
					return false
				}
			}
		}
	}
	return true
}

func (rt *Runtime) lfgNow() float64 { return rt.GameTime() }

// lfgAllowed holds back a call that came more than twice in a short while.
func (rt *Runtime) lfgAllowed(name string, window float64) bool {
	s := &rt.lfg
	throttle := s.throttles[name]
	if throttle == nil {
		throttle = &lfgThrottle{}
		s.throttles[name] = throttle
	}
	throttle.count++
	if throttle.count > 2 {
		now := rt.lfgNow()
		if now-throttle.last < window {
			return false
		}
		throttle.last = now
		throttle.count = 0
	}
	return true
}

// availableRoles are the roles the character's class can take.
func (rt *Runtime) availableRoles() uint8 {
	if player := rt.unitInfo("player"); player != nil && player.ClassID >= 0 && player.ClassID < len(lfgClassRoles) {
		return lfgClassRoles[player.ClassID]
	}
	return 0
}

// groupSize is how many the character's group has, the character counted.
func (rt *Runtime) lfgGroupSize() int {
	if !rt.group.in {
		return 0
	}
	return len(rt.group.members) + 1
}

func (rt *Runtime) lfgEvent(name string, args ...lua.LValue) { rt.FireEvent(name, args...) }

// lfgStatusFor is the entry of the finder the character is in: the party's when
// in a group, the character's own otherwise.
func (rt *Runtime) lfgStatusFor() *lfgStatus {
	if rt.group.in {
		return &rt.lfg.party
	}
	return &rt.lfg.solo
}

// SetGroupLFG is told by the group list which dungeon the group was formed for
// and whether the finder restricts it.
func (rt *Runtime) setGroupLFG(dungeon uint32, restricted bool) {
	s := &rt.lfg
	if s.groupDungeon != dungeon {
		s.groupDungeon = dungeon
		rt.FireEvent("LFG_UPDATE")
	}
	if s.restricted != restricted {
		s.restricted = restricted
		rt.FireEvent("PARTY_LFG_RESTRICTED")
	}
}
