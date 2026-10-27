package world

import "encoding/binary"

// Channel packets.
const (
	ChannelNotifyPacket uint16 = 0x0099
	ChannelListPacket   uint16 = 0x009B

	ClientJoinChannel     ClientOpcode = 0x0097
	ClientLeaveChannel    ClientOpcode = 0x0098
	ClientChannelList     ClientOpcode = 0x009A
	ClientChannelPassword ClientOpcode = 0x009C
	ClientChannelSetOwner ClientOpcode = 0x009D
	ClientChannelOwner    ClientOpcode = 0x009E
	ClientChannelModerate ClientOpcode = 0x009F
	ClientChannelUnmod    ClientOpcode = 0x00A0
	ClientChannelMute     ClientOpcode = 0x00A1
	ClientChannelUnmute   ClientOpcode = 0x00A2
	ClientChannelInvite   ClientOpcode = 0x00A3
	ClientChannelKick     ClientOpcode = 0x00A4
	ClientChannelBan      ClientOpcode = 0x00A5
	ClientChannelUnban    ClientOpcode = 0x00A6
	ClientChannelAnnounce ClientOpcode = 0x00A7
	ClientChannelDisplay  ClientOpcode = 0x03D2
)

// Notice kinds of SMSG_CHANNEL_NOTIFY.
const (
	ChannelJoinedNotice        uint8 = 0x00
	ChannelLeftNotice          uint8 = 0x01
	ChannelYouJoined           uint8 = 0x02
	ChannelYouLeft             uint8 = 0x03
	ChannelWrongPassword       uint8 = 0x04
	ChannelNotMember           uint8 = 0x05
	ChannelNotModerator        uint8 = 0x06
	ChannelPasswordChanged     uint8 = 0x07
	ChannelOwnerChanged        uint8 = 0x08
	ChannelPlayerNotFound      uint8 = 0x09
	ChannelNotOwner            uint8 = 0x0A
	ChannelChannelOwner        uint8 = 0x0B
	ChannelModeChange          uint8 = 0x0C
	ChannelAnnouncementsOn     uint8 = 0x0D
	ChannelAnnouncementsOff    uint8 = 0x0E
	ChannelModerationOn        uint8 = 0x0F
	ChannelModerationOff       uint8 = 0x10
	ChannelMuted               uint8 = 0x11
	ChannelPlayerKicked        uint8 = 0x12
	ChannelBanned              uint8 = 0x13
	ChannelPlayerBanned        uint8 = 0x14
	ChannelPlayerUnbanned      uint8 = 0x15
	ChannelPlayerNotBanned     uint8 = 0x16
	ChannelPlayerAlreadyMember uint8 = 0x17
	ChannelInvite              uint8 = 0x18
	ChannelInviteWrongFaction  uint8 = 0x19
	ChannelWrongFaction        uint8 = 0x1A
	ChannelInvalidName         uint8 = 0x1B
	ChannelNotModerated        uint8 = 0x1C
	ChannelPlayerInvited       uint8 = 0x1D
	ChannelPlayerInviteBanned  uint8 = 0x1E
	ChannelThrottled           uint8 = 0x1F
	ChannelNotInArea           uint8 = 0x20
	ChannelNotInLFG            uint8 = 0x21
	ChannelVoiceOn             uint8 = 0x22
	ChannelVoiceOff            uint8 = 0x23
)

// ChannelNotify is SMSG_CHANNEL_NOTIFY.
type ChannelNotify struct {
	Kind    uint8
	Channel string
	// Player and Other are the guids a notice names.
	Player, Other uint64
	// Flags and ID belong to YOU_JOINED (the channel's flags and its row of
	// ChatChannels.dbc), Instance is the number that follows them.
	Flags    uint8
	ID       uint32
	Instance uint32
	// OldFlags and NewFlags belong to MODE_CHANGE.
	OldFlags, NewFlags uint8
	// Name is the player a notice names by text.
	Name string
	// Text is the message that follows a YOU_JOINED notice.
	Text string
	// Suspended is the flag of a YOU_LEFT notice that keeps the channel.
	Suspended bool
}

