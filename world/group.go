package world

import (
	"encoding/binary"
	"math"
)

// Group packets.
const (
	GroupInvite            uint16 = 0x006F
	GroupCancel            uint16 = 0x0071
	GroupDecline           uint16 = 0x0074
	GroupUninvite          uint16 = 0x0077
	GroupSetLeaderResponse uint16 = 0x0079
	GroupDestroyed         uint16 = 0x007C
	GroupList              uint16 = 0x007D
	PartyMemberStats       uint16 = 0x007E
	PartyCommandResult     uint16 = 0x007F
	MinimapPing            uint16 = 0x01D5
	RandomRoll             uint16 = 0x01FB
	PartyMemberStatsFull   uint16 = 0x02F2
	RaidTargetUpdate       uint16 = 0x0321
	RaidReadyCheck         uint16 = 0x0322
	RaidReadyCheckConfirm  uint16 = 0x03AE
	RaidReadyCheckFinished uint16 = 0x03C6
	PartyAssignment        uint16 = 0x038E
	LootAllPassed          uint16 = 0x029E
	LootRollWon            uint16 = 0x029F
	LootStartRoll          uint16 = 0x02A1
	LootRollPacket         uint16 = 0x02A2
	LootMasterList         uint16 = 0x02A4
	RaidInstanceInfo       uint16 = 0x02CC
	InstanceReset          uint16 = 0x031E
	InstanceResetFailed    uint16 = 0x031F
	RaidInstanceMessage    uint16 = 0x02FA
	InstanceSaveCreated    uint16 = 0x02CB
	DungeonDifficulty      uint16 = 0x0329
	RaidDifficulty         uint16 = 0x04EB
	InstanceDifficulty     uint16 = 0x033B
	RaidGroupOnly          uint16 = 0x0286

	ClientGroupInvite       ClientOpcode = 0x006E
	ClientGroupAccept       ClientOpcode = 0x0072
	ClientGroupDecline      ClientOpcode = 0x0073
	ClientGroupUninvite     ClientOpcode = 0x0075
	ClientGroupUninviteGUID ClientOpcode = 0x0076
	ClientGroupSetLeader    ClientOpcode = 0x0078
	ClientLootMethod        ClientOpcode = 0x007A
	ClientGroupDisband      ClientOpcode = 0x007B
	ClientMinimapPing       ClientOpcode = 0x01D5
	ClientRandomRoll        ClientOpcode = 0x01FB
	ClientChangeSubGroup    ClientOpcode = 0x027E
	ClientRequestMemberStat ClientOpcode = 0x027F
	ClientSwapSubGroup      ClientOpcode = 0x0280
	ClientRaidConvert       ClientOpcode = 0x028E
	ClientAssistantLeader   ClientOpcode = 0x028F
	ClientLootRoll          ClientOpcode = 0x02A0
	ClientLootMasterGive    ClientOpcode = 0x02A3
	ClientRequestRaidInfo   ClientOpcode = 0x02CD
	ClientResetInstances    ClientOpcode = 0x031D
	ClientRaidTargetUpdate  ClientOpcode = 0x0321
	ClientRaidReadyCheck    ClientOpcode = 0x0322
	ClientPartyAssignment   ClientOpcode = 0x038E
	ClientReadyCheckDone    ClientOpcode = 0x03C6
	ClientSetDungeonDiff    ClientOpcode = 0x0329
	ClientSetRaidDiff       ClientOpcode = 0x04EB
	ClientOptOutOfLoot      ClientOpcode = 0x0409
)

// Group type flags.
const (
	GroupTypeBattleground uint8 = 0x01
	GroupTypeRaid         uint8 = 0x02
	GroupTypeLFG          uint8 = 0x08
)

// Member flags.
const (
	MemberAssistant  uint8 = 0x01
	MemberMainTank   uint8 = 0x02
	MemberMainAssist uint8 = 0x04
)

// Member online status bits.
const (
	MemberStatusOnline uint16 = 0x0001
	MemberStatusPVP    uint16 = 0x0002
	MemberStatusDead   uint16 = 0x0004
	MemberStatusGhost  uint16 = 0x0008
	MemberStatusFFA    uint16 = 0x0010
	MemberStatusAFK    uint16 = 0x0040
	MemberStatusDND    uint16 = 0x0080
)

// Loot methods.
const (
	LootFreeForAll     uint8 = 0
	LootRoundRobin     uint8 = 1
	LootMaster         uint8 = 2
	LootGroup          uint8 = 3
	LootNeedBeforeGree uint8 = 4
)

