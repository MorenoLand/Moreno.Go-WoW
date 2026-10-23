package ui

import (
	"fmt"
	"math"
	"strconv"
	"strings"
)

// spellTextContext is what the $-variables in a Spell.dbc description refer to.
type spellTextContext struct {
	catalog map[uint32]SpellInfo
	known   func(id uint32) bool
	level   int
	male    bool
}

// formatSpellDuration writes a duration the way the tooltip variables do:
// "10 sec", "1.5 min", "2 hr".
func formatSpellDuration(ms int) string {
	seconds := float64(ms) / 1000
	switch {
	case seconds >= 3600:
		return fmt.Sprintf("%s hr", trimNumber(seconds/3600))
	case seconds >= 60:
		return fmt.Sprintf("%s min", trimNumber(seconds/60))
	}
	return fmt.Sprintf("%s sec", trimNumber(seconds))
}

func trimNumber(value float64) string {
	return strconv.FormatFloat(math.Round(value*100)/100, 'f', -1, 64)
}

func (context spellTextContext) effectPoints(info SpellInfo, effect int) (low, high int) {
	e := info.Effects[effect]
	base := float64(e.BasePoints)
	if e.PointsPerLevel != 0 && context.level > 0 {
		level := context.level
		if info.MaxLevel > 0 && level > info.MaxLevel {
			level = info.MaxLevel
		}
		if level > info.BaseLevel {
			base += e.PointsPerLevel * float64(level-info.BaseLevel)
		}
	}
	low = int(base) + 1
	high = int(base) + e.DieSides
	if e.DieSides <= 1 {
		high = low
	}
	return low, high
}

// expand replaces the variables of a spell description.
func (context spellTextContext) expand(text string, info SpellInfo) string {
	var out strings.Builder
	lastNumber := 0.0
	runes := []rune(text)
	for i := 0; i < len(runes); i++ {
		if runes[i] != '$' || i+1 >= len(runes) {
			out.WriteRune(runes[i])
			continue
		}
		i++
		switch runes[i] {
		case '$':
			out.WriteRune('$')
			continue
		case 'l', 'L':
			// $lsingular:plural;
			end := indexRune(runes, i+1, ';')
			if end < 0 {
				out.WriteString("$l")
				continue
			}
			parts := strings.SplitN(string(runes[i+1:end]), ":", 2)
			if len(parts) == 2 {
				if lastNumber == 1 {
					out.WriteString(parts[0])
				} else {
					out.WriteString(parts[1])
				}
			}
			i = end
			continue
		case 'g', 'G':
			end := indexRune(runes, i+1, ';')
			if end < 0 {
				out.WriteString("$g")
				continue
			}
			parts := strings.SplitN(string(runes[i+1:end]), ":", 2)
			if len(parts) == 2 {
				if context.male {
					out.WriteString(strings.TrimSpace(parts[0]))
				} else {
					out.WriteString(strings.TrimSpace(parts[1]))
				}
			}
			i = end
			continue
		case '{':
			// ${expression} with an optional .N for the decimals shown.
			end := indexRune(runes, i+1, '}')
			if end < 0 {
				out.WriteString("$")
				i--
				continue
			}
			value := context.evaluateSpellExpression(string(runes[i+1:end]), info)
			decimals := -1
			if end+2 < len(runes) && runes[end+1] == '.' && runes[end+2] >= '0' && runes[end+2] <= '9' {
				decimals = int(runes[end+2] - '0')
				end += 2
			}
			if decimals >= 0 {
				out.WriteString(strconv.FormatFloat(value, 'f', decimals, 64))
			} else {
				out.WriteString(trimNumber(value))
			}
			lastNumber = value
			i = end
			continue
		case '/', '*':
			// $/N;variable and $*N;variable scale the variable that follows.
			end := indexRune(runes, i+1, ';')
			if end < 0 {
				out.WriteString("$")
				i--
				continue
			}
			factor, _ := strconv.ParseFloat(string(runes[i+1:end]), 64)
			value, next, ok := context.expandVariableValue(runes, end+1, info)
			if !ok {
				out.WriteString("$")
				i--
				continue
			}
			if runes[i] == '/' && factor != 0 {
				value /= factor
			} else if runes[i] == '*' {
				value *= factor
			}
			out.WriteString(trimNumber(value))
			lastNumber = value
			i = next - 1
			continue
		case '?':
			// $?s12345[shown if known][shown otherwise]
			j := i + 1
			if j < len(runes) && (runes[j] == 's' || runes[j] == 'a' || runes[j] == 'A') {
				kind := runes[j]
				j++
				k := j
				for k < len(runes) && runes[k] >= '0' && runes[k] <= '9' {
					k++
				}
				id, _ := strconv.Atoi(string(runes[j:k]))
				first, afterFirst := bracketGroup(runes, k)
				second, afterSecond := bracketGroup(runes, afterFirst)
				condition := false
				if kind == 's' && context.known != nil {
					condition = context.known(uint32(id))
				}
				if condition {
					out.WriteString(context.expand(first, info))
				} else {
					out.WriteString(context.expand(second, info))
				}
				i = afterSecond - 1
				continue
			}
		}
		// $<spell id><variable><effect> or $<variable><effect>
		target := info
		j := i
		if runes[j] >= '0' && runes[j] <= '9' {
			k := j
			for k < len(runes) && runes[k] >= '0' && runes[k] <= '9' {
				k++
			}
			id, _ := strconv.Atoi(string(runes[j:k]))
			if other, ok := context.catalog[uint32(id)]; ok {
				target = other
			}
			j = k
		}
		if j >= len(runes) {
			out.WriteRune('$')
			continue
		}
		variable := runes[j]
		effect := 0
		if j+1 < len(runes) && runes[j+1] >= '1' && runes[j+1] <= '3' {
			effect = int(runes[j+1] - '1')
			j++
		}
		value, text, ok := context.variable(target, variable, effect)
		if !ok {
			out.WriteRune('$')
			out.WriteRune(runes[i])
			continue
		}
		if text != "" {
			out.WriteString(text)
		} else {
			out.WriteString(trimNumber(value))
			lastNumber = value
		}
		i = j
	}
	return out.String()
}

