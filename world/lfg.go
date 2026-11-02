package world

import "encoding/binary"

// Dungeon finder packets.
const (
	LFGPlayerRewardPacket   uint16       = 0x01FF
	LFGTeleportDeniedPacket uint16       = 0x0200
	LFGOfferContinuePacket  uint16       = 0x0293
	LFGRoleChosenPacket     uint16       = 0x02BB
	LFGProposalUpdatePacket uint16       = 0x0361
	LFGRoleCheckPacket      uint16       = 0x0363
	LFGJoinResultPacket     uint16       = 0x0364
	LFGQueueStatusPacket    uint16       = 0x0365
	LFGUpdatePlayerPacket   uint16       = 0x0367
	LFGUpdatePartyPacket    uint16       = 0x0368
	LFGUpdateSearchPacket   uint16       = 0x0369
	LFGBootUpdatePacket     uint16       = 0x036D
	LFGPlayerInfoPacket     uint16       = 0x036F
	LFGPartyInfoPacket      uint16       = 0x0372
	LFGDisabledPacket       uint16       = 0x0398
	LFGOpenFinderPacket     uint16       = 0x0515
	ClientLFGJoin           ClientOpcode = 0x035C
	ClientLFGLeave          ClientOpcode = 0x035D
	ClientSearchLFGJoin     ClientOpcode = 0x035E
	ClientSearchLFGLeave    ClientOpcode = 0x035F
	ClientLFGProposalResult ClientOpcode = 0x0362
	ClientLFGSetComment     ClientOpcode = 0x0366
	ClientLFGSetRoles       ClientOpcode = 0x036A
	ClientLFGBootVote       ClientOpcode = 0x036C
	ClientLFGPlayerLockInfo ClientOpcode = 0x036E
	ClientLFGTeleport       ClientOpcode = 0x0370
	ClientLFGPartyLockInfo  ClientOpcode = 0x0371
)

// LFGRewardItem is an item a dungeon pays out.
type LFGRewardItem struct {
	Item, Display, Quantity uint32
}

// LFGRandomInfo is what the finder says of a random dungeon: whether the
// character did it today and what it pays.
type LFGRandomInfo struct {
	Entry                      uint32
	Done                       bool
	Money, XP, MoneyVar, XPVar uint32
	Items                      []LFGRewardItem
}

// LFGLock is a dungeon a player may not enter and why.
type LFGLock struct {
	Entry, Status uint32
}

// LFGPlayerInfo is SMSG_LFG_PLAYER_INFO.
type LFGPlayerInfo struct {
	Random []LFGRandomInfo
	Locks  []LFGLock
}

func readLFGLocks(r *Reader) ([]LFGLock, error) {
	count, err := r.U32()
	if err != nil {
		return nil, err
	}
	locks := make([]LFGLock, 0, min(int(count), 512))
	for index := uint32(0); index < count; index++ {
		var lock LFGLock
		if lock.Entry, err = r.U32(); err != nil {
			return locks, err
		}
		if lock.Status, err = r.U32(); err != nil {
			return locks, err
		}
		locks = append(locks, lock)
	}
	return locks, nil
}

func readLFGRewardItems(r *Reader) ([]LFGRewardItem, error) {
	count, err := r.U8()
	if err != nil {
		return nil, err
	}
	items := make([]LFGRewardItem, 0, count)
	for index := uint8(0); index < count; index++ {
		var item LFGRewardItem
		if item.Item, err = r.U32(); err != nil {
			return items, err
		}
		if item.Display, err = r.U32(); err != nil {
			return items, err
		}
		if item.Quantity, err = r.U32(); err != nil {
			return items, err
		}
		items = append(items, item)
	}
	return items, nil
}

// ParseLFGPlayerInfo reads SMSG_LFG_PLAYER_INFO.
func ParseLFGPlayerInfo(body []byte) (LFGPlayerInfo, error) {
	r := NewReader(body, "SMSG_LFG_PLAYER_INFO")
	var info LFGPlayerInfo
	count, err := r.U8()
	if err != nil {
		return info, err
	}
	for index := uint8(0); index < count; index++ {
		var random LFGRandomInfo
		if random.Entry, err = r.U32(); err != nil {
			return info, err
		}
		done, err := r.U8()
		if err != nil {
			return info, err
		}
		random.Done = done != 0
		for _, field := range []*uint32{&random.Money, &random.XP, &random.MoneyVar, &random.XPVar} {
			if *field, err = r.U32(); err != nil {
				return info, err
			}
		}
		if random.Items, err = readLFGRewardItems(r); err != nil {
			return info, err
		}
		info.Random = append(info.Random, random)
	}
	info.Locks, err = readLFGLocks(r)
	return info, err
}

