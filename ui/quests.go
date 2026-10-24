package ui

import (
	"fmt"
	"sort"
	"strings"

	"github.com/MorenoLand/Moreno.WoW/world"
	lua "github.com/yuin/gopher-lua"
)

// QuestHost is what quests ask of the connection.
type QuestHost interface {
	QueryQuest(id uint32) error
	QuestGiverHello(guid uint64) error
	QuestGiverQuery(guid uint64, quest uint32) error
	QuestGiverAccept(guid uint64, quest uint32) error
	QuestGiverComplete(guid uint64, quest uint32) error
	QuestGiverRequestReward(guid uint64, quest uint32) error
	QuestGiverChoose(guid uint64, quest, choice uint32) error
	QuestGiverCancel() error
	AbandonQuest(slot uint8) error
}

// Quest log slot states and the log's size.
const (
	questLogSlots    = 25
	questStateDone   = 1
	questStateFailed = 2
	// questIconActive is the icon the server gives a quest the character holds,
	// finished or not; the others are the quests it can take.
	questIconActive   = 4
	questIconComplete = 1 << 30
)

// PLAYER_QUEST_LOG_1_1 of the 3.3.5a player object: five fields per slot, the
// quest id, its state, two with the four counters and the timer.
const (
	questLogField  = 158
	questLogStride = 5
)

// QuestLogSlot is one quest the character holds.
type QuestLogSlot struct {
	ID     uint32
	State  uint32
	Counts [4]uint16
	Timer  uint32
}

type questDialogKind int

const (
	questDialogNone questDialogKind = iota
	questDialogGreeting
	questDialogDetail
	questDialogProgress
	questDialogComplete
)

// questDialog is the window of a quest giver.
type questDialog struct {
	kind      questDialogKind
	giver     uint64
	id        uint32
	title     string
	text      string
	objective string
	greeting  string
	quests    []world.GossipQuest
	rewards   world.QuestRewards
	items     []world.QuestItem
	money     uint32
	complete  bool
	auto      bool
	// choice is what the reward window answers.
	choice int
}

type questState struct {
	slots     [questLogSlots]QuestLogSlot
	info      map[uint32]world.QuestInfo
	asked     map[uint32]bool
	selected  int
	collapsed map[string]bool
	watched   map[uint32]bool
	dialog    questDialog
	// sortName names a quest's zone or category, set by the world.
	sortName func(sort int32) string
	// creatureName names a creature or object a quest asks to kill or use.
	creatureName func(entry int32) string
}

// SetQuestNames gives the quest log the lookups that name a quest's zone and
// its targets.
func (rt *Runtime) SetQuestNames(sort func(int32) string, creature func(int32) string) {
	rt.quests.sortName, rt.quests.creatureName = sort, creature
}

func (rt *Runtime) questHost() QuestHost {
	host, _ := rt.Host.(QuestHost)
	return host
}

// SetQuestLog replaces the quest slots from the player's fields and asks for
// the text of quests it does not know.
func (rt *Runtime) SetQuestLog(slots [questLogSlots]QuestLogSlot) {
	if rt == nil {
		return
	}
	changed := slots != rt.quests.slots
	rt.quests.slots = slots
	for _, slot := range slots {
		if slot.ID != 0 {
			rt.requestQuestInfo(slot.ID)
		}
	}
	if changed {
		rt.FireEvent("QUEST_LOG_UPDATE")
		rt.FireEvent("UNIT_QUEST_LOG_CHANGED", lua.LString("player"))
	}
}

// QuestSlotsFrom reads the quest slots out of the player's fields.
func QuestSlotsFrom(fields map[uint16]uint32) [questLogSlots]QuestLogSlot {
	var slots [questLogSlots]QuestLogSlot
	for index := range slots {
		base := uint16(questLogField + index*questLogStride)
		slot := &slots[index]
		slot.ID = fields[base]
		if slot.ID == 0 {
			continue
		}
		slot.State = fields[base+1]
		for counter := 0; counter < 4; counter++ {
			value := fields[base+2+uint16(counter/2)]
			slot.Counts[counter] = uint16(value >> (16 * uint(counter%2)))
		}
		slot.Timer = fields[base+4]
	}
	return slots
}

