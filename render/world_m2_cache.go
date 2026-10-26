package render

import (
	"strings"
	"sync"

	"github.com/MorenoLand/Moreno.WoW/ui"
	"github.com/g3n/engine/texture"
)

// A model's animation tracks are large and never change, yet every creature of
// a kind used to read and keep its own copy of them. The parsed model is read
// once and each user gets a copy of what its animation moves: the bones' pose.
var (
	sharedM2Mu    sync.Mutex
	sharedM2Cache = make(map[string]*parsedM2)
)

// loadSharedM2 reads a model with its animation tracks.
func loadSharedM2(loader *ui.Loader, path string) (parsedM2, error) {
	key := strings.ToLower(path)
	sharedM2Mu.Lock()
	template := sharedM2Cache[key]
	sharedM2Mu.Unlock()
	if template == nil {
		data, err := loader.ReadFile(path)
		if err != nil {
			return parsedM2{}, err
		}
		model, err := parseM2(data)
		if err != nil {
			return parsedM2{}, err
		}
		loadM2AnimationTracks(loader, path, &model)
		template = &model
		sharedM2Mu.Lock()
		if existing := sharedM2Cache[key]; existing != nil {
			template = existing
		} else {
			sharedM2Cache[key] = template
		}
		sharedM2Mu.Unlock()
	}
	model := *template
	model.bones = append([]m2Bone(nil), template.bones...)
	// The values animation writes into are each model's own, so units of one model can be posed side by side.
	model.colors = append(model.colors[:0:0], template.colors...)
	model.textureWeights = append(model.textureWeights[:0:0], template.textureWeights...)
	return model, nil
}

// sharedModelTextures are the textures of models read by path, which every
// model that names one draws with.
var (
	sharedTextureMu    sync.Mutex
	sharedTextureCache = make(map[string]*texture.Texture2D)
)

// loadSharedModelTexture is loadModelTexture with one decoded copy per path;
// the caller takes a reference of its own for each material it puts it in.
func loadSharedModelTexture(loader *ui.Loader, path string) *texture.Texture2D {
	key := strings.ToLower(path)
	sharedTextureMu.Lock()
	tex, ok := sharedTextureCache[key]
	sharedTextureMu.Unlock()
	if ok {
		return tex
	}
	tex = loadModelTexture(loader, path)
	sharedTextureMu.Lock()
	if existing, found := sharedTextureCache[key]; found {
		tex = existing
	} else {
		sharedTextureCache[key] = tex
	}
	sharedTextureMu.Unlock()
	return tex
}

// dropSharedTextureData lets go of the pixels of the shared textures the card
// has.
func dropSharedTextureData() {
	sharedTextureMu.Lock()
	defer sharedTextureMu.Unlock()
	for _, tex := range sharedTextureCache {
		dropTextureData(tex)
	}
}
