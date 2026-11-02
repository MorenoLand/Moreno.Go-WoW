package world

import "encoding/binary"

// Arena team opcodes.
const (
	ArenaTeamQueryResponse uint16 = 0x034C
	ArenaTeamRosterPacket  uint16 = 0x034E
	ArenaTeamInvitePacket  uint16 = 0x0350
	ArenaTeamEventPacket   uint16 = 0x0357
	ArenaTeamStatsPacket   uint16 = 0x035B

	ClientArenaTeamQuery     ClientOpcode = 0x034B
	ClientArenaTeamRoster    ClientOpcode = 0x034D
	ClientArenaTeamInvite    ClientOpcode = 0x034F
	ClientArenaTeamAccept    ClientOpcode = 0x0351
	ClientArenaTeamDecline   ClientOpcode = 0x0352
	ClientArenaTeamLeave     ClientOpcode = 0x0353
	ClientArenaTeamRemove    ClientOpcode = 0x0354
	ClientArenaTeamDisband   ClientOpcode = 0x0355
	ClientArenaTeamSetLeader ClientOpcode = 0x0356
)

// ArenaTeamInfo is the name and emblem of a team.
type ArenaTeamInfo struct {
	ID, Size                                          uint32
	Name                                              string
	Background, EmblemStyle, EmblemColor, BorderStyle uint32
	BorderColor                                       uint32
}

func ParseArenaTeamQuery(body []byte) (ArenaTeamInfo, error) {
	r := NewReader(body, "SMSG_ARENA_TEAM_QUERY_RESPONSE")
	var info ArenaTeamInfo
	var err error
	if info.ID, err = r.U32(); err != nil {
		return info, err
	}
	if info.Name, err = r.CString(); err != nil {
		return info, err
	}
	for _, field := range []*uint32{&info.Size, &info.Background, &info.EmblemStyle, &info.EmblemColor, &info.BorderStyle, &info.BorderColor} {
		if *field, err = r.U32(); err != nil {
			return info, err
		}
	}
	return info, nil
}

// ArenaTeamMember is a line of a team's roster.
type ArenaTeamMember struct {
	GUID                    uint64
	Online                  bool
	Name                    string
	Rank                    uint32
	Level, Class            uint8
	PlayedWeek, WonWeek     uint32
	PlayedSeason, WonSeason uint32
	PersonalRating          uint32
}

// ArenaTeamRoster is SMSG_ARENA_TEAM_ROSTER.
type ArenaTeamRoster struct {
	ID, Size uint32
	Members  []ArenaTeamMember
}

func ParseArenaTeamRoster(body []byte) (ArenaTeamRoster, error) {
	r := NewReader(body, "SMSG_ARENA_TEAM_ROSTER")
	var roster ArenaTeamRoster
	var err error
	if roster.ID, err = r.U32(); err != nil {
		return roster, err
	}
	extra, err := r.U8()
	if err != nil {
		return roster, err
	}
	count, err := r.U32()
	if err != nil {
		return roster, err
	}
	if roster.Size, err = r.U32(); err != nil {
		return roster, err
	}
	for ; count > 0; count-- {
		var member ArenaTeamMember
		if member.GUID, err = r.U64(); err != nil {
			return roster, err
		}
		online, err := r.U8()
		if err != nil {
			return roster, err
		}
		member.Online = online != 0
		if member.Name, err = r.CString(); err != nil {
			return roster, err
		}
		if member.Rank, err = r.U32(); err != nil {
			return roster, err
		}
		if member.Level, err = r.U8(); err != nil {
			return roster, err
		}
		if member.Class, err = r.U8(); err != nil {
			return roster, err
		}
		for _, field := range []*uint32{&member.PlayedWeek, &member.WonWeek, &member.PlayedSeason, &member.WonSeason, &member.PersonalRating} {
			if *field, err = r.U32(); err != nil {
				return roster, err
			}
		}
		if extra != 0 {
			if err = r.Skip(8); err != nil {
				return roster, err
			}
		}
		roster.Members = append(roster.Members, member)
	}
	return roster, nil
}

// ArenaTeamStats is SMSG_ARENA_TEAM_STATS.
type ArenaTeamStats struct {
	ID                                                 uint32
	Rating, Played, Won, SeasonPlayed, SeasonWon, Rank uint32
}

func ParseArenaTeamStats(body []byte) (ArenaTeamStats, error) {
	r := NewReader(body, "SMSG_ARENA_TEAM_STATS")
	var stats ArenaTeamStats
	for _, field := range []*uint32{&stats.ID, &stats.Rating, &stats.Played, &stats.Won, &stats.SeasonPlayed, &stats.SeasonWon, &stats.Rank} {
		value, err := r.U32()
		if err != nil {
			return stats, err
		}
		*field = value
	}
	return stats, nil
}

// BuildArenaTeamID is the body of the requests that name only a team.
func BuildArenaTeamID(team uint32) []byte { return binary.LittleEndian.AppendUint32(nil, team) }

// BuildArenaTeamName is the body of the requests that name a team and a player.
func BuildArenaTeamName(team uint32, name string) []byte {
	return append(binary.LittleEndian.AppendUint32(nil, team), append([]byte(name), 0)...)
}
