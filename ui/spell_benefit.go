package ui

import lua "github.com/yuin/gopher-lua"

// What a spell does to whoever it lands on.
const (
	spellNeutral = 0
	spellHelpful = 1
	spellHarmful = 2
	// The bits of the spell word the classification reads.
	spellWordHelpful = 0x100
	spellWordHarmful = 0x80
	// auraNoneHelpful is the aura that does not make a friendly target helpful.
	auraNoneHelpful = 4
)

// Implicit targets that mean an enemy.
var spellHarmfulTargets = map[int]bool{2: true, 6: true, 0xf: true, 0x10: true, 0x18: true, 0x1c: true, 0x35: true, 0x36: true, 0x5d: true}

// Implicit targets that mean a friend.
var spellHelpfulTargets = map[int]bool{
	3: true, 4: true, 5: true, 0x14: true, 0x15: true, 0x1b: true, 0x1d: true, 0x1e: true, 0x1f: true, 0x21: true,
	0x22: true, 0x23: true, 0x2d: true, 0x38: true, 0x39: true, 0x3a: true, 0x3b: true, 0x3d: true, 0x3e: true,
}

// spellBenefit tells whether a spell helps (1), harms (2) or does neither (0).
func spellBenefit(info SpellInfo) int {
	if info.Word16&spellWordHelpful != 0 {
		return spellHelpful
	}
	if info.Word16&spellWordHarmful != 0 {
		return spellHarmful
	}
	for _, effect := range info.Effects {
		if spellHarmfulTargets[effect.TargetA] || spellHarmfulTargets[effect.TargetB] {
			return spellHarmful
		}
	}
	for _, effect := range info.Effects {
		for _, target := range [...]int{effect.TargetA, effect.TargetB} {
			if (target == 1 && effect.Aura != auraNoneHelpful) || spellHelpfulTargets[target] {
				return spellHelpful
			}
		}
	}
	return spellNeutral
}

func registerSpellBenefitAPI(rt *Runtime) {
	L := rt.L
	reg := func(name string, fn lua.LGFunction) { L.SetGlobal(name, L.NewFunction(fn)) }
	answer := func(L *lua.LState, ok bool, benefit, want int) int {
		if ok && benefit == want {
			L.Push(lua.LNumber(1))
		} else {
			L.Push(lua.LNil)
		}
		return 1
	}
	spell := func(want int) lua.LGFunction {
		return func(L *lua.LState) int {
			info, ok := rt.spellNamed(L, 1)
			return answer(L, ok, spellBenefit(info), want)
		}
	}
	item := func(want int) lua.LGFunction {
		return func(L *lua.LState) int {
			entry := rt.itemArgEntry(L, 1)
			template := rt.itemTemplate(entry)
			if entry == 0 || template == nil {
				return answer(L, false, 0, want)
			}
			for _, use := range template.Spells {
				if use.ID != 0 && use.Trigger == 0 {
					info, ok := rt.gameplay.Catalog[use.ID]
					return answer(L, ok, spellBenefit(info), want)
				}
			}
			return answer(L, false, 0, want)
		}
	}
	reg("IsHelpfulSpell", spell(spellHelpful))
	reg("IsHarmfulSpell", spell(spellHarmful))
	reg("IsHelpfulItem", item(spellHelpful))
	reg("IsHarmfulItem", item(spellHarmful))
}
