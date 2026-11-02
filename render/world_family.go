package render

import (
	"math"

	"github.com/MorenoLand/Moreno.WoW/ui"
)

// loadCreatureFamilies reads the names of the beast families.
func loadCreatureFamilies(loader *ui.Loader) {
	table, err := loadCharacterDBC(loader, `DBFilesClient\CreatureFamily.dbc`)
	if err != nil || table.fields < 11 {
		return
	}
	for row := 0; row < table.records; row++ {
		worldCreatureFamilyNames[table.value(row, 0)] = table.string(row, 10)
	}
}

// loadBankSlotPrices reads what each bank bag slot costs in copper, in the
// order the slots are bought.
func loadBankSlotPrices(loader *ui.Loader) []uint32 {
	table, err := loadCharacterDBC(loader, `DBFilesClient\BankBagSlotPrices.dbc`)
	if err != nil || table.fields < 2 {
		return nil
	}
	var prices []uint32
	for row := 0; row < table.records; row++ {
		id := int(table.value(row, 0))
		for len(prices) < id {
			prices = append(prices, 0)
		}
		if id >= 1 {
			prices[id-1] = table.value(row, 1)
		}
	}
	return prices
}

// loadExhaustionStates reads the rest states (Exhaustion.dbc: id, name and
// the factor experience is scaled by).
func loadExhaustionStates(loader *ui.Loader) []ui.RestState {
	table, err := loadCharacterDBC(loader, `DBFilesClient\Exhaustion.dbc`)
	if err != nil || table.fields < 7 {
		return nil
	}
	var states []ui.RestState
	for row := 0; row < table.records; row++ {
		states = append(states, ui.RestState{ID: table.value(row, 0), Name: table.string(row, 5), Factor: math.Float32frombits(table.value(row, 2))})
	}
	return states
}

// loadStableTables reads what the stable shows of a pet: the beast families
// with their icon, food and talent type, the foods, the pet talent trees and
// the slot prices.
func loadStableTables(loader *ui.Loader) ui.StableTables {
	tables := ui.StableTables{Families: map[uint32]ui.StableFamily{}}
	if table, err := loadCharacterDBC(loader, `DBFilesClient\CreatureFamily.dbc`); err == nil && table.fields >= 28 {
		for row := 0; row < table.records; row++ {
			tables.Families[table.value(row, 0)] = ui.StableFamily{Icon: table.string(row, 27), Name: table.string(row, 10),
				TalentType: int32(table.value(row, 8)), FoodMask: table.value(row, 7)}
		}
	}
	if table, err := loadCharacterDBC(loader, `DBFilesClient\ItemPetFood.dbc`); err == nil && table.fields >= 2 {
		for row := 0; row < table.records; row++ {
			tables.Foods = append(tables.Foods, ui.StableFood{ID: table.value(row, 0), Name: table.string(row, 1)})
		}
	}
	if table, err := loadCharacterDBC(loader, `DBFilesClient\TalentTab.dbc`); err == nil && table.fields >= 22 {
		for row := 0; row < table.records; row++ {
			tables.TalentTabs = append(tables.TalentTabs, ui.StableTalentTab{Name: table.string(row, 1), Mask: table.value(row, 21)})
		}
	}
	if table, err := loadCharacterDBC(loader, `DBFilesClient\StableSlotPrices.dbc`); err == nil && table.fields >= 2 {
		for row := 0; row < table.records; row++ {
			id := int(table.value(row, 0))
			for len(tables.SlotPrices) <= id {
				tables.SlotPrices = append(tables.SlotPrices, 0)
			}
			tables.SlotPrices[id] = table.value(row, 1)
		}
	}
	return tables
}

// loadLanguages reads the languages a character can speak (Languages.dbc).
func loadLanguages(loader *ui.Loader) []ui.Language {
	table, err := loadCharacterDBC(loader, `DBFilesClient\Languages.dbc`)
	if err != nil || table.fields < 2 {
		return nil
	}
	var languages []ui.Language
	for row := 0; row < table.records; row++ {
		languages = append(languages, ui.Language{ID: table.value(row, 0), Name: table.string(row, 1)})
	}
	return languages
}

// loadEncounters reads the bosses of the dungeons (DungeonEncounter.dbc) with
// the icon each is shown with.
func loadEncounters(loader *ui.Loader) []ui.Encounter {
	table, err := loadCharacterDBC(loader, `DBFilesClient\DungeonEncounter.dbc`)
	if err != nil || table.fields < 23 {
		return nil
	}
	icons := map[uint32]string{}
	if icon, err := loadCharacterDBC(loader, `DBFilesClient\SpellIcon.dbc`); err == nil && icon.fields >= 2 {
		for row := 0; row < icon.records; row++ {
			icons[icon.value(row, 0)] = icon.string(row, 1)
		}
	}
	var list []ui.Encounter
	for row := 0; row < table.records; row++ {
		list = append(list, ui.Encounter{Map: table.value(row, 1), Difficulty: table.value(row, 2), Bit: table.value(row, 4),
			Name: table.string(row, 5), Icon: icons[table.value(row, 22)]})
	}
	return list
}

// loadSurveyTables reads the tables of the game master survey.
func loadSurveyTables(loader *ui.Loader) ui.SurveyTables {
	tables := ui.SurveyTables{Current: map[uint32]uint32{}, Surveys: map[uint32][10]uint32{}, Questions: map[uint32]string{}, Answers: map[uint32][]ui.SurveyAnswer{}}
	if table, err := loadCharacterDBC(loader, `DBFilesClient\GMSurveyCurrentSurvey.dbc`); err == nil && table.fields >= 2 {
		for row := 0; row < table.records; row++ {
			tables.Current[table.value(row, 0)] = table.value(row, 1)
		}
	}
	if table, err := loadCharacterDBC(loader, `DBFilesClient\GMSurveySurveys.dbc`); err == nil && table.fields >= 11 {
		for row := 0; row < table.records; row++ {
			var questions [10]uint32
			for index := range questions {
				questions[index] = table.value(row, 1+index)
			}
			tables.Surveys[table.value(row, 0)] = questions
		}
	}
	if table, err := loadCharacterDBC(loader, `DBFilesClient\GMSurveyQuestions.dbc`); err == nil && table.fields >= 2 {
		for row := 0; row < table.records; row++ {
			tables.Questions[table.value(row, 0)] = table.string(row, 1)
		}
	}
	if table, err := loadCharacterDBC(loader, `DBFilesClient\GMSurveyAnswers.dbc`); err == nil && table.fields >= 4 {
		for row := 0; row < table.records; row++ {
			group := table.value(row, 2)
			tables.Answers[group] = append(tables.Answers[group], ui.SurveyAnswer{Sort: table.value(row, 1), Text: table.string(row, 3)})
		}
	}
	return tables
}