// Group update flags of a member's stats packet.
const (
	GroupUpdateStatus     uint32 = 0x00000001
	GroupUpdateCurHP      uint32 = 0x00000002
	GroupUpdateMaxHP      uint32 = 0x00000004
	GroupUpdatePowerType  uint32 = 0x00000008
	GroupUpdateCurPower   uint32 = 0x00000010
	GroupUpdateMaxPower   uint32 = 0x00000020
	GroupUpdateLevel      uint32 = 0x00000040
	GroupUpdateZone       uint32 = 0x00000080
	GroupUpdatePosition   uint32 = 0x00000100
	GroupUpdateAuras      uint32 = 0x00000200
	GroupUpdatePetGUID    uint32 = 0x00000400
	GroupUpdatePetName    uint32 = 0x00000800
	GroupUpdatePetModel   uint32 = 0x00001000
	GroupUpdatePetCurHP   uint32 = 0x00002000
	GroupUpdatePetMaxHP   uint32 = 0x00004000
	GroupUpdatePetPowerTy uint32 = 0x00008000
	GroupUpdatePetCurPow  uint32 = 0x00010000
	GroupUpdatePetMaxPow  uint32 = 0x00020000
	GroupUpdatePetAuras   uint32 = 0x00040000
	GroupUpdateVehicle    uint32 = 0x00080000
)

// GroupMember is a member of the group the character is in.
type GroupMember struct {
	Name   string
	GUID   uint64
	Status uint8
	Group  uint8
	Flags  uint8
	Roles  uint8
}

// GroupInfo is SMSG_GROUP_LIST: the character's own place in the group, the
// others in it and how it loots.
type GroupInfo struct {
	Type       uint8
	OwnGroup   uint8
	OwnFlags   uint8
	OwnRoles   uint8
	GUID       uint64
	Counter    uint32
	Members    []GroupMember
	Leader     uint64
	LootMethod uint8
	Master     uint64
	Threshold  uint8
	Dungeon    uint8
	Raid       uint8
	Dynamic    uint8
}

// ParseGroupList reads SMSG_GROUP_LIST.
func ParseGroupList(body []byte) (GroupInfo, error) {
	r := NewReader(body, "SMSG_GROUP_LIST")
	var info GroupInfo
	var err error
	if info.Type, err = r.U8(); err != nil {
		return info, err
	}
	if info.OwnGroup, err = r.U8(); err != nil {
		return info, err
	}
	if info.OwnFlags, err = r.U8(); err != nil {
		return info, err
	}
	if info.OwnRoles, err = r.U8(); err != nil {
		return info, err
	}
	if info.Type&GroupTypeLFG != 0 {
		if err = r.Skip(5); err != nil {
			return info, err
		}
	}
	if info.GUID, err = r.U64(); err != nil {
		return info, err
	}
	if info.Counter, err = r.U32(); err != nil {
		return info, err
	}
	count, err := r.U32()
	if err != nil {
		return info, err
	}
	if count > 40 {
		count = 40
	}
	for index := uint32(0); index < count; index++ {
		var member GroupMember
		if member.Name, err = r.CString(); err != nil {
			return info, err
		}
		if member.GUID, err = r.U64(); err != nil {
			return info, err
		}
		if member.Status, err = r.U8(); err != nil {
			return info, err
		}
		if member.Group, err = r.U8(); err != nil {
			return info, err
		}
		if member.Flags, err = r.U8(); err != nil {
			return info, err
		}
		if member.Roles, err = r.U8(); err != nil {
			return info, err
		}
		info.Members = append(info.Members, member)
	}
	if info.Leader, err = r.U64(); err != nil {
		return info, err
	}
	if count == 0 || r.Remaining() < 1 {
		return info, nil
	}
	if info.LootMethod, err = r.U8(); err != nil {
		return info, err
	}
	if info.Master, err = r.U64(); err != nil {
		return info, err
	}
	if info.Threshold, err = r.U8(); err != nil {
		return info, err
	}
	if info.Dungeon, err = r.U8(); err != nil {
		return info, err
	}
	if info.Raid, err = r.U8(); err != nil {
		return info, err
	}
	if r.Remaining() > 0 {
		info.Dynamic, err = r.U8()
	}
	return info, err
}

// GroupInvitation is SMSG_GROUP_INVITE: who asks the character to join.
type GroupInvitation struct {
	Valid   bool
	Inviter string
}

