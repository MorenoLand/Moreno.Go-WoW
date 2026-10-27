package ui

import (
	"fmt"
	"sort"
	"strconv"

	"github.com/MorenoLand/Moreno.WoW/world"
	lua "github.com/yuin/gopher-lua"
)

// AchievementEntry is a row of Achievement.dbc.
type AchievementEntry struct {
	ID          uint32
	Faction     int32
	Map         int32
	Previous    uint32
	Name        string
	Description string
	Category    uint32
	Points      uint32
	Order       uint32
	Flags       uint32
	Icon        string
	Reward      string
}

// AchievementCategory is a row of Achievement_Category.dbc.
type AchievementCategory struct {
	ID     uint32
	Parent int32
	Name   string
	Order  uint32
}

// AchievementCriterion is a row of Achievement_Criteria.dbc.
type AchievementCriterion struct {
	ID          uint32
	Achievement uint32
	Type        uint32
	Asset       uint32
	Quantity    uint32
	Name        string
	Flags       uint32
	Order       uint32
}

// AchievementTables are the client tables of achievements.
type AchievementTables struct {
	Achievements map[uint32]*AchievementEntry
	Categories   map[uint32]*AchievementCategory
	Criteria     map[uint32]*AchievementCriterion
}

const (
	achievementFlagCounter = 0x1
	achievementFlagHidden  = 0x2
	criteriaFlagHidden     = 0x2
	// statisticsCategory is the root every statistic category hangs from.
	statisticsCategory = 1
)

// achievementSet is the achievements of one player: the character or a
// player being compared.
type achievementSet struct {
	done     map[uint32]uint32
	progress map[uint32]uint64
}

type achievementState struct {
	tables AchievementTables
	// Derived from the tables when they are installed.
	next            map[uint32]uint32
	byAchievement   map[uint32][]*AchievementCriterion
	byCategory      map[uint32][]*AchievementEntry
	categoriesOrder []*AchievementCategory
	own             achievementSet
	compare         achievementSet
	compareGUID     uint64
	tracked         []uint32
}

// AchievementHost is what the achievement window asks of the connection.
type AchievementHost interface {
	// InspectAchievements asks for another player's achievements.
	InspectAchievements(guid uint64) error
}

// SetAchievementTables installs the tables the achievement window lists.
func (rt *Runtime) SetAchievementTables(tables AchievementTables) {
	state := &rt.achievements
	state.tables = tables
	state.next = map[uint32]uint32{}
	state.byAchievement = map[uint32][]*AchievementCriterion{}
	state.byCategory = map[uint32][]*AchievementEntry{}
	for _, entry := range tables.Achievements {
		if entry.Previous != 0 {
			state.next[entry.Previous] = entry.ID
		}
		state.byCategory[entry.Category] = append(state.byCategory[entry.Category], entry)
	}
	for _, list := range state.byCategory {
		sort.Slice(list, func(a, b int) bool {
			if list[a].Order != list[b].Order {
				return list[a].Order < list[b].Order
			}
			return list[a].ID < list[b].ID
		})
	}
	for _, criterion := range tables.Criteria {
		state.byAchievement[criterion.Achievement] = append(state.byAchievement[criterion.Achievement], criterion)
	}
	for _, list := range state.byAchievement {
		sort.Slice(list, func(a, b int) bool {
			if list[a].Order != list[b].Order {
				return list[a].Order < list[b].Order
			}
			return list[a].ID < list[b].ID
		})
	}
	state.categoriesOrder = state.categoriesOrder[:0]
	for _, category := range tables.Categories {
		state.categoriesOrder = append(state.categoriesOrder, category)
	}
	sort.Slice(state.categoriesOrder, func(a, b int) bool {
		list := state.categoriesOrder
		if list[a].Order != list[b].Order {
			return list[a].Order < list[b].Order
		}
		return list[a].ID < list[b].ID
	})
}

func newAchievementSet(data world.AchievementData) achievementSet {
	set := achievementSet{done: map[uint32]uint32{}, progress: map[uint32]uint64{}}
	for _, done := range data.Done {
		set.done[done.ID] = done.Date
	}
	for _, progress := range data.Progress {
		set.progress[progress.ID] = progress.Counter
	}
	return set
}