func indexRune(runes []rune, from int, target rune) int {
	for index := from; index < len(runes); index++ {
		if runes[index] == target {
			return index
		}
	}
	return -1
}

// bracketGroup reads "[...]" at runes[from:] and returns its content and the
// index after the closing bracket.
func bracketGroup(runes []rune, from int) (string, int) {
	if from >= len(runes) || runes[from] != '[' {
		return "", from
	}
	depth := 0
	for index := from; index < len(runes); index++ {
		switch runes[index] {
		case '[':
			depth++
		case ']':
			depth--
			if depth == 0 {
				return string(runes[from+1 : index]), index + 1
			}
		}
	}
	return string(runes[from+1:]), len(runes)
}

func (context spellTextContext) variable(info SpellInfo, variable rune, effect int) (float64, string, bool) {
	e := info.Effects[effect]
	switch variable {
	case 's', 'S':
		low, high := context.effectPoints(info, effect)
		if low == high {
			return float64(absInt(low)), "", true
		}
		return 0, fmt.Sprintf("%d to %d", absInt(low), absInt(high)), true
	case 'm':
		low, _ := context.effectPoints(info, effect)
		return float64(absInt(low)), "", true
	case 'M':
		_, high := context.effectPoints(info, effect)
		return float64(absInt(high)), "", true
	case 'd', 'D':
		return 0, formatSpellDuration(info.DurationMS), true
	case 't', 'T':
		return float64(e.PeriodMS) / 1000, "", true
	case 'a', 'A':
		return e.Radius, "", true
	case 'x', 'X':
		return float64(e.Chain), "", true
	case 'h', 'H':
		return float64(info.ProcChance), "", true
	case 'n', 'N':
		return float64(info.ProcCharges), "", true
	case 'u', 'U':
		return float64(info.StackAmount), "", true
	case 'o', 'O':
		low, _ := context.effectPoints(info, effect)
		period := e.PeriodMS
		if period <= 0 {
			return float64(absInt(low)), "", true
		}
		return float64(absInt(low)) * float64(info.DurationMS) / float64(period), "", true
	case 'q', 'Q':
		return float64(e.Misc), "", true
	case 'e', 'E':
		return e.Amplitude, "", true
	}
	return 0, "", false
}

