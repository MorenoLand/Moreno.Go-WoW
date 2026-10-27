package world

import (
	"encoding/binary"
	"fmt"
)

const (
	ClientQuestQuery          ClientOpcode = 0x005C
	ClientQuestGiverStatusQ   ClientOpcode = 0x0182
	ClientQuestGiverHello     ClientOpcode = 0x0184
	ClientQuestGiverQuery     ClientOpcode = 0x0186
	ClientQuestGiverAccept    ClientOpcode = 0x0189
	ClientQuestGiverComplete  ClientOpcode = 0x018A
	ClientQuestGiverReqReward ClientOpcode = 0x018C
	ClientQuestGiverChoose    ClientOpcode = 0x018E
	ClientQuestGiverCancel    ClientOpcode = 0x0190
	ClientQuestLogRemove      ClientOpcode = 0x0194
	ClientQuestConfirmAccept  ClientOpcode = 0x019B

	QuestQueryResponse       uint16 = 0x005D
	QuestGiverStatus         uint16 = 0x0183
	QuestGiverQuestList      uint16 = 0x0185
	QuestGiverQuestDetails   uint16 = 0x0188
	QuestGiverRequestItems   uint16 = 0x018B
	QuestGiverOfferReward    uint16 = 0x018D
	QuestGiverQuestInvalid   uint16 = 0x018F
	QuestGiverQuestComplete  uint16 = 0x0191
	QuestGiverQuestFailed    uint16 = 0x0192
	QuestLogFull             uint16 = 0x0195
	QuestUpdateFailed        uint16 = 0x0196
	QuestUpdateFailedTimer   uint16 = 0x0197
	QuestUpdateComplete      uint16 = 0x0198
	QuestUpdateAddKill       uint16 = 0x0199
	QuestUpdateAddItem       uint16 = 0x019A
	QuestConfirmAcceptPacket uint16 = 0x019C
	QuestForceRemove         uint16 = 0x021E
)

// QuestItem is an item a quest rewards or asks for.
type QuestItem struct {
	Entry, Count, DisplayID uint32
}

// QuestRewards is what a quest gives, as the details and the reward windows list it.
type QuestRewards struct {
	Choices        []QuestItem
	Items          []QuestItem
	Money          uint32
	XPDifficulty   uint32
	Honor          uint32
	DisplaySpell   uint32
	Spell          int32
	TitleID        uint32
	Talents        uint32
	ArenaPoints    uint32
	FactionFlags   uint32
	FactionIDs     [5]uint32
	FactionValues  [5]int32
	FactionBonuses [5]int32
}

// QuestDetails is SMSG_QUESTGIVER_QUEST_DETAILS.
type QuestDetails struct {
	Giver, InformUnit uint64
	ID                uint32
	Title             string
	Details           string
	Objectives        string
	AutoLaunched      bool
	Flags             uint32
	SuggestedGroup    uint32
	Rewards           QuestRewards
}

