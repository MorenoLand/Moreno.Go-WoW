package render

import (
	"github.com/MorenoLand/Moreno.WoW/ui"
	"github.com/MorenoLand/Moreno.WoW/world"
)

// PLAYER_FIELD_GLYPH_SLOTS_1 holds the glyph slot of each of six sockets,
// PLAYER_FIELD_GLYPHS_1 the glyph in each and PLAYER_GLYPHS_ENABLED the mask of
// the sockets that are open.
const (
	fieldGlyphSlots   = 1312
	fieldGlyphs       = 1318
	fieldGlyphEnabled = 1324
	glyphSocketCount  = 6
)

// worldGlyphTables read GlyphProperties.dbc and GlyphSlot.dbc once.
type worldGlyphTables struct {
	loaded bool
	tables ui.GlyphTables
}

func (glyphs *worldGlyphTables) load(loader *ui.Loader) {
	if glyphs.loaded {
		return
	}
	glyphs.loaded = true
	glyphs.tables = ui.GlyphTables{Properties: make(map[uint32]ui.GlyphProperty), Slots: make(map[uint32]int)}
	icons := make(map[uint32]string)
	if table, err := loadCharacterDBC(loader, `DBFilesClient\SpellIcon.dbc`); err == nil {
		for row := 0; row < table.records; row++ {
			icons[table.value(row, 0)] = table.string(row, 1)
		}
	}
	if table, err := loadCharacterDBC(loader, `DBFilesClient\GlyphProperties.dbc`); err == nil && table.fields >= 4 {
		for row := 0; row < table.records; row++ {
			glyphs.tables.Properties[table.value(row, 0)] = ui.GlyphProperty{Spell: table.value(row, 1), Type: int(table.value(row, 2)), Icon: icons[table.value(row, 3)]}
		}
	}
	if table, err := loadCharacterDBC(loader, `DBFilesClient\GlyphSlot.dbc`); err == nil && table.fields >= 2 {
		for row := 0; row < table.records; row++ {
			glyphs.tables.Slots[table.value(row, 0)] = int(table.value(row, 1))
		}
	}
}

// syncWorldGlyphs hands the interface the glyph fields of the player when they
// change.
func syncWorldGlyphs(rt *ui.Runtime, loader *ui.Loader, glyphs *worldGlyphTables, player *worldEntity, last *[2*glyphSocketCount + 1]uint32) {
	var current [2*glyphSocketCount + 1]uint32
	for socket := 0; socket < glyphSocketCount; socket++ {
		current[socket] = player.fields[uint16(fieldGlyphSlots+socket)]
		current[glyphSocketCount+socket] = player.fields[uint16(fieldGlyphs+socket)]
	}
	current[2*glyphSocketCount] = player.fields[fieldGlyphEnabled]
	if current == *last {
		return
	}
	*last = current
	if !glyphs.loaded {
		glyphs.load(loader)
		rt.SetGlyphTables(glyphs.tables)
	}
	var slots, held [glyphSocketCount]uint32
	copy(slots[:], current[:glyphSocketCount])
	copy(held[:], current[glyphSocketCount:2*glyphSocketCount])
	rt.SetGlyphFields(slots, held, current[2*glyphSocketCount])
}

func (h *clientHost) UseGlyphItem(bag, slot uint8, spellID uint32, itemGUID uint64, socket uint32) error {
	session, err := h.worldSession()
	if err != nil {
		return err
	}
	return session.SendRaw(world.UseItemOpcode, world.BuildUseItemGlyph(bag, slot, 1, spellID, itemGUID, socket, 0, 0))
}

func (h *clientHost) RemoveGlyph(socket uint32) error {
	session, err := h.worldSession()
	if err != nil {
		return err
	}
	return session.SendRaw(world.RemoveGlyphOpcode, world.BuildRemoveGlyph(socket))
}

// PLAYER_FIELD_KNOWN_CURRENCIES is two fields wide; honor and arena points
// follow in their own fields.
const (
	fieldKnownCurrencies = 632
	fieldHonorCurrency   = 1277
	fieldArenaCurrency   = 1278
)

// worldCurrencyTables read CurrencyCategory.dbc and CurrencyTypes.dbc once.
type worldCurrencyTables struct {
	loaded bool
}

// syncWorldCurrencies hands the interface the currency fields of the player.
func syncWorldCurrencies(rt *ui.Runtime, loader *ui.Loader, tables *worldCurrencyTables, player *worldEntity) {
	if !tables.loaded {
		tables.loaded = true
		var data ui.CurrencyTables
		if table, err := loadCharacterDBC(loader, `DBFilesClient\CurrencyCategory.dbc`); err == nil && table.fields >= 3 {
			for row := 0; row < table.records; row++ {
				data.Categories = append(data.Categories, ui.CurrencyCategory{ID: table.value(row, 0), Flags: table.value(row, 1), Name: table.string(row, 2)})
			}
		}
		if table, err := loadCharacterDBC(loader, `DBFilesClient\CurrencyTypes.dbc`); err == nil && table.fields >= 4 {
			for row := 0; row < table.records; row++ {
				data.Types = append(data.Types, ui.CurrencyType{ID: table.value(row, 0), Item: table.value(row, 1), Category: table.value(row, 2), Bit: int(table.value(row, 3))})
			}
		}
		rt.SetCurrencyTables(data)
	}
	rt.SetCurrencyFields([2]uint32{player.fields[fieldKnownCurrencies], player.fields[fieldKnownCurrencies+1]}, player.fields[fieldHonorCurrency], player.fields[fieldArenaCurrency])
}