// SetAchievementData installs the achievements the server sent at login.
func (rt *Runtime) SetAchievementData(data world.AchievementData) {
	rt.achievements.own = newAchievementSet(data)
	rt.FireEvent("ACHIEVEMENT_EARNED_UPDATE")
}

// SetInspectedAchievements installs the achievements of a compared player.
func (rt *Runtime) SetInspectedAchievements(data world.AchievementData) {
	rt.achievements.compare = newAchievementSet(data)
	rt.FireEvent("INSPECT_ACHIEVEMENT_READY", numberValue(0))
}

// EarnAchievement records an achievement the character has just earned.
func (rt *Runtime) EarnAchievement(info world.AchievementEarnedInfo) {
	state := &rt.achievements
	if state.own.done == nil {
		state.own = newAchievementSet(world.AchievementData{})
	}
	state.own.done[info.ID] = info.Date
	rt.FireEvent("ACHIEVEMENT_EARNED", numberValue(info.ID))
}

// UpdateCriteria records the new progress of a criterion.
func (rt *Runtime) UpdateCriteria(progress world.CriteriaProgress) {
	state := &rt.achievements
	if state.own.progress == nil {
		state.own = newAchievementSet(world.AchievementData{})
	}
	state.own.progress[progress.ID] = progress.Counter
	rt.FireEvent("CRITERIA_UPDATE")
}

func (rt *Runtime) achievementHost() AchievementHost {
	host, _ := rt.Host.(AchievementHost)
	return host
}

// playerTeam is 0 for the Horde and 1 for the Alliance.
func (rt *Runtime) playerTeam() int32 {
	if info := rt.unitInfo("player"); info != nil {
		switch info.RaceID {
		case 1, 3, 4, 7, 11:
			return 1
		}
	}
	return 0
}

// achievementShown tells whether the player's faction may see an achievement.
func (rt *Runtime) achievementShown(entry *AchievementEntry) bool {
	if entry.Flags&achievementFlagHidden != 0 {
		return false
	}
	return entry.Faction < 0 || entry.Faction == rt.playerTeam()
}

// categoryRoot is the top of a category's chain of parents.
func (rt *Runtime) categoryRoot(id uint32) uint32 {
	for steps := 0; steps < 8; steps++ {
		category := rt.achievements.tables.Categories[id]
		if category == nil || category.Parent < 0 {
			return id
		}
		id = uint32(category.Parent)
	}
	return id
}

func (rt *Runtime) isStatisticCategory(id uint32) bool {
	return rt.categoryRoot(id) == statisticsCategory
}

// criterionCounter is how far a criterion has come in a set.
func criterionCounter(set achievementSet, criterion *AchievementCriterion) uint64 {
	return set.progress[criterion.ID]
}

func criterionDone(set achievementSet, criterion *AchievementCriterion) bool {
	return criterionCounter(set, criterion) >= uint64(max(criterion.Quantity, 1))
}

// chainHead is the achievement of a chain a category lists: the first one not
// yet done, or the last when all are.
func (rt *Runtime) chainHead(set achievementSet, first uint32) uint32 {
	state := &rt.achievements
	id := first
	for steps := 0; steps < 16; steps++ {
		if _, done := set.done[id]; !done {
			return id
		}
		next, ok := state.next[id]
		if !ok {
			return id
		}
		id = next
	}
	return id
}

// categoryAchievements lists what a category shows: all of them, or one of
// each chain.
func (rt *Runtime) categoryAchievements(set achievementSet, category uint32, showAll bool) []*AchievementEntry {
	state := &rt.achievements
	var list []*AchievementEntry
	for _, entry := range state.byCategory[category] {
		if !rt.achievementShown(entry) {
			continue
		}
		if !showAll && !rt.isStatisticCategory(category) {
			if entry.Previous != 0 {
				continue
			}
			head := rt.chainHead(set, entry.ID)
			if head != entry.ID {
				if shown := state.tables.Achievements[head]; shown != nil {
					entry = shown
				}
			}
		}
		list = append(list, entry)
	}
	return list
}

// achievementTexture is the picture of an achievement.
func achievementTexture(entry *AchievementEntry) string { return entry.Icon }

