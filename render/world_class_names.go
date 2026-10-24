package render

import (
	"fmt"
	"strings"

	"github.com/MorenoLand/Moreno.WoW/ui"
)

// setLocalizedClassNames fills LOCALIZED_CLASS_NAMES_MALE and _FEMALE from
// ChrClasses.dbc, as the client does before the scripts read them.
func setLocalizedClassNames(rt *ui.Runtime, loader *ui.Loader) {
	table, err := loadCharacterDBC(loader, `DBFilesClient\ChrClasses.dbc`)
	if err != nil || table.fields < 56 {
		return
	}
	var male, female strings.Builder
	for row := 0; row < table.records; row++ {
		token := table.string(row, 55)
		if token == "" {
			continue
		}
		name := table.string(row, 4)
		maleName, femaleName := table.string(row, 38), table.string(row, 21)
		if maleName == "" {
			maleName = name
		}
		if femaleName == "" {
			femaleName = name
		}
		fmt.Fprintf(&female, "[%q]=%q,", token, femaleName)
		fmt.Fprintf(&male, "[%q]=%q,", token, maleName)
	}
	rt.Execute(fmt.Sprintf("LOCALIZED_CLASS_NAMES_FEMALE = {%s}; LOCALIZED_CLASS_NAMES_MALE = {%s}", female.String(), male.String()), "@classnames")
}