// LFGPlayerLocks is one player's locks in the party lock list.
type LFGPlayerLocks struct {
	GUID  uint64
	Locks []LFGLock
}

// ParseLFGPartyLocks reads the lock list of SMSG_LFG_PARTY_INFO and of a join
// result the party could not meet.
func ParseLFGPartyLocks(r *Reader) ([]LFGPlayerLocks, error) {
	count, err := r.U8()
	if err != nil {
		return nil, err
	}
	players := make([]LFGPlayerLocks, 0, count)
	for index := uint8(0); index < count; index++ {
		var player LFGPlayerLocks
		if player.GUID, err = r.U64(); err != nil {
			return players, err
		}
		if player.Locks, err = readLFGLocks(r); err != nil {
			return players, err
		}
		players = append(players, player)
	}
	return players, nil
}

// LFGJoinResult is SMSG_LFG_JOIN_RESULT.
type LFGJoinResult struct {
	Result, State uint32
	Locks         []LFGPlayerLocks
}

// ParseLFGJoinResult reads SMSG_LFG_JOIN_RESULT.
func ParseLFGJoinResult(body []byte) (LFGJoinResult, error) {
	r := NewReader(body, "SMSG_LFG_JOIN_RESULT")
	var result LFGJoinResult
	var err error
	if result.Result, err = r.U32(); err != nil {
		return result, err
	}
	if result.State, err = r.U32(); err != nil {
		return result, err
	}
	if result.Result == 6 {
		result.Locks, err = ParseLFGPartyLocks(r)
	}
	return result, err
}

// LFGParty reads SMSG_LFG_PARTY_INFO.
func ParseLFGPartyInfo(body []byte) ([]LFGPlayerLocks, error) {
	return ParseLFGPartyLocks(NewReader(body, "SMSG_LFG_PARTY_INFO"))
}

// LFGStatus is a player's or party's entry in the finder: SMSG_LFG_UPDATE_PLAYER
// and SMSG_LFG_UPDATE_PARTY.
type LFGStatus struct {
	UpdateType   uint8
	Joined       bool // in the finder's queue or list
	Join         bool // for a party: the finder was joined
	Queued       bool
	NoPartial    bool
	Achievements bool
	Needs        [3]uint8
	Dungeons     []uint32
	Comment      string
}

// ParseLFGUpdatePlayer reads SMSG_LFG_UPDATE_PLAYER.
func ParseLFGUpdatePlayer(body []byte) (LFGStatus, error) {
	r := NewReader(body, "SMSG_LFG_UPDATE_PLAYER")
	var status LFGStatus
	var err error
	if status.UpdateType, err = r.U8(); err != nil {
		return status, err
	}
	joined, err := r.U8()
	if err != nil {
		return status, err
	}
	status.Joined, status.Join = joined != 0, joined != 0
	if joined == 0 {
		return status, nil
	}
	flags := make([]uint8, 3)
	for index := range flags {
		if flags[index], err = r.U8(); err != nil {
			return status, err
		}
	}
	status.Queued, status.NoPartial, status.Achievements = flags[0] != 0, flags[1] != 0, flags[2] != 0
	return readLFGStatusTail(r, status)
}

// ParseLFGUpdateParty reads SMSG_LFG_UPDATE_PARTY.
func ParseLFGUpdateParty(body []byte) (LFGStatus, error) {
	r := NewReader(body, "SMSG_LFG_UPDATE_PARTY")
	var status LFGStatus
	var err error
	if status.UpdateType, err = r.U8(); err != nil {
		return status, err
	}
	joined, err := r.U8()
	if err != nil {
		return status, err
	}
	status.Joined = joined != 0
	if joined == 0 {
		return status, nil
	}
	flags := make([]uint8, 4)
	for index := range flags {
		if flags[index], err = r.U8(); err != nil {
			return status, err
		}
	}
	status.Join, status.Queued, status.NoPartial, status.Achievements = flags[0] != 0, flags[1] != 0, flags[2] != 0, flags[3] != 0
	for index := range status.Needs {
		if status.Needs[index], err = r.U8(); err != nil {
			return status, err
		}
	}
	return readLFGStatusTail(r, status)
}

func readLFGStatusTail(r *Reader, status LFGStatus) (LFGStatus, error) {
	count, err := r.U8()
	if err != nil {
		return status, err
	}
	for index := uint8(0); index < count; index++ {
		entry, err := r.U32()
		if err != nil {
			return status, err
		}
		status.Dungeons = append(status.Dungeons, entry)
	}
	status.Comment, err = r.CString()
	return status, err
}

// LFGProposalPlayer is one of the five a proposal names.
type LFGProposalPlayer struct {
	Role                       uint32
	Self, InDungeon, SameGroup bool
	Answered, Accepted         bool
}

