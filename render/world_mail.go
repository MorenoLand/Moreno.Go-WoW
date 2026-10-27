package render

import (
	"encoding/binary"
	"math"
	"sort"
	"strings"

	"github.com/MorenoLand/Moreno.WoW/network"
	"github.com/MorenoLand/Moreno.WoW/ui"
	"github.com/MorenoLand/Moreno.WoW/world"
)

// The query of when mail next arrives; the server answers with the same opcode.
const (
	clientQueryNextMailTime world.ClientOpcode = 0x0284
	serverNextMailTime      uint16             = 0x0284
)

// guildBankTabPrices are the gold the guild leader pays for the first to the
// sixth tab.
var guildBankTabPrices = [...]uint32{100, 250, 500, 1000, 2500, 5000}

type worldStationery struct {
	item    uint32
	texture string
}

// worldMailData is what the client's tables say about mail and the lists the
// bank tabs are named by.
type worldMailData struct {
	stationery map[int32]worldStationery
	auctions   map[uint32]string
	// depositRates are the deposit rates of the auction houses, in percent.
	depositRates map[uint32]float64
}

func loadWorldMailData(loader *ui.Loader) *worldMailData {
	data := &worldMailData{stationery: map[int32]worldStationery{}, auctions: map[uint32]string{}, depositRates: map[uint32]float64{}}
	if table, err := loadCharacterDBC(loader, `DBFilesClient\Stationery.dbc`); err == nil && table.fields >= 3 {
		for row := 0; row < table.records; row++ {
			data.stationery[int32(table.value(row, 0))] = worldStationery{item: table.value(row, 1), texture: table.string(row, 2)}
		}
	}
	if table, err := loadCharacterDBC(loader, `DBFilesClient\AuctionHouse.dbc`); err == nil && table.fields >= 5 {
		for row := 0; row < table.records; row++ {
			data.auctions[table.value(row, 0)] = table.string(row, 4)
			data.depositRates[table.value(row, 0)] = float64(table.value(row, 2))
		}
	}
	return data
}