// ParseChannelNotify reads SMSG_CHANNEL_NOTIFY.
func ParseChannelNotify(body []byte) (ChannelNotify, error) {
	r := NewReader(body, "SMSG_CHANNEL_NOTIFY")
	var notify ChannelNotify
	var err error
	if notify.Kind, err = r.U8(); err != nil {
		return notify, err
	}
	if notify.Channel, err = r.CString(); err != nil {
		return notify, err
	}
	switch notify.Kind {
	case ChannelJoinedNotice, ChannelLeftNotice, ChannelPasswordChanged, ChannelOwnerChanged,
		ChannelAnnouncementsOn, ChannelAnnouncementsOff, ChannelModerationOn, ChannelModerationOff,
		ChannelPlayerAlreadyMember, ChannelInvite, ChannelVoiceOn, ChannelVoiceOff:
		notify.Player, err = r.U64()
	case ChannelYouJoined:
		if notify.Flags, err = r.U8(); err != nil {
			return notify, err
		}
		if notify.ID, err = r.U32(); err != nil {
			return notify, err
		}
		if notify.Instance, err = r.U32(); err != nil {
			return notify, err
		}
		if r.Remaining() > 0 {
			notify.Text, err = r.CString()
		}
	case ChannelYouLeft:
		if notify.ID, err = r.U32(); err != nil {
			return notify, err
		}
		var suspended uint8
		suspended, err = r.U8()
		notify.Suspended = suspended != 0
	case ChannelPlayerNotFound, ChannelChannelOwner, ChannelPlayerNotBanned, ChannelPlayerInvited, ChannelPlayerInviteBanned:
		notify.Name, err = r.CString()
	case ChannelModeChange:
		if notify.Player, err = r.U64(); err != nil {
			return notify, err
		}
		if notify.OldFlags, err = r.U8(); err != nil {
			return notify, err
		}
		notify.NewFlags, err = r.U8()
	case ChannelPlayerKicked, ChannelPlayerBanned, ChannelPlayerUnbanned:
		if notify.Player, err = r.U64(); err != nil {
			return notify, err
		}
		notify.Other, err = r.U64()
	}
	return notify, err
}

// ChannelMember is a member in SMSG_CHANNEL_LIST.
type ChannelMember struct {
	GUID  uint64
	Flags uint8
}

// ChannelRoster is SMSG_CHANNEL_LIST.
type ChannelRoster struct {
	Kind    uint8
	Channel string
	Flags   uint8
	Members []ChannelMember
}

// ParseChannelList reads SMSG_CHANNEL_LIST.
func ParseChannelList(body []byte) (ChannelRoster, error) {
	r := NewReader(body, "SMSG_CHANNEL_LIST")
	var roster ChannelRoster
	var err error
	if roster.Kind, err = r.U8(); err != nil {
		return roster, err
	}
	if roster.Channel, err = r.CString(); err != nil {
		return roster, err
	}
	if roster.Flags, err = r.U8(); err != nil {
		return roster, err
	}
	count, err := r.U32()
	if err != nil {
		return roster, err
	}
	for ; count > 0 && r.Remaining() >= 9; count-- {
		var member ChannelMember
		if member.GUID, err = r.U64(); err != nil {
			return roster, err
		}
		if member.Flags, err = r.U8(); err != nil {
			return roster, err
		}
		roster.Members = append(roster.Members, member)
	}
	return roster, nil
}

// BuildJoinChannel is the body of CMSG_JOIN_CHANNEL: the channel's row, two
// flags (the second asks for voice), its name and the password.
func BuildJoinChannel(id uint32, first, second bool, name, password string) []byte {
	body := binary.LittleEndian.AppendUint32(nil, id)
	body = append(body, boolByte(first), boolByte(second))
	body = append(body, cstring(name)...)
	body = append(body, 0)
	body = append(body, cstring(password)...)
	return append(body, 0)
}

// BuildLeaveChannel is the body of CMSG_LEAVE_CHANNEL.
func BuildLeaveChannel(id uint32, name string) []byte {
	body := binary.LittleEndian.AppendUint32(nil, id)
	body = append(body, cstring(name)...)
	return append(body, 0)
}

// BuildChannelCommand is the body of the channel commands that name a channel
// and optionally a player or password.
func BuildChannelCommand(channel string, arguments ...string) []byte {
	body := append(cstring(channel), 0)
	for _, argument := range arguments {
		body = append(body, cstring(argument)...)
		body = append(body, 0)
	}
	return body
}
