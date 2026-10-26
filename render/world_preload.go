package render

import (
	"math"
	"sort"
	"sync"

	"github.com/MorenoLand/Moreno.WoW/ui"
	"github.com/MorenoLand/Moreno.WoW/world"
)

// While the player looks at the character list the map around the selected
// character is read and built in the background, so entering the world finds the
// tiles, models and textures ready instead of building them behind a loading
// screen and then with the world already on screen. Nothing here touches the
// card: tiles are built into nodes that are not in any scene yet.

type worldPreloadState struct {
	mu         sync.Mutex
	key        [3]int
	active     bool
	generation uint64
	cache      *worldSceneAssetCache
	ready      map[[2]int]*worldStreamedTile
	done       chan struct{}
}

var worldPreload worldPreloadState

// worldPreloaded is what a preload hands over to a world that is being entered.
type worldPreloaded struct {
	cache *worldSceneAssetCache
	tiles map[[2]int]*worldStreamedTile
}

// startWorldPreload begins building the tiles around a character's last
// position, unless that is what is already being built.
func startWorldPreload(loader *ui.Loader, character world.Character) {
	if loader == nil || character.GUID == 0 {
		return
	}
	tileX, tileY := worldTileAt(character.Position[0], character.Position[1])
	if tileX < 0 || tileX >= 64 || tileY < 0 || tileY >= 64 {
		return
	}
	key := [3]int{int(character.Map), tileX, tileY}
	worldPreload.mu.Lock()
	if worldPreload.active && worldPreload.key == key {
		worldPreload.mu.Unlock()
		return
	}
	previous := worldPreload.done
	worldPreload.generation++
	generation := worldPreload.generation
	worldPreload.key, worldPreload.active = key, true
	worldPreload.cache = nil
	worldPreload.ready = make(map[[2]int]*worldStreamedTile)
	done := make(chan struct{})
	worldPreload.done = done
	worldPreload.mu.Unlock()
	go func() {
		defer close(done)
		if previous != nil {
			<-previous
		}
		mapName := loadMapName(loader, character.Map)
		if mapName == "" {
			return
		}
		position := world.WorldPosition{Map: character.Map, X: character.Position[0], Y: character.Position[1], Z: character.Position[2]}
		cache := newWorldSceneAssetCache()
		cache.setFocus(position.X, position.Y)
		streamer := newWorldTerrainStreamer(loader, mapName, position, worldMapBigAlpha(loader, mapName), cache, nil)
		type wanted struct {
			key      [2]int
			distance float64
		}
		var tiles []wanted
		for dy := -worldStreamRadius; dy <= worldStreamRadius; dy++ {
			for dx := -worldStreamRadius; dx <= worldStreamRadius; dx++ {
				at := [2]int{tileX + dx, tileY + dy}
				if at[0] < 0 || at[0] >= 64 || at[1] < 0 || at[1] >= 64 {
					continue
				}
				centreX := (32 - float64(at[1]) - 0.5) * worldTileSize
				centreY := (32 - float64(at[0]) - 0.5) * worldTileSize
				tiles = append(tiles, wanted{at, math.Hypot(centreX-float64(position.X), centreY-float64(position.Y))})
			}
		}
		sort.Slice(tiles, func(i, j int) bool { return tiles[i].distance < tiles[j].distance })
		worldPreload.mu.Lock()
		if worldPreload.generation == generation {
			worldPreload.cache = cache
		}
		worldPreload.mu.Unlock()
		for _, entry := range tiles {
			worldPreload.mu.Lock()
			current := worldPreload.generation == generation
			worldPreload.mu.Unlock()
			if !current {
				return
			}
			tile, err := streamer.loadTile(entry.key[0], entry.key[1])
			if err != nil {
				continue
			}
			worldPreload.mu.Lock()
			if worldPreload.generation == generation {
				worldPreload.ready[entry.key] = tile
			}
			worldPreload.mu.Unlock()
		}
	}()
}

// takeWorldPreload hands over what was built for the map and tile being
// entered, after the tile being built at that moment is done, or nothing when
// the preload was for somewhere else.
func takeWorldPreload(mapID uint32, tileX, tileY int) *worldPreloaded {
	worldPreload.mu.Lock()
	if !worldPreload.active {
		worldPreload.mu.Unlock()
		return nil
	}
	wanted := worldPreload.key == [3]int{int(mapID), tileX, tileY}
	worldPreload.generation++
	done := worldPreload.done
	worldPreload.mu.Unlock()
	if done != nil {
		<-done
	}
	worldPreload.mu.Lock()
	defer worldPreload.mu.Unlock()
	taken := &worldPreloaded{cache: worldPreload.cache, tiles: worldPreload.ready}
	worldPreload.active = false
	worldPreload.cache, worldPreload.ready, worldPreload.done = nil, nil, nil
	if !wanted || taken.cache == nil {
		return nil
	}
	return taken
}

// dropWorldPreload lets go of a preload nobody entered.
func dropWorldPreload() {
	worldPreload.mu.Lock()
	worldPreload.generation++
	worldPreload.active = false
	worldPreload.cache, worldPreload.ready = nil, nil
	worldPreload.mu.Unlock()
}