// pushAchievementInfo gives the values of GetAchievementInfo.
func (rt *Runtime) pushAchievementInfo(L *lua.LState, set achievementSet, entry *AchievementEntry) int {
	date, completed := set.done[entry.ID]
	month, day, year := 0, 0, 0
	if completed {
		y, m, d, _, _ := world.UnpackTime(date)
		month, day, year = m, d, y%100
	}
	L.Push(lua.LNumber(entry.ID))
	L.Push(lua.LString(entry.Name))
	L.Push(lua.LNumber(entry.Points))
	L.Push(lua.LBool(completed))
	L.Push(lua.LNumber(month))
	L.Push(lua.LNumber(day))
	L.Push(lua.LNumber(year))
	L.Push(lua.LString(entry.Description))
	L.Push(lua.LNumber(entry.Flags))
	L.Push(lua.LString(achievementTexture(entry)))
	L.Push(lua.LString(entry.Reward))
	return 11
}

// statisticValue is the figure a statistic shows.
func (rt *Runtime) statisticValue(set achievementSet, id uint32) string {
	list := rt.achievements.byAchievement[id]
	if len(list) == 0 {
		return "--"
	}
	var total uint64
	seen := false
	for _, criterion := range list {
		if counter, ok := set.progress[criterion.ID]; ok {
			total += counter
			seen = true
		}
	}
	if !seen {
		return "--"
	}
	return strconv.FormatUint(total, 10)
}