func (rt *Runtime) requestQuestInfo(id uint32) {
	if _, known := rt.quests.info[id]; known || rt.quests.asked[id] {
		return
	}
	if rt.quests.asked == nil {
		rt.quests.asked = make(map[uint32]bool)
	}
	rt.quests.asked[id] = true
	if host := rt.questHost(); host != nil {
		_ = host.QueryQuest(id)
	}
}

// SetQuestInfo records what the server told of a quest.
func (rt *Runtime) SetQuestInfo(info world.QuestInfo) {
	if rt.quests.info == nil {
		rt.quests.info = make(map[uint32]world.QuestInfo)
	}
	rt.quests.info[info.ID] = info
	rt.FireEvent("QUEST_LOG_UPDATE")
}

// questRow is a line of the quest log: a zone header or a quest.
type questRow struct {
	header string
	slot   int
}

// questRows lists the quest log under its zone headers.
func (rt *Runtime) questRows() []questRow {
	groups := make(map[string][]int)
	var order []string
	for index, slot := range rt.quests.slots {
		if slot.ID == 0 {
			continue
		}
		header := "Quests"
		if info, ok := rt.quests.info[slot.ID]; ok && rt.quests.sortName != nil {
			if name := rt.quests.sortName(info.Sort); name != "" {
				header = name
			}
		}
		if _, seen := groups[header]; !seen {
			order = append(order, header)
		}
		groups[header] = append(groups[header], index)
	}
	sort.Strings(order)
	var rows []questRow
	for _, header := range order {
		rows = append(rows, questRow{header: header, slot: -1})
		if rt.quests.collapsed[header] {
			continue
		}
		for _, index := range groups[header] {
			rows = append(rows, questRow{slot: index})
		}
	}
	return rows
}

func (rt *Runtime) questTitle(id uint32) string {
	if info, ok := rt.quests.info[id]; ok {
		return info.Title
	}
	return fmt.Sprintf("Quest %d", id)
}

func (rt *Runtime) questComplete(slot QuestLogSlot) bool { return slot.State&questStateDone != 0 }

// gossipQuests are the quests of the open gossip menu, the ones the character
// holds (active) or can take.
func (rt *Runtime) gossipQuests(active bool) []world.GossipQuest {
	var list []world.GossipQuest
	for _, quest := range rt.gossip.menu.Quests {
		if (quest.Icon == questIconActive) == active {
			if active {
				quest.Icon = 0
				for _, slot := range rt.quests.slots {
					if slot.ID == quest.ID && rt.questComplete(slot) {
						quest.Icon = questIconComplete
					}
				}
			}
			list = append(list, quest)
		}
	}
	return list
}

func (rt *Runtime) selectGossipQuest(index int, active bool) {
	quests := rt.gossipQuests(active)
	if index < 1 || index > len(quests) {
		return
	}
	host := rt.questHost()
	if host == nil {
		return
	}
	quest := quests[index-1]
	if active {
		_ = host.QuestGiverRequestReward(rt.gossip.menu.GUID, quest.ID)
		_ = host.QuestGiverComplete(rt.gossip.menu.GUID, quest.ID)
		return
	}
	_ = host.QuestGiverQuery(rt.gossip.menu.GUID, quest.ID)
}

// ShowQuestGreeting opens the list of quests a creature without gossip offers.
func (rt *Runtime) ShowQuestGreeting(list world.QuestList) {
	rt.quests.dialog = questDialog{kind: questDialogGreeting, giver: list.Giver, greeting: list.Greeting, quests: list.Quests}
	rt.FireEvent("QUEST_GREETING")
}

// ShowQuestDetails opens the offer of a quest.
func (rt *Runtime) ShowQuestDetails(details world.QuestDetails) {
	rt.CloseGossipWindow()
	rt.quests.dialog = questDialog{kind: questDialogDetail, giver: details.Giver, id: details.ID, title: details.Title, text: details.Details, objective: details.Objectives, rewards: details.Rewards, auto: details.AutoLaunched}
	rt.FireEvent("QUEST_DETAIL")
}