// ParseGroupInvite reads SMSG_GROUP_INVITE.
func ParseGroupInvite(body []byte) (GroupInvitation, error) {
	r := NewReader(body, "SMSG_GROUP_INVITE")
	var invite GroupInvitation
	flag, err := r.U8()
	if err != nil {
		return invite, err
	}
	invite.Valid = flag != 0
	invite.Inviter, err = r.CString()
	return invite, err
}

// PartyResultInfo is SMSG_PARTY_COMMAND_RESULT.
type PartyResultInfo struct {
	Operation uint32
	Name      string
	Result    uint32
	Value     uint32
}

// ParsePartyCommandResult reads SMSG_PARTY_COMMAND_RESULT.
func ParsePartyCommandResult(body []byte) (PartyResultInfo, error) {
	r := NewReader(body, "SMSG_PARTY_COMMAND_RESULT")
	var info PartyResultInfo
	var err error
	if info.Operation, err = r.U32(); err != nil {
		return info, err
	}
	if info.Name, err = r.CString(); err != nil {
		return info, err
	}
	if info.Result, err = r.U32(); err != nil {
		return info, err
	}
	if r.Remaining() >= 4 {
		info.Value, err = r.U32()
	}
	return info, err
}

// MemberAura is an aura shown in a member's frame.
type MemberAura struct {
	Slot  int
	Spell uint32
	Flags uint8
}

// MemberStats is SMSG_PARTY_MEMBER_STATS (or the full one): the fields of a
// member that changed.
type MemberStats struct {
	GUID      uint64
	Mask      uint32
	Status    uint16
	Health    uint32
	MaxHealth uint32
	PowerType uint8
	Power     uint16
	MaxPower  uint16
	Level     uint16
	Zone      uint16
	X, Y      int16
	Auras     []MemberAura
	PetGUID   uint64
	PetName   string
	PetModel  uint16
	PetHealth uint32
	PetMaxHP  uint32
	PetPower  uint8
	PetCur    uint16
	PetMax    uint16
	PetAuras  []MemberAura
	Seat      uint32
}

func readMemberAuras(r *Reader) ([]MemberAura, error) {
	mask, err := r.U64()
	if err != nil {
		return nil, err
	}
	var auras []MemberAura
	for slot := 0; slot < 64; slot++ {
		if mask&(1<<uint(slot)) == 0 {
			continue
		}
		spell, err := r.U32()
		if err != nil {
			return auras, err
		}
		flags, err := r.U8()
		if err != nil {
			return auras, err
		}
		auras = append(auras, MemberAura{Slot: slot, Spell: spell, Flags: flags})
	}
	return auras, nil
}

