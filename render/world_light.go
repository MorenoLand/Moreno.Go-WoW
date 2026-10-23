package render

import (
	"encoding/binary"
	"math"
	"sort"
	"time"

	"github.com/MorenoLand/Moreno.WoW/ui"
	"github.com/MorenoLand/Moreno.WoW/world"
)

// Light.dbc places lights in a coordinate system that is the world's with both
// axes flipped about the map centre and scaled by 36: x = (17066.666-worldY)*36
// and z = (17066.666-worldX)*36. Fog distances are in the same scaled units.
const (
	worldLightMapCentre  = 17066.666
	worldLightUnitScale  = 36.0
	worldLightIntBands   = 18
	worldLightFloatBands = 6
	worldLightDayHalfMin = 2880.0
	worldLightMaxLights  = 3
)

// LightIntBand channels, verified against the client's own light sampler
// (Wow.exe FUN_007ebff0 reads row (params-1)*18+channel+1 for each).
const (
	worldLightChannelDiffuse  = 0
	worldLightChannelAmbient  = 1
	worldLightChannelSkyTop   = 2
	worldLightChannelSkyMid   = 3
	worldLightChannelSkyBand1 = 4
	worldLightChannelSkyBand2 = 5
	worldLightChannelSkySmog  = 6
	worldLightChannelFog      = 7
	worldLightChannelOceanS   = 14
	worldLightChannelOceanD   = 15
	worldLightChannelRiverS   = 16
	worldLightChannelRiverD   = 17
)

// LightFloatBand channels: fog end distance and the scalar that places fog start.
const (
	worldLightFloatFogEnd   = 0
	worldLightFloatFogStart = 1
)

type worldLightRow struct {
	mapID     uint32
	x, y, z   float32
	fallStart float32
	fallEnd   float32
	clear     uint32
	storm     uint32
}

type worldLightBand struct {
	count  int
	times  [16]uint32
	values [16]uint32
}

type worldLightParams struct {
	riverShallowAlpha float32
	riverDeepAlpha    float32
	oceanShallowAlpha float32
	oceanDeepAlpha    float32
}

type worldLightTables struct {
	lights     []worldLightRow
	params     map[uint32]worldLightParams
	intBands   map[uint32]worldLightBand
	floatBands map[uint32]worldLightBand
}

// worldAtmosphere is the lighting state at the camera: what the sky, fog,
// water and sun-lit surfaces use this frame.
type worldAtmosphere struct {
	valid     bool
	ambient   [3]float32
	diffuse   [3]float32
	direction [3]float32 // towards the light, world space
	sky       [5][3]float32
	fog       [3]float32
	fogStart  float32
	fogEnd    float32
	// water colours by [river, ocean] and shallow/deep.
	waterShallow [2][3]float32
	waterDeep    [2][3]float32
	waterAlpha   [2][2]float32
}

// worldAtmo is the lighting the world materials read when they draw. It is
// written once a frame by the render loop and read during drawing.
var worldAtmo worldAtmosphere

func readWorldDBCRows(loader *ui.Loader, path string, fields int, each func(row []uint32)) bool {
	data, err := loader.ReadFile(path)
	if err != nil || len(data) < 20 || string(data[:4]) != "WDBC" {
		return false
	}
	records := int(binary.LittleEndian.Uint32(data[4:8]))
	columns := int(binary.LittleEndian.Uint32(data[8:12]))
	stride := int(binary.LittleEndian.Uint32(data[12:16]))
	if columns < fields || stride < fields*4 || 20+records*stride > len(data) {
		return false
	}
	row := make([]uint32, fields)
	for record := 0; record < records; record++ {
		base := 20 + record*stride
		for column := range row {
			row[column] = binary.LittleEndian.Uint32(data[base+column*4:])
		}
		each(row)
	}
	return true
}

func readWorldLightTables(loader *ui.Loader) *worldLightTables {
	tables := &worldLightTables{params: make(map[uint32]worldLightParams), intBands: make(map[uint32]worldLightBand), floatBands: make(map[uint32]worldLightBand)}
	f := math.Float32frombits
	if !readWorldDBCRows(loader, `DBFilesClient\Light.dbc`, 15, func(row []uint32) {
		tables.lights = append(tables.lights, worldLightRow{
			mapID: row[1], x: f(row[2]), y: f(row[3]), z: f(row[4]), fallStart: f(row[5]), fallEnd: f(row[6]),
			clear: row[7], storm: row[9],
		})
	}) {
		return nil
	}
	readWorldDBCRows(loader, `DBFilesClient\LightParams.dbc`, 9, func(row []uint32) {
		tables.params[row[0]] = worldLightParams{riverShallowAlpha: f(row[5]), riverDeepAlpha: f(row[6]), oceanShallowAlpha: f(row[7]), oceanDeepAlpha: f(row[8])}
	})
	readBands := func(path string, into map[uint32]worldLightBand) {
		readWorldDBCRows(loader, path, 34, func(row []uint32) {
			band := worldLightBand{count: int(row[1])}
			if band.count > 16 {
				band.count = 16
			}
			copy(band.times[:], row[2:18])
			copy(band.values[:], row[18:34])
			into[row[0]] = band
		})
	}
	readBands(`DBFilesClient\LightIntBand.dbc`, tables.intBands)
	readBands(`DBFilesClient\LightFloatBand.dbc`, tables.floatBands)
	return tables
}

