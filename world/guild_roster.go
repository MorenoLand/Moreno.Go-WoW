package world

import "encoding/binary"

// Guild packets other than the bank's.
const (
	GuildInvite         uint16 = 0x0083
	GuildDecline        uint16 = 0x0086
	GuildInfo           uint16 = 0x0088
	GuildRoster         uint16 = 0x008A
	GuildEvent          uint16 = 0x0092
	GuildCommandResult  uint16 = 0x0093
	GuildEventLogResult uint16 = 0x03FF

	ClientGuildCreate     ClientOpcode = 0x0081
	ClientGuildInvite     ClientOpcode = 0x0082
	ClientGuildAccept     ClientOpcode = 0x0084
	ClientGuildDecline    ClientOpcode = 0x0085
	ClientGuildInfo       ClientOpcode = 0x0087
	ClientGuildRoster     ClientOpcode = 0x0089
	ClientGuildPromote    ClientOpcode = 0x008B
	ClientGuildDemote     ClientOpcode = 0x008C
	ClientGuildLeave      ClientOpcode = 0x008D
	ClientGuildRemove     ClientOpcode = 0x008E
	ClientGuildDisband    ClientOpcode = 0x008F
	ClientGuildLeader     ClientOpcode = 0x0090
	ClientGuildMotd       ClientOpcode = 0x0091
	ClientGuildRank       ClientOpcode = 0x0231
	ClientGuildAddRank    ClientOpcode = 0x0232
	ClientGuildDelRank    ClientOpcode = 0x0233
	ClientGuildPublicNote ClientOpcode = 0x0234
	ClientGuildOfficerNte ClientOpcode = 0x0235
	ClientGuildInfoText   ClientOpcode = 0x02FC
	ClientGuildEventLog   ClientOpcode = 0x03FF
	ClientSaveGuildEmblem ClientOpcode = 0x01F1
)

// Guild member status bits.
const (
	GuildMemberOnline uint8 = 0x01
	GuildMemberAFK    uint8 = 0x02
	GuildMemberDND    uint8 = 0x04
)

// Guild events.
const (
	GuildEventPromotion  uint8 = 0
	GuildEventDemotion   uint8 = 1
	GuildEventMotd       uint8 = 2
	GuildEventJoined     uint8 = 3
	GuildEventLeft       uint8 = 4
	GuildEventRemoved    uint8 = 5
	GuildEventLeaderIs   uint8 = 6
	GuildEventLeaderNew  uint8 = 7
	GuildEventDisbanded  uint8 = 8
	GuildEventTabard     uint8 = 9
	GuildEventRankUpdate uint8 = 10
	GuildEventRankDelete uint8 = 11
	GuildEventSignedOn   uint8 = 12
	GuildEventSignedOff  uint8 = 13
)

// GuildQuery is SMSG_GUILD_QUERY_RESPONSE in full.
type GuildQuery struct {
	ID              uint32
	Name            string
	Ranks           [10]string
	EmblemStyle     uint32
	EmblemColor     uint32
	BorderStyle     uint32
	BorderColor     uint32
	BackgroundColor uint32
	RankCount       uint32
}

// ParseGuildQuery reads SMSG_GUILD_QUERY_RESPONSE.
func ParseGuildQuery(body []byte) (GuildQuery, error) {
	r := NewReader(body, "SMSG_GUILD_QUERY_RESPONSE")
	var guild GuildQuery
	var err error
	if guild.ID, err = r.U32(); err != nil {
		return guild, err
	}
	if guild.Name, err = r.CString(); err != nil {
		return guild, err
	}
	for index := range guild.Ranks {
		if guild.Ranks[index], err = r.CString(); err != nil {
			return guild, err
		}
	}
	for _, field := range []*uint32{&guild.EmblemStyle, &guild.EmblemColor, &guild.BorderStyle, &guild.BorderColor, &guild.BackgroundColor, &guild.RankCount} {
		if *field, err = r.U32(); err != nil {
			return guild, err
		}
	}
	return guild, nil
}