// loadMacroIcons lists the icon textures a macro or a bank tab can show.
func loadMacroIcons(loader *ui.Loader) []string {
	seen := map[string]bool{}
	var icons []string
	add := func(path string) {
		key := strings.ToLower(path)
		if path != "" && !seen[key] {
			seen[key] = true
			icons = append(icons, path)
		}
	}
	if table, err := loadCharacterDBC(loader, `DBFilesClient\SpellIcon.dbc`); err == nil && table.fields >= 2 {
		for row := 0; row < table.records; row++ {
			add(table.string(row, 1))
		}
	}
	if table, err := loadCharacterDBC(loader, `DBFilesClient\ItemDisplayInfo.dbc`); err == nil && table.fields >= 6 {
		for row := 0; row < table.records; row++ {
			if name := table.string(row, 5); name != "" {
				add(`Interface\Icons\` + name)
			}
		}
	}
	sort.Slice(icons, func(a, b int) bool { return strings.ToLower(icons[a]) < strings.ToLower(icons[b]) })
	return append([]string{`Interface\Icons\INV_Misc_QuestionMark`}, icons...)
}

// handleMailPacket applies a packet about the mailbox or the guild bank; it
// reports whether it was one.
func handleMailPacket(rt *ui.Runtime, packet world.Packet) bool {
	switch packet.Opcode {
	case world.MailList:
		if listing, err := world.ParseMailList(packet.Body); err == nil {
			rt.SetMailList(listing)
		}
	case world.ShowMailbox:
		if guid, err := world.NewReader(packet.Body, "SMSG_SHOW_MAILBOX").U64(); err == nil {
			rt.OpenMailbox(guid)
		}
	case world.MailResult:
		if result, err := world.ParseMailResult(packet.Body); err == nil {
			rt.ApplyMailResult(result)
		}
	case world.MailReceived:
		rt.SetPendingMail(true)
	case serverNextMailTime:
		if next, err := world.NewReader(packet.Body, "MSG_QUERY_NEXT_MAIL_TIME").F32(); err == nil {
			rt.SetPendingMail(next >= 0)
		}
	case world.GuildBankList:
		if list, err := world.ParseGuildBankList(packet.Body); err == nil {
			rt.SetGuildBankList(list)
		}
	case world.GuildBankLogResult:
		if log, err := world.ParseGuildBankLog(packet.Body); err == nil {
			rt.SetGuildBankLog(log)
		}
	case world.GuildBankTextReply:
		if tab, text, err := world.ParseGuildBankText(packet.Body); err == nil {
			rt.SetGuildBankText(tab, text)
		}
	case world.GuildPermissionsReply:
		if info, err := world.ParseGuildPermissions(packet.Body); err == nil {
			rt.SetGuildPermissions(info)
		}
	case world.GuildBankMoneyLeft:
		if left, err := world.ParseGuildBankMoneyLeft(packet.Body); err == nil {
			rt.SetGuildBankMoneyLeft(left)
		}
	case world.AuctionHello:
		if info, err := world.ParseAuctionHello(packet.Body); err == nil {
			rt.OpenAuctionHouse(info)
		}
	case world.AuctionCommandResult:
		if result, err := world.ParseAuctionResult(packet.Body); err == nil {
			rt.ApplyAuctionResult(result)
		}
	case world.AuctionListResult, world.AuctionOwnerResult, world.AuctionBidderResult:
		if list, err := world.ParseAuctionList(packet.Body); err == nil {
			if kind, ok := ui.AuctionListKind(packet.Opcode); ok {
				rt.SetAuctionList(kind, list)
			}
		}
	case world.InspectTalentReply:
		if guid, talents, err := world.ParseInspectTalent(packet.Body); err == nil {
			rt.SetInspectedTalents(guid, talents)
		}
	case world.AllAchievementData:
		if data, err := world.ParseAllAchievementData(packet.Body); err == nil {
			rt.SetAchievementData(data)
		}
	case world.RespondInspectAchieves:
		if data, err := world.ParseInspectAchievements(packet.Body); err == nil {
			rt.SetInspectedAchievements(data)
		}
	case world.AchievementEarned:
		if info, err := world.ParseAchievementEarned(packet.Body); err == nil {
			rt.EarnAchievement(info)
		}
	case world.CriteriaUpdate:
		if progress, err := world.ParseCriteriaUpdate(packet.Body); err == nil {
			rt.UpdateCriteria(progress)
		}
	default:
		return false
	}
	return true
}

func (h *clientHost) InspectAchievements(guid uint64) error {
	return h.sendMail(world.ClientQueryInspectAchievements, world.BuildInspectAchievements(guid))
}

// loadAchievementTables reads the achievements, their categories and criteria.
func loadAchievementTables(loader *ui.Loader) ui.AchievementTables {
	tables := ui.AchievementTables{Achievements: map[uint32]*ui.AchievementEntry{}, Categories: map[uint32]*ui.AchievementCategory{}, Criteria: map[uint32]*ui.AchievementCriterion{}}
	icons := map[uint32]string{}
	if table, err := loadCharacterDBC(loader, `DBFilesClient\SpellIcon.dbc`); err == nil && table.fields >= 2 {
		for row := 0; row < table.records; row++ {
			icons[table.value(row, 0)] = table.string(row, 1)
		}
	}
	if table, err := loadCharacterDBC(loader, `DBFilesClient\Achievement.dbc`); err == nil && table.fields >= 62 {
		for row := 0; row < table.records; row++ {
			entry := &ui.AchievementEntry{
				ID: table.value(row, 0), Faction: int32(table.value(row, 1)), Map: int32(table.value(row, 2)), Previous: table.value(row, 3),
				Name: table.string(row, 4), Description: table.string(row, 21), Category: table.value(row, 38), Points: table.value(row, 39),
				Order: table.value(row, 40), Flags: table.value(row, 41), Icon: icons[table.value(row, 42)], Reward: table.string(row, 43),
			}
			tables.Achievements[entry.ID] = entry
		}
	}
	if table, err := loadCharacterDBC(loader, `DBFilesClient\Achievement_Category.dbc`); err == nil && table.fields >= 20 {
		for row := 0; row < table.records; row++ {
			category := &ui.AchievementCategory{ID: table.value(row, 0), Parent: int32(table.value(row, 1)), Name: table.string(row, 2), Order: table.value(row, 19)}
			tables.Categories[category.ID] = category
		}
	}
	if table, err := loadCharacterDBC(loader, `DBFilesClient\Achievement_Criteria.dbc`); err == nil && table.fields >= 31 {
		for row := 0; row < table.records; row++ {
			criterion := &ui.AchievementCriterion{
				ID: table.value(row, 0), Achievement: table.value(row, 1), Type: table.value(row, 2), Asset: table.value(row, 3), Quantity: table.value(row, 4),
				Name: table.string(row, 9), Flags: table.value(row, 26), Order: table.value(row, 30),
			}
			tables.Criteria[criterion.ID] = criterion
		}
	}
	return tables
}

func (h *clientHost) sendMail(opcode world.ClientOpcode, body []byte) error {
	session, err := h.worldSession()
	if err != nil {
		return err
	}
	return session.SendRaw(opcode, body)
}

func (h *clientHost) RequestMailList(mailbox uint64) error {
	return h.sendMail(world.ClientGetMailList, world.BuildMailbox(mailbox))
}

func (h *clientHost) MarkMailRead(mailbox uint64, mail uint32) error {
	return h.sendMail(world.ClientMailMarkAsRead, world.BuildMailAction(mailbox, mail))
}

func (h *clientHost) TakeMailMoney(mailbox uint64, mail uint32) error {
	return h.sendMail(world.ClientMailTakeMoney, world.BuildMailAction(mailbox, mail))
}

func (h *clientHost) TakeMailItem(mailbox uint64, mail, attachment uint32) error {
	return h.sendMail(world.ClientMailTakeItem, world.BuildMailTakeItem(mailbox, mail, attachment))
}

func (h *clientHost) DeleteMail(mailbox uint64, mail uint32) error {
	return h.sendMail(world.ClientMailDelete, world.BuildMailDelete(mailbox, mail))
}

func (h *clientHost) ReturnMail(mailbox uint64, mail uint32, sender uint64) error {
	return h.sendMail(world.ClientMailReturnToSender, world.BuildMailReturn(mailbox, mail, sender))
}

func (h *clientHost) CopyMailLetter(mailbox uint64, mail uint32) error {
	return h.sendMail(world.ClientMailCreateTextItem, world.BuildMailAction(mailbox, mail))
}

func (h *clientHost) SendMail(mailbox uint64, target, subject, body string, stationery uint32, items []world.MailAttachmentSend, money, cod uint32) error {
	return h.sendMail(world.ClientSendMail, world.BuildSendMail(mailbox, target, subject, body, stationery, 0, items, money, cod))
}

// playerName is a player's name, asking the server the first time it is wanted.
func (h *clientHost) playerName(guid uint64) string {
	if h.names == nil || guid == 0 {
		return ""
	}
	if name, ok := h.names.players[guid]; ok {
		return name.Name
	}
	if !h.names.asked[guid] {
		h.names.asked[guid] = true
		_ = h.sendMail(world.NameQueryOpcode, world.BuildNameQuery(guid))
	}
	return ""
}

func (h *clientHost) MailSenderName(kind uint8, id uint64) string {
	switch kind {
	case world.MailFromPlayer:
		return h.playerName(id)
	case world.MailFromCreature:
		if h.names == nil {
			return ""
		}
		entry := uint32(id)
		if creature, ok := h.names.creatures[entry]; ok {
			return creature.Name
		}
		if key := uint64(1)<<63 | uint64(entry); !h.names.asked[key] {
			h.names.asked[key] = true
			_ = h.sendMail(world.CreatureQueryOpcode, world.BuildCreatureQuery(entry, 0))
		}
	case world.MailFromAuction:
		if h.mailData != nil {
			return h.mailData.auctions[uint32(id)]
		}
	}
	return ""
}

func (h *clientHost) MailStationery(id int32) (uint32, string) {
	if h.mailData == nil {
		return 0, ""
	}
	stationery := h.mailData.stationery[id]
	return stationery.item, stationery.texture
}

func (h *clientHost) GuildBankSend(opcode world.ClientOpcode, body []byte) error {
	return h.sendMail(opcode, body)
}

func (h *clientHost) GuildMemberName(guid uint64) string { return h.playerName(guid) }

func (h *clientHost) GuildBankTabCost(purchased int) uint32 {
	if purchased < 0 || purchased >= len(guildBankTabPrices) {
		return 0
	}
	return guildBankTabPrices[purchased] * 10000
}

// Game object types the client opens itself rather than asking the server to use.
const (
	gameObjectMailbox   = 19
	gameObjectGuildBank = 34
	// gameObjectUseRange is how near the player must be to open them.
	gameObjectUseRange                     = 10
	clientGameObjectUse world.ClientOpcode = 0x00B1
)

// useWorldGameObject does what a right click on a game object does: a mailbox
// and a guild bank open their windows, anything else is used on the server.
func useWorldGameObject(rt *ui.Runtime, session *network.Session, object, player *worldEntity) {
	if object == nil || session == nil {
		return
	}
	kind := (object.fields[gameObjectBytes1Field] >> 8) & 0xFF
	if kind == gameObjectMailbox || kind == gameObjectGuildBank {
		if player != nil {
			dx, dy := object.movement.Position.X-player.movement.Position.X, object.movement.Position.Y-player.movement.Position.Y
			dz := object.movement.Position.Z - player.movement.Position.Z
			if math.Sqrt(float64(dx*dx+dy*dy+dz*dz)) > gameObjectUseRange {
				rt.ShowGlobalError("ERR_USE_TOO_FAR")
				return
			}
		}
		if kind == gameObjectMailbox {
			// The server answers a use of a mailbox by showing it.
			body := make([]byte, 8)
			binary.LittleEndian.PutUint64(body, object.guid)
			_ = session.SendRaw(clientGameObjectUse, body)
			rt.OpenMailbox(object.guid)
		} else {
			rt.OpenGuildBank(object.guid)
		}
		return
	}
	body := make([]byte, 8)
	binary.LittleEndian.PutUint64(body, object.guid)
	_ = session.SendRaw(clientGameObjectUse, body)
}

func (h *clientHost) AuctionSend(opcode world.ClientOpcode, body []byte) error {
	return h.sendMail(opcode, body)
}

func (h *clientHost) AuctionPlayerName(guid uint64) string { return h.playerName(guid) }

func (h *clientHost) AuctionDepositRate(house uint32) float64 {
	if h.mailData == nil {
		return 0
	}
	return h.mailData.depositRates[house]
}

func (h *clientHost) Inspect(guid uint64) error {
	return h.sendMail(world.ClientInspect, world.BuildInspect(guid))
}

// UnitVisibleItem is the item another unit is seen wearing in a slot.
func (h *clientHost) UnitVisibleItem(guid uint64, slot int) uint32 {
	if h.entities == nil || slot < 1 || slot > visibleItemSlotsReadable {
		return 0
	}
	entity := h.entities()[guid]
	if entity == nil {
		return 0
	}
	return entity.fields[uint16(fieldPlayerVisibleItem1+(slot-1)*2)]
}

// closeDistantWindows closes the mailbox, the guild bank and the auction house
// when the player has walked away from them.
func closeDistantWindows(rt *ui.Runtime, entities map[uint64]*worldEntity, player *worldEntity) {
	mailbox, bank, auctioneer := rt.InteractionGUIDs()
	if player == nil || (mailbox == 0 && bank == 0 && auctioneer == 0) {
		return
	}
	far := func(guid uint64) bool {
		entity := entities[guid]
		if entity == nil {
			return true
		}
		dx, dy, dz := entity.movement.Position.X-player.movement.Position.X, entity.movement.Position.Y-player.movement.Position.Y, entity.movement.Position.Z-player.movement.Position.Z
		return math.Sqrt(float64(dx*dx+dy*dy+dz*dz)) > gameObjectUseRange
	}
	if mailbox != 0 && far(mailbox) {
		rt.CloseMailbox()
	}
	if bank != 0 && far(bank) {
		rt.CloseGuildBank()
	}
	if auctioneer != 0 && far(auctioneer) {
		rt.CloseAuctionHouse()
	}
}