// LFGProposal is SMSG_LFG_PROPOSAL_UPDATE.
type LFGProposal struct {
	Entry      uint32
	State      uint8
	ID         uint32
	Encounters uint32
	Silent     bool
	Players    []LFGProposalPlayer
}

// ParseLFGProposal reads SMSG_LFG_PROPOSAL_UPDATE.
func ParseLFGProposal(body []byte) (LFGProposal, error) {
	r := NewReader(body, "SMSG_LFG_PROPOSAL_UPDATE")
	var proposal LFGProposal
	var err error
	if proposal.Entry, err = r.U32(); err != nil {
		return proposal, err
	}
	if proposal.State, err = r.U8(); err != nil {
		return proposal, err
	}
	if proposal.ID, err = r.U32(); err != nil {
		return proposal, err
	}
	if proposal.Encounters, err = r.U32(); err != nil {
		return proposal, err
	}
	silent, err := r.U8()
	if err != nil {
		return proposal, err
	}
	proposal.Silent = silent != 0
	count, err := r.U8()
	if err != nil {
		return proposal, err
	}
	for index := uint8(0); index < count && index < 5; index++ {
		var player LFGProposalPlayer
		if player.Role, err = r.U32(); err != nil {
			return proposal, err
		}
		flags := make([]uint8, 5)
		for slot := range flags {
			if flags[slot], err = r.U8(); err != nil {
				return proposal, err
			}
		}
		player.Self, player.InDungeon, player.SameGroup = flags[0] != 0, flags[1] != 0, flags[2] != 0
		player.Answered, player.Accepted = flags[3] != 0, flags[4] != 0
		proposal.Players = append(proposal.Players, player)
	}
	return proposal, nil
}

// LFGRoleCheckMember is a member of the group a role check is about.
type LFGRoleCheckMember struct {
	GUID  uint64
	Ready bool
	Roles uint32
	Level uint8
}

// LFGRoleCheck is SMSG_LFG_ROLE_CHECK_UPDATE.
type LFGRoleCheck struct {
	State     uint32
	Beginning bool
	Dungeons  []uint32
	Members   []LFGRoleCheckMember
}

// ParseLFGRoleCheck reads SMSG_LFG_ROLE_CHECK_UPDATE.
func ParseLFGRoleCheck(body []byte) (LFGRoleCheck, error) {
	r := NewReader(body, "SMSG_LFG_ROLE_CHECK_UPDATE")
	var check LFGRoleCheck
	var err error
	if check.State, err = r.U32(); err != nil {
		return check, err
	}
	beginning, err := r.U8()
	if err != nil {
		return check, err
	}
	check.Beginning = beginning != 0
	count, err := r.U8()
	if err != nil {
		return check, err
	}
	for index := uint8(0); index < count; index++ {
		entry, err := r.U32()
		if err != nil {
			return check, err
		}
		check.Dungeons = append(check.Dungeons, entry)
	}
	members, err := r.U8()
	if err != nil {
		return check, err
	}
	for index := uint8(0); index < members; index++ {
		var member LFGRoleCheckMember
		if member.GUID, err = r.U64(); err != nil {
			return check, err
		}
		ready, err := r.U8()
		if err != nil {
			return check, err
		}
		member.Ready = ready != 0
		if member.Roles, err = r.U32(); err != nil {
			return check, err
		}
		if member.Level, err = r.U8(); err != nil {
			return check, err
		}
		check.Members = append(check.Members, member)
	}
	return check, nil
}

// LFGQueueStatus is SMSG_LFG_QUEUE_STATUS.
type LFGQueueStatus struct {
	Entry                            uint32
	AverageWait, Wait                int32
	TankWait, HealerWait, DamageWait int32
	Tanks, Healers, Damage           uint8
	QueuedSeconds                    uint32
}

// ParseLFGQueueStatus reads SMSG_LFG_QUEUE_STATUS.
func ParseLFGQueueStatus(body []byte) (LFGQueueStatus, error) {
	r := NewReader(body, "SMSG_LFG_QUEUE_STATUS")
	var status LFGQueueStatus
	var err error
	if status.Entry, err = r.U32(); err != nil {
		return status, err
	}
	for _, field := range []*int32{&status.AverageWait, &status.Wait, &status.TankWait, &status.HealerWait, &status.DamageWait} {
		value, err := r.U32()
		if err != nil {
			return status, err
		}
		*field = int32(value)
	}
	for _, field := range []*uint8{&status.Tanks, &status.Healers, &status.Damage} {
		if *field, err = r.U8(); err != nil {
			return status, err
		}
	}
	status.QueuedSeconds, err = r.U32()
	return status, err
}