// ParsePartyMemberStats reads SMSG_PARTY_MEMBER_STATS; full is set for the
// answer to a request, which has one more byte in front.
func ParsePartyMemberStats(body []byte, full bool) (MemberStats, error) {
	r := NewReader(body, "SMSG_PARTY_MEMBER_STATS")
	var stats MemberStats
	if full {
		if _, err := r.U8(); err != nil {
			return stats, err
		}
	}
	guid, err := ReadPackedGUID(r)
	if err != nil {
		return stats, err
	}
	stats.GUID = guid
	if stats.Mask, err = r.U32(); err != nil {
		return stats, err
	}
	mask := stats.Mask
	if mask&GroupUpdateStatus != 0 {
		if stats.Status, err = r.U16(); err != nil {
			return stats, err
		}
	}
	if mask&GroupUpdateCurHP != 0 {
		if stats.Health, err = r.U32(); err != nil {
			return stats, err
		}
	}
	if mask&GroupUpdateMaxHP != 0 {
		if stats.MaxHealth, err = r.U32(); err != nil {
			return stats, err
		}
	}
	if mask&GroupUpdatePowerType != 0 {
		if stats.PowerType, err = r.U8(); err != nil {
			return stats, err
		}
	}
	if mask&GroupUpdateCurPower != 0 {
		if stats.Power, err = r.U16(); err != nil {
			return stats, err
		}
	}
	if mask&GroupUpdateMaxPower != 0 {
		if stats.MaxPower, err = r.U16(); err != nil {
			return stats, err
		}
	}
	if mask&GroupUpdateLevel != 0 {
		if stats.Level, err = r.U16(); err != nil {
			return stats, err
		}
	}
	if mask&GroupUpdateZone != 0 {
		if stats.Zone, err = r.U16(); err != nil {
			return stats, err
		}
	}
	if mask&GroupUpdatePosition != 0 {
		x, err := r.U16()
		if err != nil {
			return stats, err
		}
		y, err := r.U16()
		if err != nil {
			return stats, err
		}
		stats.X, stats.Y = int16(x), int16(y)
	}
	if mask&GroupUpdateAuras != 0 {
		if stats.Auras, err = readMemberAuras(r); err != nil {
			return stats, err
		}
	}
	if mask&GroupUpdatePetGUID != 0 {
		if stats.PetGUID, err = r.U64(); err != nil {
			return stats, err
		}
	}
	if mask&GroupUpdatePetName != 0 {
		if stats.PetName, err = r.CString(); err != nil {
			return stats, err
		}
	}
	if mask&GroupUpdatePetModel != 0 {
		if stats.PetModel, err = r.U16(); err != nil {
			return stats, err
		}
	}
	if mask&GroupUpdatePetCurHP != 0 {
		if stats.PetHealth, err = r.U32(); err != nil {
			return stats, err
		}
	}
	if mask&GroupUpdatePetMaxHP != 0 {
		if stats.PetMaxHP, err = r.U32(); err != nil {
			return stats, err
		}
	}
	if mask&GroupUpdatePetPowerTy != 0 {
		if stats.PetPower, err = r.U8(); err != nil {
			return stats, err
		}
	}
	if mask&GroupUpdatePetCurPow != 0 {
		if stats.PetCur, err = r.U16(); err != nil {
			return stats, err
		}
	}
	if mask&GroupUpdatePetMaxPow != 0 {
		if stats.PetMax, err = r.U16(); err != nil {
			return stats, err
		}
	}
	if mask&GroupUpdatePetAuras != 0 {
		if stats.PetAuras, err = readMemberAuras(r); err != nil {
			return stats, err
		}
	}
	if mask&GroupUpdateVehicle != 0 {
		stats.Seat, err = r.U32()
	}
	return stats, err
}

// TargetIcon is a raid target mark on a unit.
type TargetIcon struct {
	Icon uint8
	GUID uint64
}

// TargetIconUpdate is MSG_RAID_TARGET_UPDATE from the server: one mark set by
// someone (Set) or the whole list.
type TargetIconUpdate struct {
	List  bool
	Who   uint64
	Icons []TargetIcon
}

// ParseRaidTargetUpdate reads MSG_RAID_TARGET_UPDATE.
func ParseRaidTargetUpdate(body []byte) (TargetIconUpdate, error) {
	r := NewReader(body, "MSG_RAID_TARGET_UPDATE")
	var update TargetIconUpdate
	kind, err := r.U8()
	if err != nil {
		return update, err
	}
	if kind == 0 {
		if update.Who, err = r.U64(); err != nil {
			return update, err
		}
		icon, err := r.U8()
		if err != nil {
			return update, err
		}
		guid, err := r.U64()
		if err != nil {
			return update, err
		}
		update.Icons = []TargetIcon{{Icon: icon, GUID: guid}}
		return update, nil
	}
	update.List = true
	for r.Remaining() >= 9 {
		icon, _ := r.U8()
		guid, _ := r.U64()
		update.Icons = append(update.Icons, TargetIcon{Icon: icon, GUID: guid})
	}
	return update, nil
}

// LootRollStart is SMSG_LOOT_START_ROLL: an item up for a roll.
type LootRollStart struct {
	Source    uint64
	Map       uint32
	Slot      uint32
	Item      uint32
	Suffix    uint32
	Property  uint32
	Count     uint32
	Countdown uint32
	Mask      uint8
}

// ParseLootStartRoll reads SMSG_LOOT_START_ROLL.
func ParseLootStartRoll(body []byte) (LootRollStart, error) {
	r := NewReader(body, "SMSG_LOOT_START_ROLL")
	var roll LootRollStart
	var err error
	if roll.Source, err = r.U64(); err != nil {
		return roll, err
	}
	for _, field := range []*uint32{&roll.Map, &roll.Slot, &roll.Item, &roll.Suffix, &roll.Property, &roll.Count, &roll.Countdown} {
		if *field, err = r.U32(); err != nil {
			return roll, err
		}
	}
	roll.Mask, err = r.U8()
	return roll, err
}

// LootRollVote is SMSG_LOOT_ROLL: one player's vote.
type LootRollVote struct {
	Source   uint64
	Slot     uint32
	Player   uint64
	Item     uint32
	Suffix   uint32
	Property uint32
	Number   uint8
	Type     uint8
	AutoPass bool
}