// spellStatNames are the player-stat variables an expression may use. The
// interface has no combat ratings to read them from, so they are zero except
// the player's level.
var spellStatNames = []string{"SPFR", "SPH", "SPS", "SPN", "SPF", "SPA", "SP", "RAP", "AP", "MWS", "MWB", "MW", "PL", "mwb", "mws"}

// expandVariableValue reads one $-variable at runes[from:] (after the '$') and
// returns its numeric value and the index after it.
func (context spellTextContext) expandVariableValue(runes []rune, from int, info SpellInfo) (float64, int, bool) {
	rest := string(runes[from:])
	for _, name := range spellStatNames {
		if strings.HasPrefix(rest, name) {
			value := 0.0
			if name == "PL" {
				value = float64(context.level)
			}
			return value, from + len([]rune(name)), true
		}
	}
	target := info
	j := from
	if j < len(runes) && runes[j] >= '0' && runes[j] <= '9' {
		k := j
		for k < len(runes) && runes[k] >= '0' && runes[k] <= '9' {
			k++
		}
		id, _ := strconv.Atoi(string(runes[j:k]))
		if other, ok := context.catalog[uint32(id)]; ok {
			target = other
		}
		j = k
	}
	if j >= len(runes) {
		return 0, from, false
	}
	variable := runes[j]
	effect := 0
	if j+1 < len(runes) && runes[j+1] >= '1' && runes[j+1] <= '3' {
		effect = int(runes[j+1] - '1')
		j++
	}
	value, text, ok := context.variable(target, variable, effect)
	if !ok || text != "" {
		return 0, from, false
	}
	return value, j + 1, true
}

// evaluateSpellExpression computes the arithmetic of a ${...} block: numbers,
// $-variables, + - * / and parentheses.
func (context spellTextContext) evaluateSpellExpression(expression string, info SpellInfo) float64 {
	runes := []rune(expression)
	position := 0
	skip := func() {
		for position < len(runes) && runes[position] == ' ' {
			position++
		}
	}
	var sum func() float64
	var factor func() float64
	factor = func() float64 {
		skip()
		if position >= len(runes) {
			return 0
		}
		switch c := runes[position]; {
		case c == '(':
			position++
			value := sum()
			skip()
			if position < len(runes) && runes[position] == ')' {
				position++
			}
			return value
		case c == '-':
			position++
			return -factor()
		case c == '$':
			value, next, ok := context.expandVariableValue(runes, position+1, info)
			if !ok {
				position++
				return 0
			}
			position = next
			return value
		case c >= '0' && c <= '9' || c == '.':
			start := position
			for position < len(runes) && (runes[position] >= '0' && runes[position] <= '9' || runes[position] == '.') {
				position++
			}
			value, _ := strconv.ParseFloat(string(runes[start:position]), 64)
			return value
		}
		position++
		return 0
	}
	product := func() float64 {
		value := factor()
		for {
			skip()
			if position >= len(runes) {
				return value
			}
			switch runes[position] {
			case '*':
				position++
				value *= factor()
			case '/':
				position++
				divisor := factor()
				if divisor != 0 {
					value /= divisor
				} else {
					value = 0
				}
			default:
				return value
			}
		}
	}
	sum = func() float64 {
		value := product()
		for {
			skip()
			if position >= len(runes) {
				return value
			}
			switch runes[position] {
			case '+':
				position++
				value += product()
			case '-':
				position++
				value -= product()
			default:
				return value
			}
		}
	}
	return sum()
}