// GuildRank is the rights of one rank, as the roster lists them.
type GuildRank struct {
	Flags     uint32
	GoldLimit uint32
	TabFlags  [6]uint32
	TabLimit  [6]uint32
}

// GuildRosterMember is one member of the guild.
type GuildRosterMember struct {
	GUID        uint64
	Status      uint8
	Name        string
	Rank        int32
	Level       uint8
	Class       uint8
	Gender      uint8
	Area        int32
	LastOnline  float32
	Note        string
	OfficerNote string
}

// GuildRosterInfo is SMSG_GUILD_ROSTER.
type GuildRosterInfo struct {
	Welcome string
	Info    string
	Ranks   []GuildRank
	Members []GuildRosterMember
}

// ParseGuildRoster reads SMSG_GUILD_ROSTER.
func ParseGuildRoster(body []byte) (GuildRosterInfo, error) {
	r := NewReader(body, "SMSG_GUILD_ROSTER")
	var roster GuildRosterInfo
	count, err := r.U32()
	if err != nil {
		return roster, err
	}
	if roster.Welcome, err = r.CString(); err != nil {
		return roster, err
	}
	if roster.Info, err = r.CString(); err != nil {
		return roster, err
	}
	ranks, err := r.U32()
	if err != nil {
		return roster, err
	}
	if ranks > 10 {
		ranks = 10
	}
	for index := uint32(0); index < ranks; index++ {
		var rank GuildRank
		if rank.Flags, err = r.U32(); err != nil {
			return roster, err
		}
		if rank.GoldLimit, err = r.U32(); err != nil {
			return roster, err
		}
		for tab := 0; tab < 6; tab++ {
			if rank.TabFlags[tab], err = r.U32(); err != nil {
				return roster, err
			}
			if rank.TabLimit[tab], err = r.U32(); err != nil {
				return roster, err
			}
		}
		roster.Ranks = append(roster.Ranks, rank)
	}
	if count > 1000 {
		count = 1000
	}
	for index := uint32(0); index < count; index++ {
		var member GuildRosterMember
		if member.GUID, err = r.U64(); err != nil {
			return roster, err
		}
		if member.Status, err = r.U8(); err != nil {
			return roster, err
		}
		if member.Name, err = r.CString(); err != nil {
			return roster, err
		}
		rank, err := r.U32()
		if err != nil {
			return roster, err
		}
		member.Rank = int32(rank)
		if member.Level, err = r.U8(); err != nil {
			return roster, err
		}
		if member.Class, err = r.U8(); err != nil {
			return roster, err
		}
		if member.Gender, err = r.U8(); err != nil {
			return roster, err
		}
		area, err := r.U32()
		if err != nil {
			return roster, err
		}
		member.Area = int32(area)
		if member.Status == 0 {
			if member.LastOnline, err = r.F32(); err != nil {
				return roster, err
			}
		}
		if member.Note, err = r.CString(); err != nil {
			return roster, err
		}
		if member.OfficerNote, err = r.CString(); err != nil {
			return roster, err
		}
		roster.Members = append(roster.Members, member)
	}
	return roster, nil
}

// GuildEventInfo is SMSG_GUILD_EVENT.
type GuildEventInfo struct {
	Type   uint8
	Params []string
	GUID   uint64
}

// ParseGuildEvent reads SMSG_GUILD_EVENT.
func ParseGuildEvent(body []byte) (GuildEventInfo, error) {
	r := NewReader(body, "SMSG_GUILD_EVENT")
	var event GuildEventInfo
	var err error
	if event.Type, err = r.U8(); err != nil {
		return event, err
	}
	count, err := r.U8()
	if err != nil {
		return event, err
	}
	for index := uint8(0); index < count; index++ {
		param, err := r.CString()
		if err != nil {
			return event, err
		}
		event.Params = append(event.Params, param)
	}
	switch event.Type {
	case GuildEventJoined, GuildEventLeft, GuildEventSignedOn, GuildEventSignedOff:
		event.GUID, err = r.U64()
	}
	return event, err
}

// GuildCommand is SMSG_GUILD_COMMAND_RESULT.
type GuildCommand struct {
	Command int32
	Name    string
	Result  int32
}