// registerAchievementAPI defines the calls the achievement window makes.
func registerAchievementAPI(rt *Runtime) {
	L := rt.L
	reg := func(name string, fn lua.LGFunction) { L.SetGlobal(name, L.NewFunction(fn)) }
	state := &rt.achievements
	categories := func(statistics bool) lua.LGFunction {
		return func(L *lua.LState) int {
			list := L.NewTable()
			for _, category := range state.categoriesOrder {
				if rt.isStatisticCategory(category.ID) == statistics {
					list.Append(lua.LNumber(category.ID))
				}
			}
			L.Push(list)
			return 1
		}
	}
	reg("CanShowAchievementUI", func(L *lua.LState) int { L.Push(lua.LTrue); return 1 })
	reg("HasCompletedAnyAchievement", func(L *lua.LState) int { L.Push(lua.LBool(len(state.own.done) > 0)); return 1 })
	reg("GetCategoryList", categories(false))
	reg("GetStatisticsCategoryList", categories(true))
	reg("GetCategoryInfo", func(L *lua.LState) int {
		category := state.tables.Categories[uint32(L.CheckInt(1))]
		if category == nil {
			return 0
		}
		L.Push(lua.LString(category.Name))
		L.Push(lua.LNumber(category.Parent))
		L.Push(lua.LNumber(0))
		return 3
	})
	numAchievements := func(set func() achievementSet) lua.LGFunction {
		return func(L *lua.LState) int {
			id := uint32(L.CheckInt(1))
			own := state.own
			list := rt.categoryAchievements(own, id, L.OptBool(2, false))
			done := 0
			for _, entry := range list {
				if _, ok := own.done[entry.ID]; ok {
					done++
				}
			}
			L.Push(lua.LNumber(len(list)))
			L.Push(lua.LNumber(done))
			L.Push(lua.LNumber(len(list) - done))
			return 3
		}
	}
	reg("GetCategoryNumAchievements", numAchievements(func() achievementSet { return state.own }))
	reg("GetComparisonCategoryNumAchievements", func(L *lua.LState) int {
		id := uint32(L.CheckInt(1))
		list := rt.categoryAchievements(state.compare, id, false)
		done := 0
		for _, entry := range list {
			if _, ok := state.compare.done[entry.ID]; ok {
				done++
			}
		}
		L.Push(lua.LNumber(len(list)))
		L.Push(lua.LNumber(done))
		L.Push(lua.LNumber(len(list) - done))
		return 3
	})
	info := func(set func() achievementSet) lua.LGFunction {
		return func(L *lua.LState) int {
			var entry *AchievementEntry
			if L.GetTop() >= 2 && L.Get(2) != lua.LNil {
				list := rt.categoryAchievements(set(), uint32(L.CheckInt(1)), false)
				if index := L.CheckInt(2); index >= 1 && index <= len(list) {
					entry = list[index-1]
				}
			} else {
				entry = state.tables.Achievements[uint32(L.CheckInt(1))]
			}
			if entry == nil {
				return 0
			}
			return rt.pushAchievementInfo(L, set(), entry)
		}
	}
	reg("GetAchievementInfo", info(func() achievementSet { return state.own }))
	reg("GetAchievementInfoFromCriteria", func(L *lua.LState) int {
		criterion := state.tables.Criteria[uint32(L.CheckInt(1))]
		if criterion == nil {
			return 0
		}
		entry := state.tables.Achievements[criterion.Achievement]
		if entry == nil {
			return 0
		}
		return rt.pushAchievementInfo(L, state.own, entry)
	})
	reg("GetAchievementCategory", func(L *lua.LState) int {
		if entry := state.tables.Achievements[uint32(L.CheckInt(1))]; entry != nil {
			L.Push(lua.LNumber(entry.Category))
			return 1
		}
		return 0
	})
	reg("GetPreviousAchievement", func(L *lua.LState) int {
		if entry := state.tables.Achievements[uint32(L.CheckInt(1))]; entry != nil && entry.Previous != 0 {
			L.Push(lua.LNumber(entry.Previous))
			L.Push(lua.LBool(func() bool { _, ok := state.own.done[entry.Previous]; return ok }()))
			return 2
		}
		return 0
	})
	reg("GetNextAchievement", func(L *lua.LState) int {
		if next, ok := state.next[uint32(L.CheckInt(1))]; ok {
			_, done := state.own.done[next]
			L.Push(lua.LNumber(next))
			L.Push(lua.LBool(done))
			return 2
		}
		return 0
	})
	reg("GetAchievementNumCriteria", func(L *lua.LState) int {
		count := 0
		for _, criterion := range state.byAchievement[uint32(L.CheckInt(1))] {
			if criterion.Flags&criteriaFlagHidden == 0 {
				count++
			}
		}
		L.Push(lua.LNumber(count))
		return 1
	})
	criteriaInfo := func(set func() achievementSet) lua.LGFunction {
		return func(L *lua.LState) int {
			var criterion *AchievementCriterion
			if L.GetTop() >= 2 && L.Get(2) != lua.LNil {
				visible := 0
				for _, candidate := range state.byAchievement[uint32(L.CheckInt(1))] {
					if candidate.Flags&criteriaFlagHidden != 0 {
						continue
					}
					if visible++; visible == L.CheckInt(2) {
						criterion = candidate
						break
					}
				}
			} else {
				criterion = state.tables.Criteria[uint32(L.CheckInt(1))]
			}
			if criterion == nil {
				return 0
			}
			counter := criterionCounter(set(), criterion)
			quantityText := strconv.FormatUint(counter, 10)
			L.Push(lua.LString(criterion.Name))
			L.Push(lua.LNumber(criterion.Type))
			L.Push(lua.LBool(criterionDone(set(), criterion)))
			L.Push(lua.LNumber(counter))
			L.Push(lua.LNumber(criterion.Quantity))
			L.Push(lua.LString(""))
			L.Push(lua.LNumber(criterion.Flags))
			L.Push(lua.LNumber(criterion.Asset))
			L.Push(lua.LString(quantityText))
			L.Push(lua.LNumber(criterion.ID))
			return 10
		}
	}
	reg("GetAchievementCriteriaInfo", criteriaInfo(func() achievementSet { return state.own }))
	reg("GetNumCompletedAchievements", func(L *lua.LState) int {
		total, done := 0, 0
		for _, entry := range state.tables.Achievements {
			if entry.Flags&(achievementFlagCounter|achievementFlagHidden) != 0 || !rt.achievementShown(entry) {
				continue
			}
			total++
			if _, ok := state.own.done[entry.ID]; ok {
				done++
			}
		}
		L.Push(lua.LNumber(total))
		L.Push(lua.LNumber(done))
		return 2
	})
	points := func(set func() achievementSet) lua.LGFunction {
		return func(L *lua.LState) int {
			total := 0
			for id := range set().done {
				if entry := state.tables.Achievements[id]; entry != nil {
					total += int(entry.Points)
				}
			}
			L.Push(lua.LNumber(total))
			return 1
		}
	}
	reg("GetTotalAchievementPoints", points(func() achievementSet { return state.own }))
	reg("GetComparisonAchievementPoints", points(func() achievementSet { return state.compare }))
	reg("GetLatestCompletedAchievements", func(L *lua.LState) int {
		type recent struct {
			id   uint32
			date uint32
		}
		var list []recent
		for id, date := range state.own.done {
			if entry := state.tables.Achievements[id]; entry != nil && entry.Flags&achievementFlagCounter == 0 {
				list = append(list, recent{id, date})
			}
		}
		sort.Slice(list, func(a, b int) bool {
			if list[a].date != list[b].date {
				return list[a].date > list[b].date
			}
			return list[a].id > list[b].id
		})
		for index := 0; index < len(list) && index < 5; index++ {
			L.Push(lua.LNumber(list[index].id))
		}
		return min(len(list), 5)
	})
	reg("GetAchievementLink", func(L *lua.LState) int {
		id := uint32(L.CheckInt(1))
		entry := state.tables.Achievements[id]
		if entry == nil {
			return 0
		}
		date, done := state.own.done[id]
		month, day, year, bits := 0, 0, 0, 0
		if done {
			y, m, d, _, _ := world.UnpackTime(date)
			month, day, year, bits = m, d, y%100, 0xFFFFFFFF
		}
		guid := uint64(0)
		if info := rt.unitInfo("player"); info != nil {
			guid = info.GUID
		}
		L.Push(lua.LString(fmt.Sprintf("|cffffff00|Hachievement:%d:%016X:%d:%d:%d:%d:%d:%d:%d:%d|h[%s]|h|r", id, guid, boolInt(done), month, day, year, bits, 0, 0, 0, entry.Name)))
		return 1
	})
	reg("GetStatistic", func(L *lua.LState) int {
		L.Push(lua.LString(rt.statisticValue(state.own, uint32(L.CheckInt(1)))))
		return 1
	})
	reg("GetComparisonStatistic", func(L *lua.LState) int {
		L.Push(lua.LString(rt.statisticValue(state.compare, uint32(L.CheckInt(1)))))
		return 1
	})
	reg("GetAchievementComparisonInfo", info(func() achievementSet { return state.compare }))
	reg("SetAchievementComparisonUnit", func(L *lua.LState) int {
		unit := L.CheckString(1)
		guid := rt.UnitGUID(unit)
		state.compareGUID = guid
		state.compare = newAchievementSet(world.AchievementData{})
		if host := rt.achievementHost(); host != nil && guid != 0 {
			_ = host.InspectAchievements(guid)
		}
		return 0
	})
	reg("ClearAchievementComparisonUnit", func(L *lua.LState) int {
		state.compareGUID = 0
		state.compare = achievementSet{}
		return 0
	})
	// The tracker.
	reg("GetTrackedAchievements", func(L *lua.LState) int {
		for _, id := range state.tracked {
			L.Push(lua.LNumber(id))
		}
		return len(state.tracked)
	})
	reg("GetNumTrackedAchievements", func(L *lua.LState) int { L.Push(lua.LNumber(len(state.tracked))); return 1 })
	reg("IsTrackedAchievement", func(L *lua.LState) int {
		id := uint32(L.CheckInt(1))
		for _, tracked := range state.tracked {
			if tracked == id {
				L.Push(lua.LTrue)
				return 1
			}
		}
		L.Push(lua.LFalse)
		return 1
	})
	reg("AddTrackedAchievement", func(L *lua.LState) int {
		id := uint32(L.CheckInt(1))
		if len(state.tracked) >= 10 {
			return 0
		}
		for _, tracked := range state.tracked {
			if tracked == id {
				return 0
			}
		}
		state.tracked = append(state.tracked, id)
		rt.FireEvent("TRACKED_ACHIEVEMENT_UPDATE", numberValue(id))
		return 0
	})
	reg("RemoveTrackedAchievement", func(L *lua.LState) int {
		id := uint32(L.CheckInt(1))
		for index, tracked := range state.tracked {
			if tracked == id {
				state.tracked = append(state.tracked[:index:index], state.tracked[index+1:]...)
				rt.FireEvent("TRACKED_ACHIEVEMENT_UPDATE", numberValue(id))
				break
			}
		}
		return 0
	})
}