// ShowQuestProgress opens what a quest still asks for.
func (rt *Runtime) ShowQuestProgress(progress world.QuestProgress) {
	rt.CloseGossipWindow()
	rt.quests.dialog = questDialog{kind: questDialogProgress, giver: progress.Giver, id: progress.ID, title: progress.Title, text: progress.Text, items: progress.Items, money: progress.Money, complete: progress.CanComplete}
	rt.FireEvent("QUEST_PROGRESS")
}

// ShowQuestReward opens the reward window.
func (rt *Runtime) ShowQuestReward(offer world.QuestOffer) {
	rt.CloseGossipWindow()
	rt.quests.dialog = questDialog{kind: questDialogComplete, giver: offer.Giver, id: offer.ID, title: offer.Title, text: offer.RewardText, rewards: offer.Rewards, auto: offer.AutoLaunched}
	rt.FireEvent("QUEST_COMPLETE")
}

// CloseQuestDialog closes the quest giver's window.
func (rt *Runtime) CloseQuestDialog() {
	if rt.quests.dialog.kind == questDialogNone {
		return
	}
	rt.quests.dialog = questDialog{}
	rt.FireEvent("QUEST_FINISHED")
}

// AnnounceQuestKill puts the progress of a kill objective on the screen.
func (rt *Runtime) AnnounceQuestKill(kill world.QuestKill) {
	name := ""
	if rt.quests.creatureName != nil {
		name = rt.quests.creatureName(int32(kill.Entry & 0x7FFFFFFF))
	}
	rt.ShowInfoMessage(fmt.Sprintf("%s: %d/%d", name, kill.Count, kill.Needed))
	rt.FireEvent("QUEST_WATCH_UPDATE", lua.LNumber(rt.questIndexOf(kill.Quest)))
	rt.FireEvent("QUEST_LOG_UPDATE")
}

func (rt *Runtime) questIndexOf(id uint32) int {
	for index, row := range rt.questRows() {
		if row.slot >= 0 && rt.quests.slots[row.slot].ID == id {
			return index + 1
		}
	}
	return 0
}

// questObjectives are the lines of a quest's objectives: kill and use targets
// then the items to gather, each with the progress made.
type questObjective struct {
	text     string
	kind     string
	finished bool
}

func (rt *Runtime) questObjectives(slot QuestLogSlot) []questObjective {
	info, ok := rt.quests.info[slot.ID]
	if !ok {
		return nil
	}
	var list []questObjective
	for index, target := range info.Targets {
		if target.Entry == 0 {
			continue
		}
		name := info.TargetText[index]
		if name == "" && rt.quests.creatureName != nil {
			name = rt.quests.creatureName(target.Entry)
		}
		kind := "monster"
		if target.Entry < 0 {
			kind = "object"
		}
		count := uint32(slot.Counts[index])
		list = append(list, questObjective{text: fmt.Sprintf("%s: %d/%d", name, count, target.Count), kind: kind, finished: count >= target.Count})
	}
	for _, item := range info.Items {
		if item.Entry == 0 {
			continue
		}
		name := ""
		if template := rt.itemTemplateIfKnown(item.Entry); template != nil {
			name = template.Name
		}
		have := uint32(rt.itemCountOf(item.Entry))
		list = append(list, questObjective{text: fmt.Sprintf("%s: %d/%d", name, have, item.Count), kind: "item", finished: have >= item.Count})
	}
	return list
}