// ParseGuildCommandResult reads SMSG_GUILD_COMMAND_RESULT.
func ParseGuildCommandResult(body []byte) (GuildCommand, error) {
	r := NewReader(body, "SMSG_GUILD_COMMAND_RESULT")
	var command GuildCommand
	value, err := r.U32()
	if err != nil {
		return command, err
	}
	command.Command = int32(value)
	if command.Name, err = r.CString(); err != nil {
		return command, err
	}
	result, err := r.U32()
	command.Result = int32(result)
	return command, err
}

// GuildInvitation is SMSG_GUILD_INVITE.
type GuildInvitation struct {
	Inviter string
	Guild   string
}

// ParseGuildInvite reads SMSG_GUILD_INVITE.
func ParseGuildInvite(body []byte) (GuildInvitation, error) {
	r := NewReader(body, "SMSG_GUILD_INVITE")
	var invite GuildInvitation
	var err error
	if invite.Inviter, err = r.CString(); err != nil {
		return invite, err
	}
	invite.Guild, err = r.CString()
	return invite, err
}

// GuildSummary is SMSG_GUILD_INFO.
type GuildSummary struct {
	Name     string
	Created  uint32
	Members  uint32
	Accounts uint32
}

// ParseGuildInfo reads SMSG_GUILD_INFO.
func ParseGuildInfo(body []byte) (GuildSummary, error) {
	r := NewReader(body, "SMSG_GUILD_INFO")
	var info GuildSummary
	var err error
	if info.Name, err = r.CString(); err != nil {
		return info, err
	}
	if info.Created, err = r.U32(); err != nil {
		return info, err
	}
	if info.Members, err = r.U32(); err != nil {
		return info, err
	}
	info.Accounts, err = r.U32()
	return info, err
}

// GuildLogEntry is a line of the guild's event log.
type GuildLogEntry struct {
	Type   uint8
	Player uint64
	Other  uint64
	Rank   uint8
	Time   uint32
}

// ParseGuildEventLog reads MSG_GUILD_EVENT_LOG_QUERY from the server.
func ParseGuildEventLog(body []byte) ([]GuildLogEntry, error) {
	r := NewReader(body, "MSG_GUILD_EVENT_LOG_QUERY")
	count, err := r.U8()
	if err != nil {
		return nil, err
	}
	var entries []GuildLogEntry
	for index := uint8(0); index < count; index++ {
		var entry GuildLogEntry
		if entry.Type, err = r.U8(); err != nil {
			return entries, err
		}
		if entry.Player, err = r.U64(); err != nil {
			return entries, err
		}
		if entry.Type != 2 && entry.Type != 6 {
			if entry.Other, err = r.U64(); err != nil {
				return entries, err
			}
		}
		if entry.Type == 3 || entry.Type == 4 {
			if entry.Rank, err = r.U8(); err != nil {
				return entries, err
			}
		}
		if entry.Time, err = r.U32(); err != nil {
			return entries, err
		}
		entries = append(entries, entry)
	}
	return entries, nil
}

// BuildGuildText is the body of the guild requests that carry one string.
func BuildGuildText(text string) []byte { return cstring(text) }

// BuildGuildNote is the body of CMSG_GUILD_SET_PUBLIC_NOTE and the officer one.
func BuildGuildNote(name, note string) []byte { return append(cstring(name), cstring(note)...) }

// BuildGuildRank is the body of CMSG_GUILD_RANK: the rights of one rank.
func BuildGuildRank(id uint32, rank GuildRank, name string) []byte {
	body := binary.LittleEndian.AppendUint32(nil, id)
	body = binary.LittleEndian.AppendUint32(body, rank.Flags)
	body = append(body, cstring(name)...)
	body = binary.LittleEndian.AppendUint32(body, rank.GoldLimit)
	for tab := 0; tab < 6; tab++ {
		body = binary.LittleEndian.AppendUint32(body, rank.TabFlags[tab])
		body = binary.LittleEndian.AppendUint32(body, rank.TabLimit[tab])
	}
	return body
}