func readQuestItems(r *Reader, count uint32) ([]QuestItem, error) {
	if count > 32 {
		return nil, fmt.Errorf("%s has %d quest items", r.what, count)
	}
	items := make([]QuestItem, 0, count)
	for index := uint32(0); index < count; index++ {
		var item QuestItem
		var err error
		if item.Entry, err = r.U32(); err != nil {
			return nil, err
		}
		if item.Count, err = r.U32(); err != nil {
			return nil, err
		}
		if item.DisplayID, err = r.U32(); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, nil
}

// readQuestRewards reads the reward block the details and the offer share;
// offer is set for the form that carries an extra unused number.
func readQuestRewards(r *Reader, offer bool) (QuestRewards, error) {
	var rewards QuestRewards
	count, err := r.U32()
	if err != nil {
		return rewards, err
	}
	if rewards.Choices, err = readQuestItems(r, count); err != nil {
		return rewards, err
	}
	if count, err = r.U32(); err != nil {
		return rewards, err
	}
	if rewards.Items, err = readQuestItems(r, count); err != nil {
		return rewards, err
	}
	if rewards.Money, err = r.U32(); err != nil {
		return rewards, err
	}
	if rewards.XPDifficulty, err = r.U32(); err != nil {
		return rewards, err
	}
	if rewards.Honor, err = r.U32(); err != nil {
		return rewards, err
	}
	if _, err = r.F32(); err != nil {
		return rewards, err
	}
	if offer {
		if _, err = r.U32(); err != nil {
			return rewards, err
		}
	}
	if rewards.DisplaySpell, err = r.U32(); err != nil {
		return rewards, err
	}
	spell, err := r.U32()
	if err != nil {
		return rewards, err
	}
	rewards.Spell = int32(spell)
	if rewards.TitleID, err = r.U32(); err != nil {
		return rewards, err
	}
	if rewards.Talents, err = r.U32(); err != nil {
		return rewards, err
	}
	if rewards.ArenaPoints, err = r.U32(); err != nil {
		return rewards, err
	}
	if rewards.FactionFlags, err = r.U32(); err != nil {
		return rewards, err
	}
	for index := range rewards.FactionIDs {
		if rewards.FactionIDs[index], err = r.U32(); err != nil {
			return rewards, err
		}
	}
	for index := range rewards.FactionValues {
		value, err := r.U32()
		if err != nil {
			return rewards, err
		}
		rewards.FactionValues[index] = int32(value)
	}
	for index := range rewards.FactionBonuses {
		value, err := r.U32()
		if err != nil {
			return rewards, err
		}
		rewards.FactionBonuses[index] = int32(value)
	}
	return rewards, nil
}

// ParseQuestDetails reads SMSG_QUESTGIVER_QUEST_DETAILS.
func ParseQuestDetails(body []byte) (QuestDetails, error) {
	r := NewReader(body, "SMSG_QUESTGIVER_QUEST_DETAILS")
	var quest QuestDetails
	var err error
	if quest.Giver, err = r.U64(); err != nil {
		return quest, err
	}
	if quest.InformUnit, err = r.U64(); err != nil {
		return quest, err
	}
	if quest.ID, err = r.U32(); err != nil {
		return quest, err
	}
	if quest.Title, err = r.CString(); err != nil {
		return quest, err
	}
	if quest.Details, err = r.CString(); err != nil {
		return quest, err
	}
	if quest.Objectives, err = r.CString(); err != nil {
		return quest, err
	}
	auto, err := r.U8()
	if err != nil {
		return quest, err
	}
	quest.AutoLaunched = auto != 0
	if quest.Flags, err = r.U32(); err != nil {
		return quest, err
	}
	if quest.SuggestedGroup, err = r.U32(); err != nil {
		return quest, err
	}
	if _, err = r.U8(); err != nil {
		return quest, err
	}
	quest.Rewards, err = readQuestRewards(r, false)
	return quest, err
}

// QuestOffer is SMSG_QUESTGIVER_OFFER_REWARD: the reward window.
type QuestOffer struct {
	Giver          uint64
	ID             uint32
	Title          string
	RewardText     string
	AutoLaunched   bool
	Flags          uint32
	SuggestedGroup uint32
	Rewards        QuestRewards
}

// ParseQuestOfferReward reads SMSG_QUESTGIVER_OFFER_REWARD.
func ParseQuestOfferReward(body []byte) (QuestOffer, error) {
	r := NewReader(body, "SMSG_QUESTGIVER_OFFER_REWARD")
	var offer QuestOffer
	var err error
	if offer.Giver, err = r.U64(); err != nil {
		return offer, err
	}
	if offer.ID, err = r.U32(); err != nil {
		return offer, err
	}
	if offer.Title, err = r.CString(); err != nil {
		return offer, err
	}
	if offer.RewardText, err = r.CString(); err != nil {
		return offer, err
	}
	auto, err := r.U8()
	if err != nil {
		return offer, err
	}
	offer.AutoLaunched = auto != 0
	if offer.Flags, err = r.U32(); err != nil {
		return offer, err
	}
	if offer.SuggestedGroup, err = r.U32(); err != nil {
		return offer, err
	}
	emotes, err := r.U32()
	if err != nil {
		return offer, err
	}
	if emotes > 16 {
		return offer, fmt.Errorf("SMSG_QUESTGIVER_OFFER_REWARD has %d emotes", emotes)
	}
	if err = r.Skip(int(emotes) * 8); err != nil {
		return offer, err
	}
	offer.Rewards, err = readQuestRewards(r, true)
	return offer, err
}

// QuestProgress is SMSG_QUESTGIVER_REQUEST_ITEMS: what the quest still asks for.
type QuestProgress struct {
	Giver       uint64
	ID          uint32
	Title       string
	Text        string
	CloseCancel bool
	Flags       uint32
	Money       uint32
	Items       []QuestItem
	// CanComplete is read from the flags the packet ends with.
	CanComplete bool
}

// ParseQuestRequestItems reads SMSG_QUESTGIVER_REQUEST_ITEMS.
func ParseQuestRequestItems(body []byte) (QuestProgress, error) {
	r := NewReader(body, "SMSG_QUESTGIVER_REQUEST_ITEMS")
	var quest QuestProgress
	var err error
	if quest.Giver, err = r.U64(); err != nil {
		return quest, err
	}
	if quest.ID, err = r.U32(); err != nil {
		return quest, err
	}
	if quest.Title, err = r.CString(); err != nil {
		return quest, err
	}
	if quest.Text, err = r.CString(); err != nil {
		return quest, err
	}
	if err = r.Skip(4 + 4); err != nil {
		return quest, err
	}
	close, err := r.U32()
	if err != nil {
		return quest, err
	}
	quest.CloseCancel = close != 0
	if quest.Flags, err = r.U32(); err != nil {
		return quest, err
	}
	if err = r.Skip(4); err != nil {
		return quest, err
	}
	if quest.Money, err = r.U32(); err != nil {
		return quest, err
	}
	count, err := r.U32()
	if err != nil {
		return quest, err
	}
	if count > 16 {
		return quest, fmt.Errorf("SMSG_QUESTGIVER_REQUEST_ITEMS has %d items", count)
	}
	for index := uint32(0); index < count; index++ {
		var item QuestItem
		if item.Entry, err = r.U32(); err != nil {
			return quest, err
		}
		if item.Count, err = r.U32(); err != nil {
			return quest, err
		}
		if item.DisplayID, err = r.U32(); err != nil {
			return quest, err
		}
		quest.Items = append(quest.Items, item)
	}
	if mask, err := r.U32(); err == nil {
		quest.CanComplete = mask&0x3 == 0x3
	}
	return quest, nil
}

// QuestList is SMSG_QUESTGIVER_QUEST_LIST: the quests of a creature with no gossip.
type QuestList struct {
	Giver    uint64
	Greeting string
	Quests   []GossipQuest
}

// ParseQuestGiverQuestList reads SMSG_QUESTGIVER_QUEST_LIST.
func ParseQuestGiverQuestList(body []byte) (QuestList, error) {
	r := NewReader(body, "SMSG_QUESTGIVER_QUEST_LIST")
	var list QuestList
	var err error
	if list.Giver, err = r.U64(); err != nil {
		return list, err
	}
	if list.Greeting, err = r.CString(); err != nil {
		return list, err
	}
	if err = r.Skip(8); err != nil {
		return list, err
	}
	count, err := r.U8()
	if err != nil {
		return list, err
	}
	for index := 0; index < int(count); index++ {
		quest, err := readGossipQuest(r)
		if err != nil {
			return list, err
		}
		list.Quests = append(list.Quests, quest)
	}
	return list, nil
}

// QuestInfo is SMSG_QUEST_QUERY_RESPONSE: what the quest log and tracker
// show of a quest.
type QuestInfo struct {
	ID, Method          uint32
	Level               int32
	MinLevel            uint32
	Sort                int32
	Type, SuggestedSize uint32
	Flags               uint32
	RewardMoney         uint32
	Title               string
	Objectives          string
	Details             string
	AreaDescription     string
	CompletedText       string
	// Targets are the creatures or objects to kill or use; a negative entry is
	// an object.
	Targets [4]QuestTarget
	Items   [6]QuestItem
	// Text is the line each target is described by.
	TargetText [4]string
	Choices    [6]QuestItem
	Rewards    [4]QuestItem
	// XPRow is the column of the experience table the reward is read from.
	XPRow uint32
	// RewardSpell is the spell the quest teaches or casts for the player.
	RewardSpell    uint32
	RewardHonor    int32
	RewardTitle    uint32
	RewardTalents  uint32
	RewardArena    uint32
	RewardFactions [5]QuestFactionReward
	}

	// QuestFactionReward is the reputation a quest gives with a faction.
	type QuestFactionReward struct {
	ID    uint32
	Value int32
	}

// QuestTarget is one kill or use objective.
type QuestTarget struct {
	Entry int32
	Count uint32
}

// ParseQuestQueryResponse reads SMSG_QUEST_QUERY_RESPONSE.
func ParseQuestQueryResponse(body []byte) (QuestInfo, error) {
	r := NewReader(body, "SMSG_QUEST_QUERY_RESPONSE")
	var info QuestInfo
	var err error
	if info.ID, err = r.U32(); err != nil {
		return info, err
	}
	if info.Method, err = r.U32(); err != nil {
		return info, err
	}
	level, err := r.U32()
	if err != nil {
		return info, err
	}
	info.Level = int32(level)
	if info.MinLevel, err = r.U32(); err != nil {
		return info, err
	}
	sort, err := r.U32()
	if err != nil {
		return info, err
	}
	info.Sort = int32(sort)
	if info.Type, err = r.U32(); err != nil {
		return info, err
	}
	if info.SuggestedSize, err = r.U32(); err != nil {
		return info, err
	}
	// Two required reputation pairs and the next quest of the chain.
	if err = r.Skip(4*4 + 4); err != nil {
	return info, err
	}
	if info.XPRow, err = r.U32(); err != nil {
	return info, err
	}
	if info.RewardMoney, err = r.U32(); err != nil {
		return info, err
	}
	// Bonus money, then the display spell, the spell, the honor, the honor
	// multiplier and the start item.
	if err = r.Skip(4); err != nil {
	return info, err
	}
	if info.RewardSpell, err = r.U32(); err != nil {
	return info, err
	}
	if err = r.Skip(4); err != nil {
	return info, err
	}
	honor, err := r.U32()
	if err != nil {
	return info, err
	}
	info.RewardHonor = int32(honor)
	if err = r.Skip(2 * 4); err != nil {
	return info, err
	}
	if info.Flags, err = r.U32(); err != nil {
		return info, err
	}
	// Title, players slain, talents, arena points, faction flags.
	if info.RewardTitle, err = r.U32(); err != nil {
	return info, err
	}
	if err = r.Skip(4); err != nil {
	return info, err
	}
	if info.RewardTalents, err = r.U32(); err != nil {
	return info, err
	}
	if info.RewardArena, err = r.U32(); err != nil {
	return info, err
	}
	if err = r.Skip(4); err != nil {
	return info, err
	}
	for index := range info.Rewards {
		if info.Rewards[index].Entry, err = r.U32(); err != nil {
			return info, err
		}
		if info.Rewards[index].Count, err = r.U32(); err != nil {
			return info, err
		}
	}
	for index := range info.Choices {
		if info.Choices[index].Entry, err = r.U32(); err != nil {
			return info, err
		}
		if info.Choices[index].Count, err = r.U32(); err != nil {
			return info, err
		}
	}
	// Reward factions and their values, then the overrides and the point of
	// interest.
	for index := range info.RewardFactions {
	if info.RewardFactions[index].ID, err = r.U32(); err != nil {
	return info, err
	}
	}
	for index := range info.RewardFactions {
	value, err := r.U32()
	if err != nil {
	return info, err
	}
	info.RewardFactions[index].Value = int32(value)
	}
	if err = r.Skip(5*4 + 4*4); err != nil {
	return info, err
	}
	if info.Title, err = r.CString(); err != nil {
		return info, err
	}
	if info.Objectives, err = r.CString(); err != nil {
		return info, err
	}
	if info.Details, err = r.CString(); err != nil {
		return info, err
	}
	if info.AreaDescription, err = r.CString(); err != nil {
		return info, err
	}
	if info.CompletedText, err = r.CString(); err != nil {
		return info, err
	}
	for index := range info.Targets {
		entry, err := r.U32()
		if err != nil {
			return info, err
		}
		if entry&0x80000000 != 0 {
			info.Targets[index].Entry = -int32(entry & 0x7FFFFFFF)
		} else {
			info.Targets[index].Entry = int32(entry)
		}
		if info.Targets[index].Count, err = r.U32(); err != nil {
			return info, err
		}
		if err = r.Skip(8); err != nil {
			return info, err
		}
	}
	for index := range info.Items {
		if info.Items[index].Entry, err = r.U32(); err != nil {
			return info, err
		}
		if info.Items[index].Count, err = r.U32(); err != nil {
			return info, err
		}
	}
	for index := range info.TargetText {
		if info.TargetText[index], err = r.CString(); err != nil {
			return info, err
		}
	}
	return info, nil
}

// QuestKill is SMSG_QUESTUPDATE_ADD_KILL: progress on a kill objective.
type QuestKill struct {
	Quest    uint32
	Entry    uint32
	Count    uint32
	Needed   uint32
	Creature uint64
}

// ParseQuestUpdateAddKill reads SMSG_QUESTUPDATE_ADD_KILL.
func ParseQuestUpdateAddKill(body []byte) (QuestKill, error) {
	r := NewReader(body, "SMSG_QUESTUPDATE_ADD_KILL")
	var kill QuestKill
	var err error
	if kill.Quest, err = r.U32(); err != nil {
		return kill, err
	}
	if kill.Entry, err = r.U32(); err != nil {
		return kill, err
	}
	if kill.Count, err = r.U32(); err != nil {
		return kill, err
	}
	if kill.Needed, err = r.U32(); err != nil {
		return kill, err
	}
	kill.Creature, err = r.U64()
	return kill, err
}

// BuildQuestID is the body of the requests that name one quest (CMSG_QUEST_QUERY,
// CMSG_QUESTLOG_REMOVE_QUEST takes a slot instead).
func BuildQuestID(id uint32) []byte {
	body := make([]byte, 4)
	binary.LittleEndian.PutUint32(body, id)
	return body
}

// BuildQuestGiverQuest is the body of CMSG_QUESTGIVER_QUERY_QUEST and
// CMSG_QUESTGIVER_ACCEPT_QUEST: the creature and a quest (and a flag for the query).
func BuildQuestGiverQuest(guid uint64, quest uint32, flag bool) []byte {
	body := make([]byte, 13)
	binary.LittleEndian.PutUint64(body, guid)
	binary.LittleEndian.PutUint32(body[8:], quest)
	if flag {
		body[12] = 1
	}
	return body
}

// BuildQuestGiverChoose is the body of CMSG_QUESTGIVER_CHOOSE_REWARD.
func BuildQuestGiverChoose(guid uint64, quest, choice uint32) []byte {
	body := make([]byte, 16)
	binary.LittleEndian.PutUint64(body, guid)
	binary.LittleEndian.PutUint32(body[8:], quest)
	binary.LittleEndian.PutUint32(body[12:], choice)
	return body
}

// BuildQuestGiverRequest is the body of CMSG_QUESTGIVER_COMPLETE_QUEST and
// CMSG_QUESTGIVER_REQUEST_REWARD: the creature and the quest.
func BuildQuestGiverRequest(guid uint64, quest uint32) []byte {
	body := make([]byte, 12)
	binary.LittleEndian.PutUint64(body, guid)
	binary.LittleEndian.PutUint32(body[8:], quest)
	return body
}

const (
	ClientQuestGiverStatusMultiple ClientOpcode = 0x0417
	QuestGiverStatusMultiple       uint16       = 0x0418
)

// Quest giver statuses (QuestGiverStatus).
const (
	QuestStatusLowLevelAvailable = 2
	QuestStatusIncomplete        = 5
	QuestStatusRewardRep         = 6
	QuestStatusAvailable         = 8
	QuestStatusReward2           = 9
	QuestStatusReward            = 10
)

// ParseQuestGiverStatus reads SMSG_QUESTGIVER_STATUS.
func ParseQuestGiverStatus(body []byte) (uint64, uint8, error) {
	r := NewReader(body, "SMSG_QUESTGIVER_STATUS")
	guid, err := r.U64()
	if err != nil {
		return 0, 0, err
	}
	status, err := r.U8()
	return guid, status, err
}

// ParseQuestGiverStatusMultiple reads SMSG_QUESTGIVER_STATUS_MULTIPLE.
func ParseQuestGiverStatusMultiple(body []byte) (map[uint64]uint8, error) {
	r := NewReader(body, "SMSG_QUESTGIVER_STATUS_MULTIPLE")
	count, err := r.U32()
	if err != nil {
		return nil, err
	}
	if count > 4096 {
		return nil, fmt.Errorf("SMSG_QUESTGIVER_STATUS_MULTIPLE has %d entries", count)
	}
	statuses := make(map[uint64]uint8, count)
	for index := uint32(0); index < count; index++ {
		guid, err := r.U64()
		if err != nil {
			return statuses, err
		}
		status, err := r.U8()
		if err != nil {
			return statuses, err
		}
		statuses[guid] = status
	}
	return statuses, nil
}