// ParseLootRoll reads SMSG_LOOT_ROLL.
func ParseLootRoll(body []byte) (LootRollVote, error) {
	r := NewReader(body, "SMSG_LOOT_ROLL")
	var vote LootRollVote
	var err error
	if vote.Source, err = r.U64(); err != nil {
		return vote, err
	}
	if vote.Slot, err = r.U32(); err != nil {
		return vote, err
	}
	if vote.Player, err = r.U64(); err != nil {
		return vote, err
	}
	if vote.Item, err = r.U32(); err != nil {
		return vote, err
	}
	if vote.Suffix, err = r.U32(); err != nil {
		return vote, err
	}
	if vote.Property, err = r.U32(); err != nil {
		return vote, err
	}
	if vote.Number, err = r.U8(); err != nil {
		return vote, err
	}
	if vote.Type, err = r.U8(); err != nil {
		return vote, err
	}
	pass, err := r.U8()
	vote.AutoPass = pass != 0
	return vote, err
}

// LootRollResult is SMSG_LOOT_ROLL_WON.
type LootRollResult struct {
	Source   uint64
	Slot     uint32
	Item     uint32
	Suffix   uint32
	Property uint32
	Winner   uint64
	Number   uint8
	Type     uint8
}

// ParseLootRollWon reads SMSG_LOOT_ROLL_WON.
func ParseLootRollWon(body []byte) (LootRollResult, error) {
	r := NewReader(body, "SMSG_LOOT_ROLL_WON")
	var won LootRollResult
	var err error
	if won.Source, err = r.U64(); err != nil {
		return won, err
	}
	for _, field := range []*uint32{&won.Slot, &won.Item, &won.Suffix, &won.Property} {
		if *field, err = r.U32(); err != nil {
			return won, err
		}
	}
	if won.Winner, err = r.U64(); err != nil {
		return won, err
	}
	if won.Number, err = r.U8(); err != nil {
		return won, err
	}
	won.Type, err = r.U8()
	return won, err
}

// LootPassed is SMSG_LOOT_ALL_PASSED.
type LootPassed struct {
	Source   uint64
	Slot     uint32
	Item     uint32
	Property uint32
	Suffix   uint32
}

// ParseLootAllPassed reads SMSG_LOOT_ALL_PASSED.
func ParseLootAllPassed(body []byte) (LootPassed, error) {
	r := NewReader(body, "SMSG_LOOT_ALL_PASSED")
	var passed LootPassed
	var err error
	if passed.Source, err = r.U64(); err != nil {
		return passed, err
	}
	for _, field := range []*uint32{&passed.Slot, &passed.Item, &passed.Property, &passed.Suffix} {
		if *field, err = r.U32(); err != nil {
			return passed, err
		}
	}
	return passed, nil
}

// RollResult is MSG_RANDOM_ROLL from the server.
type RollResult struct {
	Min, Max, Roll uint32
	GUID           uint64
}

// ParseRandomRoll reads MSG_RANDOM_ROLL.
func ParseRandomRoll(body []byte) (RollResult, error) {
	r := NewReader(body, "MSG_RANDOM_ROLL")
	var roll RollResult
	var err error
	if roll.Min, err = r.U32(); err != nil {
		return roll, err
	}
	if roll.Max, err = r.U32(); err != nil {
		return roll, err
	}
	if roll.Roll, err = r.U32(); err != nil {
		return roll, err
	}
	roll.GUID, err = r.U64()
	return roll, err
}

// SavedInstance is a raid or dungeon the character is saved to.
type SavedInstance struct {
	Map        uint32
	Difficulty uint32
	ID         uint64
	Expired    bool
	Extended   bool
	Reset      uint32
}

// ParseRaidInstanceInfo reads SMSG_RAID_INSTANCE_INFO.
func ParseRaidInstanceInfo(body []byte) ([]SavedInstance, error) {
	r := NewReader(body, "SMSG_RAID_INSTANCE_INFO")
	count, err := r.U32()
	if err != nil {
		return nil, err
	}
	var saved []SavedInstance
	for index := uint32(0); index < count && index < 64; index++ {
		var instance SavedInstance
		if instance.Map, err = r.U32(); err != nil {
			return saved, err
		}
		if instance.Difficulty, err = r.U32(); err != nil {
			return saved, err
		}
		if instance.ID, err = r.U64(); err != nil {
			return saved, err
		}
		live, err := r.U8()
		if err != nil {
			return saved, err
		}
		extended, err := r.U8()
		if err != nil {
			return saved, err
		}
		if instance.Reset, err = r.U32(); err != nil {
			return saved, err
		}
		instance.Expired, instance.Extended = live == 0, extended != 0
		saved = append(saved, instance)
	}
	return saved, nil
}