// bandAt interpolates a band at a time in half-minutes. The day is a circle:
// before the first key the curve runs from the last key of the previous day.
func (b worldLightBand) at(time float32, blend func(a, b uint32, t float32) uint32) (uint32, bool) {
	if b.count == 0 {
		return 0, false
	}
	if b.count == 1 {
		return b.values[0], true
	}
	next := 0
	for next < b.count && float32(b.times[next]) < time {
		next++
	}
	previous := next - 1
	var previousTime, nextTime float32
	if next == b.count {
		next = 0
		previous = b.count - 1
		previousTime, nextTime = float32(b.times[previous]), float32(b.times[next])+worldLightDayHalfMin
	} else if previous < 0 {
		previous = b.count - 1
		previousTime, nextTime = float32(b.times[previous])-worldLightDayHalfMin, float32(b.times[next])
	} else {
		previousTime, nextTime = float32(b.times[previous]), float32(b.times[next])
	}
	span := nextTime - previousTime
	if span <= 0 {
		return b.values[next], true
	}
	return blend(b.values[previous], b.values[next], (time-previousTime)/span), true
}

func blendWorldLightColor(a, b uint32, t float32) uint32 {
	var out uint32
	for shift := uint(0); shift <= 16; shift += 8 {
		x, y := float32((a>>shift)&0xff), float32((b>>shift)&0xff)
		out |= uint32(math.Round(float64(x+(y-x)*t))) << shift
	}
	return out
}

func blendWorldLightFloat(a, b uint32, t float32) uint32 {
	x, y := math.Float32frombits(a), math.Float32frombits(b)
	return math.Float32bits(x + (y-x)*t)
}

func unpackWorldLightColor(packed uint32) [3]float32 {
	return [3]float32{float32((packed>>16)&0xff) / 255, float32((packed>>8)&0xff) / 255, float32(packed&0xff) / 255}
}

func (t *worldLightTables) colour(paramsID uint32, channel int, time float32) ([3]float32, bool) {
	band, ok := t.intBands[(paramsID-1)*worldLightIntBands+1+uint32(channel)]
	if !ok {
		return [3]float32{}, false
	}
	value, ok := band.at(time, blendWorldLightColor)
	return unpackWorldLightColor(value), ok
}

func (t *worldLightTables) scalar(paramsID uint32, channel int, time float32) (float32, bool) {
	band, ok := t.floatBands[(paramsID-1)*worldLightFloatBands+1+uint32(channel)]
	if !ok {
		return 0, false
	}
	value, ok := band.at(time, blendWorldLightFloat)
	return math.Float32frombits(value), ok
}

// worldLightWeight is one light's share of the blend at a position.
type worldLightWeight struct {
	paramsID uint32
	weight   float32
}

// weightsAt picks up to three positioned lights covering a position, nearest
// first, each weighted 1 inside its inner radius and falling to 0 at the outer
// radius, and gives whatever weight remains to the map's default light (the
// one at the origin).
func (t *worldLightTables) weightsAt(mapID uint32, x, y float32) []worldLightWeight {
	lx := (worldLightMapCentre - y) * worldLightUnitScale
	lz := (worldLightMapCentre - x) * worldLightUnitScale
	type candidate struct {
		distance float32
		weight   float32
		params   uint32
	}
	var candidates []candidate
	var defaultParams uint32
	for _, light := range t.lights {
		if light.mapID != mapID {
			continue
		}
		if light.x == 0 && light.y == 0 && light.z == 0 {
			defaultParams = light.clear
			continue
		}
		dx, dz := float64(light.x-lx), float64(light.z-lz)
		distance := float32(math.Sqrt(dx*dx + dz*dz))
		if distance >= light.fallEnd {
			continue
		}
		weight := float32(1)
		if distance > light.fallStart && light.fallEnd > light.fallStart {
			weight = (light.fallEnd - distance) / (light.fallEnd - light.fallStart)
		}
		candidates = append(candidates, candidate{distance: distance, weight: weight, params: light.clear})
	}
	sort.Slice(candidates, func(i, j int) bool { return candidates[i].distance < candidates[j].distance })
	remaining := float32(1)
	var weights []worldLightWeight
	for _, c := range candidates {
		if len(weights) == worldLightMaxLights || remaining <= 0 {
			break
		}
		w := c.weight
		if w > remaining {
			w = remaining
		}
		weights = append(weights, worldLightWeight{paramsID: c.params, weight: w})
		remaining -= w
	}
	if remaining > 0 && defaultParams != 0 {
		weights = append(weights, worldLightWeight{paramsID: defaultParams, weight: remaining})
	}
	return weights
}

