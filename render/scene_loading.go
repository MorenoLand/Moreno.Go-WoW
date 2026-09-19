package render

import (
	"log"
	"math"
	"time"

	"github.com/MorenoLand/Moreno.WoW/ui"
	"github.com/MorenoLand/Moreno.WoW/world"
)

func loadGlueSceneRequest(loader *ui.Loader, tables *worldCreatureTables, sceneModels []ui.GlueSceneModel, path, sceneKey, characterKey string, selected world.Character, facing float32, debug bool) sceneLoadResult {
	started := time.Now()
	result := sceneLoadResult{path: path, sceneKey: sceneKey, characterKey: characterKey, characterFacing: facing}
	var err error
	if len(sceneModels) > 0 {
		result.model, err = loadGlueScene(loader, sceneModels)
	} else {
		result.model, err = loadGlueModel(loader, path)
	}
	if err == nil && result.model != nil {
		if info, ok := result.model.UserData().(glueModelInfo); ok {
			result.fov = info.fov
		}
		if characterKey != "" {
			characterModel, characterErr := loadGlueCharacterModel(loader, selected)
			if characterErr != nil {
				if debug {
					log.Printf("character select model %s: %v", worldCharacterModelPath(selected), characterErr)
				}
			} else {
				if backgroundInfo, ok := result.model.UserData().(glueModelInfo); ok && backgroundInfo.hasStand {
					characterInfo, _ := characterModel.UserData().(glueModelInfo)
					characterScale, characterPosition := sceneCharacterTransform(backgroundInfo, characterInfo, characterModel.Position())
					characterModel.SetScale(characterScale, characterScale, characterScale)
					characterModel.SetPosition(characterPosition.X, characterPosition.Y, characterPosition.Z)
				}
				characterModel.SetRotation(0, facing*math.Pi/180, 0)
				result.characterModel = characterModel
				if selected.PetDisplayID != 0 && tables != nil {
					petDefinition, petErr := tables.definition(loader, selected.PetDisplayID, 0)
					if petErr != nil {
						if debug {
							log.Printf("character select pet display %d: %v", selected.PetDisplayID, petErr)
						}
					} else if petModel, petModelErr := buildWorldCreatureModel(loader, petDefinition); petModelErr != nil {
						if debug {
							log.Printf("character select pet model %d: %v", selected.PetDisplayID, petModelErr)
						}
					} else {
						petInfo, _ := petModel.UserData().(glueModelInfo)
						petScale := float32(1)
						if petDefinition.scale > 0 {
							petScale *= petDefinition.scale
						}
						if backgroundInfo, ok := result.model.UserData().(glueModelInfo); ok {
							petScale *= backgroundInfo.modelScale
							petFactor := petScale
							if petInfo.modelScale > 0 {
								petFactor /= petInfo.modelScale
							}
							petPosition := petModel.Position()
							petPosition.X += 0.8 / petFactor
							petModel.SetScale(petScale, petScale, petScale)
							petModel.SetPosition(backgroundInfo.standPosition.X+petPosition.X*petFactor, backgroundInfo.standPosition.Y+(petPosition.Y-petInfo.modelBottom)*petFactor, backgroundInfo.standPosition.Z+petPosition.Z*petFactor)
						}
						petModel.SetRotation(0, facing*math.Pi/180, 0)
						result.petModel = petModel
					}
				}
			}
		}
	}
	result.err = err
	result.loadMS = time.Since(started).Seconds() * 1000
	return result
}