// LFGBootVote is SMSG_LFG_BOOT_PROPOSAL_UPDATE.
type LFGBootVote struct {
	InProgress, Voted, Agreed bool
	Victim                    uint64
	Total, Agree, TimeLeft    uint32
	Needed                    uint32
	Reason                    string
}

// ParseLFGBootVote reads SMSG_LFG_BOOT_PROPOSAL_UPDATE.
func ParseLFGBootVote(body []byte) (LFGBootVote, error) {
	r := NewReader(body, "SMSG_LFG_BOOT_PROPOSAL_UPDATE")
	var vote LFGBootVote
	flags := make([]uint8, 3)
	var err error
	for index := range flags {
		if flags[index], err = r.U8(); err != nil {
			return vote, err
		}
	}
	vote.InProgress, vote.Voted, vote.Agreed = flags[0] != 0, flags[1] != 0, flags[2] != 0
	if vote.Victim, err = r.U64(); err != nil {
		return vote, err
	}
	for _, field := range []*uint32{&vote.Total, &vote.Agree, &vote.TimeLeft, &vote.Needed} {
		if *field, err = r.U32(); err != nil {
			return vote, err
		}
	}
	vote.Reason, err = r.CString()
	return vote, err
}

// LFGReward is SMSG_LFG_PLAYER_REWARD: what a finished dungeon paid.
type LFGReward struct {
	Random, Actual             uint32
	First                      uint8
	Strangers                  uint32
	Money, XP, MoneyVar, XPVar uint32
	Items                      []LFGRewardItem
}

// ParseLFGReward reads SMSG_LFG_PLAYER_REWARD.
func ParseLFGReward(body []byte) (LFGReward, error) {
	r := NewReader(body, "SMSG_LFG_PLAYER_REWARD")
	var reward LFGReward
	var err error
	if reward.Random, err = r.U32(); err != nil {
		return reward, err
	}
	if reward.Actual, err = r.U32(); err != nil {
		return reward, err
	}
	if reward.First, err = r.U8(); err != nil {
		return reward, err
	}
	for _, field := range []*uint32{&reward.Strangers, &reward.Money, &reward.XP, &reward.MoneyVar, &reward.XPVar} {
		if *field, err = r.U32(); err != nil {
			return reward, err
		}
	}
	reward.Items, err = readLFGRewardItems(r)
	return reward, err
}

// LFGRoleChosen is SMSG_LFG_ROLE_CHOSEN.
type LFGRoleChosen struct {
	GUID  uint64
	Ready bool
	Roles uint32
}

// ParseLFGRoleChosen reads SMSG_LFG_ROLE_CHOSEN.
func ParseLFGRoleChosen(body []byte) (LFGRoleChosen, error) {
	r := NewReader(body, "SMSG_LFG_ROLE_CHOSEN")
	var chosen LFGRoleChosen
	var err error
	if chosen.GUID, err = r.U64(); err != nil {
		return chosen, err
	}
	ready, err := r.U8()
	if err != nil {
		return chosen, err
	}
	chosen.Ready = ready != 0
	chosen.Roles, err = r.U32()
	return chosen, err
}

// BuildLFGJoin is the body of CMSG_LFG_JOIN.
func BuildLFGJoin(roles uint8, noPartial, achievements bool, dungeons []uint32, needs [3]uint8, comment string) []byte {
	body := []byte{roles, boolByte(noPartial), boolByte(achievements), uint8(len(dungeons))}
	for _, entry := range dungeons {
		body = binary.LittleEndian.AppendUint32(body, entry)
	}
	body = append(body, 3, needs[0], needs[1], needs[2])
	return append(append(body, comment...), 0)
}

// BuildLFGProposalResult is the body of CMSG_LFG_PROPOSAL_RESULT.
func BuildLFGProposalResult(id uint32, accept bool) []byte {
	return append(binary.LittleEndian.AppendUint32(nil, id), boolByte(accept))
}

// BuildLFGComment is the body of CMSG_SET_LFG_COMMENT.
func BuildLFGComment(comment string) []byte { return append([]byte(comment), 0) }

// BuildLFGBootVote is the body of CMSG_LFG_SET_BOOT_VOTE.
func BuildLFGBootVote(agree bool) []byte { return []byte{boolByte(agree)} }

// BuildLFGTeleport is the body of CMSG_LFG_TELEPORT.
func BuildLFGTeleport(out bool) []byte { return []byte{boolByte(out)} }

// BuildSearchLFG is the body of CMSG_SEARCH_LFG_JOIN and _LEAVE.
func BuildSearchLFG(entry uint32) []byte { return binary.LittleEndian.AppendUint32(nil, entry) }