// sunDirection is the direction towards the light for a time of day (minutes
// since midnight): the sun climbs from the east at 06:00 to its highest at
// noon and sets in the west at 18:00; at night the same arc is the moon's.
func worldSunDirection(minuteOfDay float32) [3]float32 {
	angle := float64(minuteOfDay/60-6) / 12 * math.Pi
	if angle < 0 || angle > math.Pi {
		angle = math.Mod(angle+2*math.Pi, math.Pi)
	}
	elevation := math.Sin(angle)
	if elevation < 0.15 {
		elevation = 0.15
	}
	// World x runs north and y runs west: the east horizon is -y.
	x, y, z := -0.35*elevation, -math.Cos(angle), elevation
	length := math.Sqrt(x*x + y*y + z*z)
	return [3]float32{float32(x / length), float32(y / length), float32(z / length)}
}

// sample computes the atmosphere at a position and minute of the day.
func (t *worldLightTables) sample(mapID uint32, x, y float32, minuteOfDay float32) worldAtmosphere {
	var out worldAtmosphere
	if t == nil {
		return out
	}
	weights := t.weightsAt(mapID, x, y)
	if len(weights) == 0 {
		return out
	}
	time := minuteOfDay * 2
	var total float32
	add := func(dst *[3]float32, channel int, paramsID uint32, w float32) {
		if c, ok := t.colour(paramsID, channel, time); ok {
			for i := range c {
				dst[i] += c[i] * w
			}
		}
	}
	for _, entry := range weights {
		w := entry.weight
		total += w
		add(&out.diffuse, worldLightChannelDiffuse, entry.paramsID, w)
		add(&out.ambient, worldLightChannelAmbient, entry.paramsID, w)
		for i := range out.sky {
			add(&out.sky[i], worldLightChannelSkyTop+i, entry.paramsID, w)
		}
		add(&out.fog, worldLightChannelFog, entry.paramsID, w)
		add(&out.waterShallow[1], worldLightChannelOceanS, entry.paramsID, w)
		add(&out.waterDeep[1], worldLightChannelOceanD, entry.paramsID, w)
		add(&out.waterShallow[0], worldLightChannelRiverS, entry.paramsID, w)
		add(&out.waterDeep[0], worldLightChannelRiverD, entry.paramsID, w)
		if end, ok := t.scalar(entry.paramsID, worldLightFloatFogEnd, time); ok {
			out.fogEnd += end / worldLightUnitScale * w
		}
		if scalar, ok := t.scalar(entry.paramsID, worldLightFloatFogStart, time); ok {
			if scalar < 0 {
				scalar = 0
			}
			if scalar > 1 {
				scalar = 1
			}
			out.fogStart += scalar * w
		}
		params := t.params[entry.paramsID]
		out.waterAlpha[0][0] += params.riverShallowAlpha * w
		out.waterAlpha[0][1] += params.riverDeepAlpha * w
		out.waterAlpha[1][0] += params.oceanShallowAlpha * w
		out.waterAlpha[1][1] += params.oceanDeepAlpha * w
	}
	if total <= 0 {
		return worldAtmosphere{}
	}
	scale := 1 / total
	scaleColour := func(c *[3]float32) {
		for i := range c {
			c[i] *= scale
		}
	}
	scaleColour(&out.diffuse)
	scaleColour(&out.ambient)
	for i := range out.sky {
		scaleColour(&out.sky[i])
	}
	scaleColour(&out.fog)
	for i := 0; i < 2; i++ {
		scaleColour(&out.waterShallow[i])
		scaleColour(&out.waterDeep[i])
		out.waterAlpha[i][0] *= scale
		out.waterAlpha[i][1] *= scale
	}
	// The scalar stored is the fraction of the end distance at which fog starts.
	out.fogEnd *= scale
	out.fogStart = out.fogEnd * out.fogStart * scale
	out.direction = worldSunDirection(minuteOfDay)
	out.valid = true
	return out
}

// worldGameClock is the server's game clock: its reading when the server
// last stated it and the speed it has run at since. Before the server states
// it, the local wall clock stands in.
type worldGameClock struct {
	stated  bool
	minute  float32
	speed   float32
	started time.Time
}

func (c *worldGameClock) set(t world.GameTime, now time.Time) {
	c.stated, c.minute, c.speed, c.started = true, float32(t.MinuteOfDay), t.Speed, now
}

// minuteOfDay is the game clock in minutes since midnight.
func (c *worldGameClock) minuteOfDay(now time.Time) float32 {
	if !c.stated {
		return float32(now.Hour()*60 + now.Minute())
	}
	minute := float64(c.minute) + now.Sub(c.started).Seconds()*float64(c.speed)
	minute = math.Mod(minute, 1440)
	if minute < 0 {
		minute += 1440
	}
	return float32(minute)
}