// registerQuestAPI defines the calls the quest windows and the quest log make.
func registerQuestAPI(rt *Runtime) {
	L := rt.L
	reg := func(name string, fn lua.LGFunction) { L.SetGlobal(name, L.NewFunction(fn)) }
	dialog := func() *questDialog { return &rt.quests.dialog }
	// The quest giver window.
	reg("GetTitleText", func(L *lua.LState) int { L.Push(lua.LString(dialog().title)); return 1 })
	reg("GetGreetingText", func(L *lua.LState) int { L.Push(lua.LString(rt.expandNPCText(dialog().greeting))); return 1 })
	reg("GetQuestText", func(L *lua.LState) int { L.Push(lua.LString(rt.expandNPCText(dialog().text))); return 1 })
	reg("GetObjectiveText", func(L *lua.LState) int { L.Push(lua.LString(rt.expandNPCText(dialog().objective))); return 1 })
	reg("GetProgressText", func(L *lua.LState) int { L.Push(lua.LString(rt.expandNPCText(dialog().text))); return 1 })
	reg("GetRewardText", func(L *lua.LState) int { L.Push(lua.LString(rt.expandNPCText(dialog().text))); return 1 })
	reg("QuestGetAutoAccept", func(L *lua.LState) int { L.Push(lua.LBool(dialog().auto)); return 1 })
	reg("QuestFlagsPVP", func(L *lua.LState) int { L.Push(lua.LFalse); return 1 })
	reg("IsQuestCompletable", func(L *lua.LState) int { L.Push(lua.LBool(dialog().complete)); return 1 })
	reg("GetQuestMoneyToGet", func(L *lua.LState) int { L.Push(lua.LNumber(dialog().money)); return 1 })
	reg("GetNumQuestItems", func(L *lua.LState) int { L.Push(lua.LNumber(len(dialog().items))); return 1 })
	reg("GetNumQuestChoices", func(L *lua.LState) int { L.Push(lua.LNumber(len(dialog().rewards.Choices))); return 1 })
	reg("GetNumQuestRewards", func(L *lua.LState) int { L.Push(lua.LNumber(len(dialog().rewards.Items))); return 1 })
	reg("GetRewardMoney", func(L *lua.LState) int { L.Push(lua.LNumber(dialog().rewards.Money)); return 1 })
	reg("GetRewardXP", func(L *lua.LState) int { L.Push(lua.LNumber(0)); return 1 })
	reg("GetRewardSpell", func(L *lua.LState) int {
		if spell := dialog().rewards.DisplaySpell; spell != 0 {
			L.Push(lua.LNumber(spell))
			return 1
		}
		return 0
	})
	reg("GetRewardTitle", func(L *lua.LState) int { return 0 })
	reg("GetRewardTalents", func(L *lua.LState) int { L.Push(lua.LNumber(dialog().rewards.Talents)); return 1 })
	reg("GetRewardArenaPoints", func(L *lua.LState) int { L.Push(lua.LNumber(dialog().rewards.ArenaPoints)); return 1 })
	reg("GetNumRewardCurrencies", func(L *lua.LState) int { L.Push(lua.LNumber(0)); return 1 })
	questItem := func(kind string, index int) (world.QuestItem, bool) {
		var list []world.QuestItem
		switch kind {
		case "choice":
			list = dialog().rewards.Choices
		case "reward":
			list = dialog().rewards.Items
		case "required":
			list = dialog().items
		}
		if index < 1 || index > len(list) {
			return world.QuestItem{}, false
		}
		return list[index-1], true
	}
	reg("GetQuestItemInfo", func(L *lua.LState) int {
		item, ok := questItem(L.CheckString(1), L.CheckInt(2))
		if !ok {
			return 0
		}
		name, quality := "", 1
		if template := rt.itemTemplateIfKnown(item.Entry); template != nil {
			name, quality = template.Name, int(template.Quality)
		}
		L.Push(lua.LString(name))
		L.Push(lua.LString(rt.itemIcon(item.Entry)))
		L.Push(lua.LNumber(item.Count))
		L.Push(lua.LNumber(quality))
		L.Push(lua.LTrue)
		return 5
	})
	reg("GetQuestItemLink", func(L *lua.LState) int {
		item, ok := questItem(L.CheckString(1), L.CheckInt(2))
		if !ok {
			return 0
		}
		L.Push(lua.LString(rt.itemLinkFor(item.Entry, 0)))
		return 1
	})
	reg("GetNumAvailableQuests", func(L *lua.LState) int { L.Push(lua.LNumber(len(rt.greetingQuests(false)))); return 1 })
	reg("GetNumActiveQuests", func(L *lua.LState) int { L.Push(lua.LNumber(len(rt.greetingQuests(true)))); return 1 })
	reg("GetAvailableTitle", func(L *lua.LState) int {
		if quests := rt.greetingQuests(false); L.CheckInt(1) >= 1 && L.CheckInt(1) <= len(quests) {
			L.Push(lua.LString(quests[L.CheckInt(1)-1].Title))
			return 1
		}
		return 0
	})
	reg("GetActiveTitle", func(L *lua.LState) int {
		if quests := rt.greetingQuests(true); L.CheckInt(1) >= 1 && L.CheckInt(1) <= len(quests) {
			L.Push(lua.LString(quests[L.CheckInt(1)-1].Title))
			return 1
		}
		return 0
	})
	reg("GetAvailableQuestInfo", func(L *lua.LState) int {
		L.Push(lua.LFalse)
		L.Push(lua.LFalse)
		L.Push(lua.LFalse)
		return 3
	})
	reg("IsActiveQuestTrivial", func(L *lua.LState) int { L.Push(lua.LFalse); return 1 })
	reg("SelectAvailableQuest", func(L *lua.LState) int {
		quests := rt.greetingQuests(false)
		if index := L.CheckInt(1); index >= 1 && index <= len(quests) {
			if host := rt.questHost(); host != nil {
				_ = host.QuestGiverQuery(dialog().giver, quests[index-1].ID)
			}
		}
		return 0
	})
	reg("SelectActiveQuest", func(L *lua.LState) int {
		quests := rt.greetingQuests(true)
		if index := L.CheckInt(1); index >= 1 && index <= len(quests) {
			if host := rt.questHost(); host != nil {
				_ = host.QuestGiverRequestReward(dialog().giver, quests[index-1].ID)
				_ = host.QuestGiverComplete(dialog().giver, quests[index-1].ID)
			}
		}
		return 0
	})
	reg("AcceptQuest", func(L *lua.LState) int {
		if host := rt.questHost(); host != nil && dialog().id != 0 {
			_ = host.QuestGiverAccept(dialog().giver, dialog().id)
		}
		rt.CloseQuestDialog()
		return 0
	})
	reg("DeclineQuest", func(L *lua.LState) int {
		if host := rt.questHost(); host != nil {
			_ = host.QuestGiverCancel()
		}
		rt.CloseQuestDialog()
		return 0
	})
	reg("CloseQuest", func(L *lua.LState) int {
		if host := rt.questHost(); host != nil && dialog().kind != questDialogNone {
			_ = host.QuestGiverCancel()
		}
		rt.CloseQuestDialog()
		return 0
	})
	reg("CompleteQuest", func(L *lua.LState) int {
		if host := rt.questHost(); host != nil && dialog().id != 0 {
			_ = host.QuestGiverComplete(dialog().giver, dialog().id)
		}
		return 0
	})
	reg("GetQuestReward", func(L *lua.LState) int {
		if host := rt.questHost(); host != nil && dialog().id != 0 {
			_ = host.QuestGiverChoose(dialog().giver, dialog().id, uint32(L.OptInt(1, 0)))
		}
		rt.CloseQuestDialog()
		return 0
	})
	reg("QuestChooseRewardError", func(L *lua.LState) int {
		rt.ShowInfoMessage(rt.globalString("ERR_QUEST_MUST_CHOOSE", "You must choose a reward."))
		return 0
	})
	reg("GetQuestBackgroundMaterial", func(L *lua.LState) int { return 0 })
	reg("GetMaterialTextColors", func(L *lua.LState) int { return 0 })
	// The quest log.
	reg("GetNumQuestLogEntries", func(L *lua.LState) int {
		rows := rt.questRows()
		quests := 0
		for _, slot := range rt.quests.slots {
			if slot.ID != 0 {
				quests++
			}
		}
		L.Push(lua.LNumber(len(rows)))
		L.Push(lua.LNumber(quests))
		return 2
	})
	reg("GetQuestLogTitle", func(L *lua.LState) int {
		rows := rt.questRows()
		index := L.CheckInt(1)
		if index < 1 || index > len(rows) {
			return 0
		}
		row := rows[index-1]
		if row.slot < 0 {
			L.Push(lua.LString(row.header))
			for i := 0; i < 2; i++ {
				L.Push(lua.LNumber(0))
			}
			L.Push(lua.LNumber(0))
			L.Push(lua.LTrue)
			L.Push(lua.LBool(rt.quests.collapsed[row.header]))
			return 6
		}
		slot := rt.quests.slots[row.slot]
		info := rt.quests.info[slot.ID]
		L.Push(lua.LString(rt.questTitle(slot.ID)))
		L.Push(lua.LNumber(info.Level))
		L.Push(lua.LNil)
		L.Push(lua.LNumber(info.SuggestedSize))
		L.Push(lua.LNil)
		L.Push(lua.LNil)
		switch {
		case slot.State&questStateFailed != 0:
			L.Push(lua.LNumber(-1))
		case rt.questComplete(slot):
			L.Push(lua.LNumber(1))
		default:
			L.Push(lua.LNil)
		}
		L.Push(lua.LNil)
		L.Push(lua.LNumber(slot.ID))
		return 9
	})
	reg("SelectQuestLogEntry", func(L *lua.LState) int {
		rt.quests.selected = L.CheckInt(1)
		rt.FireEvent("QUEST_LOG_UPDATE")
		return 0
	})
	reg("GetQuestLogSelection", func(L *lua.LState) int { L.Push(lua.LNumber(rt.quests.selected)); return 1 })
	selectedSlot := func() (QuestLogSlot, bool) {
		rows := rt.questRows()
		if rt.quests.selected < 1 || rt.quests.selected > len(rows) || rows[rt.quests.selected-1].slot < 0 {
			return QuestLogSlot{}, false
		}
		return rt.quests.slots[rows[rt.quests.selected-1].slot], true
	}
	reg("GetQuestLogQuestText", func(L *lua.LState) int {
		slot, ok := selectedSlot()
		if !ok {
			return 0
		}
		info := rt.quests.info[slot.ID]
		L.Push(lua.LString(rt.expandNPCText(info.Details)))
		L.Push(lua.LString(rt.expandNPCText(info.Objectives)))
		return 2
	})
	reg("GetQuestLogCompletionText", func(L *lua.LState) int {
		if slot, ok := selectedSlot(); ok {
			L.Push(lua.LString(rt.quests.info[slot.ID].CompletedText))
			return 1
		}
		return 0
	})
	reg("GetNumQuestLeaderBoards", func(L *lua.LState) int {
		slot, ok := selectedSlot()
		if index := L.OptInt(1, 0); index > 0 {
			rows := rt.questRows()
			if index <= len(rows) && rows[index-1].slot >= 0 {
				slot, ok = rt.quests.slots[rows[index-1].slot], true
			}
		}
		count := 0
		if ok {
			count = len(rt.questObjectives(slot))
		}
		L.Push(lua.LNumber(count))
		return 1
	})
	reg("GetQuestLogLeaderBoard", func(L *lua.LState) int {
		slot, ok := selectedSlot()
		if quest := L.OptInt(2, 0); quest > 0 {
			rows := rt.questRows()
			if quest <= len(rows) && rows[quest-1].slot >= 0 {
				slot, ok = rt.quests.slots[rows[quest-1].slot], true
			}
		}
		if !ok {
			return 0
		}
		objectives := rt.questObjectives(slot)
		index := L.CheckInt(1)
		if index < 1 || index > len(objectives) {
			return 0
		}
		L.Push(lua.LString(objectives[index-1].text))
		L.Push(lua.LString(objectives[index-1].kind))
		L.Push(lua.LBool(objectives[index-1].finished))
		return 3
	})
	reg("GetQuestLogRewardMoney", func(L *lua.LState) int {
		if slot, ok := selectedSlot(); ok {
			L.Push(lua.LNumber(rt.quests.info[slot.ID].RewardMoney))
			return 1
		}
		L.Push(lua.LNumber(0))
		return 1
	})
	reg("GetQuestLogRequiredMoney", func(L *lua.LState) int { L.Push(lua.LNumber(0)); return 1 })
	reg("GetQuestLogRewardXP", func(L *lua.LState) int { L.Push(lua.LNumber(0)); return 1 })
	reg("GetQuestLogRewardSpell", func(L *lua.LState) int { return 0 })
	reg("GetQuestLogTimeLeft", func(L *lua.LState) int { return 0 })
	reg("GetQuestLogGroupNum", func(L *lua.LState) int {
		if slot, ok := selectedSlot(); ok {
			L.Push(lua.LNumber(rt.quests.info[slot.ID].SuggestedSize))
			return 1
		}
		return 0
	})
	reg("GetQuestLogIsAutoComplete", func(L *lua.LState) int { L.Push(lua.LFalse); return 1 })
	reg("GetQuestLogSpecialItemInfo", func(L *lua.LState) int { return 0 })
	reg("GetNumQuestLogRewards", func(L *lua.LState) int {
		if slot, ok := selectedSlot(); ok {
			count := 0
			for _, item := range rt.quests.info[slot.ID].Rewards {
				if item.Entry != 0 {
					count++
				}
			}
			L.Push(lua.LNumber(count))
			return 1
		}
		L.Push(lua.LNumber(0))
		return 1
	})
	reg("GetNumQuestLogChoices", func(L *lua.LState) int {
		if slot, ok := selectedSlot(); ok {
			count := 0
			for _, item := range rt.quests.info[slot.ID].Choices {
				if item.Entry != 0 {
					count++
				}
			}
			L.Push(lua.LNumber(count))
			return 1
		}
		L.Push(lua.LNumber(0))
		return 1
	})
	logItem := func(choice bool) lua.LGFunction {
		return func(L *lua.LState) int {
			slot, ok := selectedSlot()
			if !ok {
				return 0
			}
			info := rt.quests.info[slot.ID]
			var list []world.QuestItem
			source := info.Rewards[:]
			if choice {
				source = info.Choices[:]
			}
			for _, item := range source {
				if item.Entry != 0 {
					list = append(list, item)
				}
			}
			index := L.CheckInt(1)
			if index < 1 || index > len(list) {
				return 0
			}
			item := list[index-1]
			name, quality := "", 1
			if template := rt.itemTemplateIfKnown(item.Entry); template != nil {
				name, quality = template.Name, int(template.Quality)
			}
			L.Push(lua.LString(name))
			L.Push(lua.LString(rt.itemIcon(item.Entry)))
			L.Push(lua.LNumber(item.Count))
			L.Push(lua.LNumber(quality))
			L.Push(lua.LTrue)
			return 5
		}
	}
	reg("GetQuestLogRewardInfo", logItem(false))
	reg("GetQuestLogChoiceInfo", logItem(true))
	reg("GetQuestLogItemLink", func(L *lua.LState) int {
		slot, ok := selectedSlot()
		if !ok {
			return 0
		}
		info := rt.quests.info[slot.ID]
		source := info.Rewards[:]
		if L.CheckString(1) == "choice" {
			source = info.Choices[:]
		}
		var list []world.QuestItem
		for _, item := range source {
			if item.Entry != 0 {
				list = append(list, item)
			}
		}
		if index := L.CheckInt(2); index >= 1 && index <= len(list) {
			L.Push(lua.LString(rt.itemLinkFor(list[index-1].Entry, 0)))
			return 1
		}
		return 0
	})
	collapse := func(L *lua.LState, collapsed bool) int {
		rows := rt.questRows()
		index := L.CheckInt(1)
		if rt.quests.collapsed == nil {
			rt.quests.collapsed = make(map[string]bool)
		}
		if index == 0 {
			for _, row := range rows {
				if row.slot < 0 {
					rt.quests.collapsed[row.header] = collapsed
				}
			}
		} else if index >= 1 && index <= len(rows) && rows[index-1].slot < 0 {
			rt.quests.collapsed[rows[index-1].header] = collapsed
		}
		rt.FireEvent("QUEST_LOG_UPDATE")
		return 0
	}
	reg("ExpandQuestHeader", func(L *lua.LState) int { return collapse(L, false) })
	reg("CollapseQuestHeader", func(L *lua.LState) int { return collapse(L, true) })
	reg("SetAbandonQuest", func(L *lua.LState) int { return 0 })
	reg("GetAbandonQuestName", func(L *lua.LState) int {
		if slot, ok := selectedSlot(); ok {
			L.Push(lua.LString(rt.questTitle(slot.ID)))
			return 1
		}
		L.Push(lua.LString(""))
		return 1
	})
	reg("GetAbandonQuestItems", func(L *lua.LState) int { return 0 })
	reg("AbandonQuest", func(L *lua.LState) int {
		rows := rt.questRows()
		if rt.quests.selected >= 1 && rt.quests.selected <= len(rows) && rows[rt.quests.selected-1].slot >= 0 {
			if host := rt.questHost(); host != nil {
				_ = host.AbandonQuest(uint8(rows[rt.quests.selected-1].slot))
			}
		}
		return 0
	})
	reg("IsCurrentQuestFailed", func(L *lua.LState) int {
		slot, ok := selectedSlot()
		L.Push(lua.LBool(ok && slot.State&questStateFailed != 0))
		return 1
	})
	reg("GetQuestLogPushable", func(L *lua.LState) int { L.Push(lua.LFalse); return 1 })
	reg("QuestLogPushQuest", func(L *lua.LState) int { return 0 })
	// The objectives tracker.
	reg("IsQuestWatched", func(L *lua.LState) int {
		rows := rt.questRows()
		index := L.CheckInt(1)
		if index >= 1 && index <= len(rows) && rows[index-1].slot >= 0 {
			L.Push(lua.LBool(rt.quests.watched[rt.quests.slots[rows[index-1].slot].ID]))
			return 1
		}
		L.Push(lua.LFalse)
		return 1
	})
	watch := func(on bool) lua.LGFunction {
		return func(L *lua.LState) int {
			rows := rt.questRows()
			index := L.CheckInt(1)
			if index >= 1 && index <= len(rows) && rows[index-1].slot >= 0 {
				if rt.quests.watched == nil {
					rt.quests.watched = make(map[uint32]bool)
				}
				rt.quests.watched[rt.quests.slots[rows[index-1].slot].ID] = on
				rt.FireEvent("QUEST_WATCH_UPDATE", lua.LNumber(index))
			}
			return 0
		}
	}
	reg("AddQuestWatch", watch(true))
	reg("RemoveQuestWatch", watch(false))
	reg("GetNumQuestWatches", func(L *lua.LState) int {
		count := 0
		for _, watched := range rt.quests.watched {
			if watched {
				count++
			}
		}
		L.Push(lua.LNumber(count))
		return 1
	})
	reg("GetQuestIndexForWatch", func(L *lua.LState) int {
		want := L.CheckInt(1)
		count := 0
		for index, row := range rt.questRows() {
			if row.slot >= 0 && rt.quests.watched[rt.quests.slots[row.slot].ID] {
				count++
				if count == want {
					L.Push(lua.LNumber(index + 1))
					return 1
				}
			}
		}
		return 0
	})
}

// greetingQuests are the quests of the quest giver's window, the ones the
// character holds (active) or can take.
func (rt *Runtime) greetingQuests(active bool) []world.GossipQuest {
	var list []world.GossipQuest
	for _, quest := range rt.quests.dialog.quests {
		if (quest.Icon == questIconActive) == active {
			list = append(list, quest)
		}
	}
	return list
}

var _ = strings.TrimSpace

// GlobalString is a string of the interface's global strings, or a fallback.
func (rt *Runtime) GlobalString(name, fallback string) string { return rt.globalString(name, fallback) }

// ShowInfoMessage puts a line of information in the middle of the screen
// (quest progress, discoveries).
func (rt *Runtime) ShowInfoMessage(text string) {
	rt.FireEvent("UI_INFO_MESSAGE", lua.LString(text))
}
