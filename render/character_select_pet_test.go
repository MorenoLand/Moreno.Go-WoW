package render

import (
	"testing"

	"github.com/MorenoLand/Moreno.WoW/network"
	"github.com/MorenoLand/Moreno.WoW/world"
)

func TestGlueStatePreservesCharacterSelectPetData(t *testing.T) {
	state := glueState(&network.Session{Characters: []world.Character{{Name: "Hunter", PetDisplayID: 2575, PetLevel: 21, PetFamily: 1}}}, nil)
	if len(state.Characters) != 1 {
		t.Fatalf("characters=%d", len(state.Characters))
	}
	character := state.Characters[0]
	if character.PetDisplayID != 2575 || character.PetLevel != 21 || character.PetFamily != 1 {
		t.Fatalf("pet state=%+v", character)
	}
}
