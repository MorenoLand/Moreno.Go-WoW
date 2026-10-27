package ui

import "regexp"

// genericForNext finds `for k, v in next, t do`: the loop is given no third
// value, and the Lua compiler leaves the register of that value holding what
// the loop before it ended with, so a second walk over a table starts from the
// last key of the first and finds nothing.
var genericForNext = regexp.MustCompile(`(\bin\s+(?:Secure)?[nN]ext\s*,\s*)([A-Za-z_][\w\.:]*(?:\([^()]*\))?(?:\[[^\]\n]*\])*(?:\.[A-Za-z_]\w*)*)(\s+do\b)`)

// fixGenericFor gives such loops the explicit nil the language says they start
// from.
func fixGenericFor(source string) string {
	return genericForNext.ReplaceAllString(source, "${1}${2}, nil${3}")
}