func cstring(text string) []byte { return append([]byte(text), 0) }

// BuildGroupInvite is the body of CMSG_GROUP_INVITE.
func BuildGroupInvite(name string) []byte {
	return binary.LittleEndian.AppendUint32(cstring(name), 0)
}

// BuildGroupAccept is the body of CMSG_GROUP_ACCEPT.
func BuildGroupAccept() []byte { return make([]byte, 4) }

// BuildGroupUninvite is the body of CMSG_GROUP_UNINVITE.
func BuildGroupUninvite(name string) []byte { return cstring(name) }

// BuildGuid is the body of the requests that name a member by guid.
func BuildGuid(guid uint64) []byte { return binary.LittleEndian.AppendUint64(nil, guid) }

// BuildLootMethod is the body of CMSG_LOOT_METHOD.
func BuildLootMethod(method uint32, master uint64, threshold uint32) []byte {
	body := binary.LittleEndian.AppendUint32(nil, method)
	body = binary.LittleEndian.AppendUint64(body, master)
	return binary.LittleEndian.AppendUint32(body, threshold)
}

// BuildLootRoll is the body of CMSG_LOOT_ROLL (0 pass, 1 need, 2 greed).
func BuildLootRoll(source uint64, slot uint32, vote uint8) []byte {
	body := binary.LittleEndian.AppendUint64(nil, source)
	body = binary.LittleEndian.AppendUint32(body, slot)
	return append(body, vote)
}

// BuildLootMasterGive is the body of CMSG_LOOT_MASTER_GIVE.
func BuildLootMasterGive(loot uint64, slot uint8, target uint64) []byte {
	body := binary.LittleEndian.AppendUint64(nil, loot)
	body = append(body, slot)
	return binary.LittleEndian.AppendUint64(body, target)
}

// BuildMinimapPing is the body of MSG_MINIMAP_PING.
func BuildMinimapPing(x, y float32) []byte {
	body := binary.LittleEndian.AppendUint32(nil, math.Float32bits(x))
	return binary.LittleEndian.AppendUint32(body, math.Float32bits(y))
}

// BuildRandomRoll is the body of MSG_RANDOM_ROLL.
func BuildRandomRoll(low, high uint32) []byte {
	return binary.LittleEndian.AppendUint32(binary.LittleEndian.AppendUint32(nil, low), high)
}

// BuildChangeSubGroup is the body of CMSG_GROUP_CHANGE_SUB_GROUP.
func BuildChangeSubGroup(name string, group uint8) []byte { return append(cstring(name), group) }

// BuildSwapSubGroup is the body of CMSG_GROUP_SWAP_SUB_GROUP.
func BuildSwapSubGroup(first, second string) []byte {
	return append(cstring(first), cstring(second)...)
}

// BuildAssistantLeader is the body of CMSG_GROUP_ASSISTANT_LEADER.
func BuildAssistantLeader(guid uint64, apply bool) []byte {
	body := binary.LittleEndian.AppendUint64(nil, guid)
	if apply {
		return append(body, 1)
	}
	return append(body, 0)
}

// BuildPartyAssignment is the body of MSG_PARTY_ASSIGNMENT (0 main tank, 1
// main assist).
func BuildPartyAssignment(assignment uint8, apply bool, guid uint64) []byte {
	body := []byte{assignment, 0}
	if apply {
		body[1] = 1
	}
	return binary.LittleEndian.AppendUint64(body, guid)
}

// BuildRaidTarget is the body of MSG_RAID_TARGET_UPDATE: a mark on a unit, or
// (icon 0xFF) the request for the list.
func BuildRaidTarget(icon uint8, guid uint64) []byte {
	if icon == 0xFF {
		return []byte{0xFF}
	}
	return binary.LittleEndian.AppendUint64([]byte{icon}, guid)
}

// BuildReadyCheckAnswer is the body of MSG_RAID_READY_CHECK that answers one.
func BuildReadyCheckAnswer(ready bool) []byte {
	if ready {
		return []byte{1}
	}
	return []byte{0}
}

// BuildDifficulty is the body of the difficulty setting messages.
func BuildDifficulty(difficulty uint32) []byte {
	return binary.LittleEndian.AppendUint32(nil, difficulty)
}
