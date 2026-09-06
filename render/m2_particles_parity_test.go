package render

import (
	"os"
	"strings"
	"testing"

	"github.com/MorenoLand/Moreno.WoW/ui"
	"github.com/g3n/engine/material"
)

func TestM2ParticleBlendMode2IsAlpha(t *testing.T) {
	if got := m2ParticleBlending(2); got != material.BlendNormal {
		t.Fatalf("blend 2=%v want BlendNormal", got)
	}
}

func TestM2ParticleAppearanceHonorsAlphaTrack(t *testing.T) {
	emitter := m2ParticleEmitter{
		alpha: 1,
		scale: [2]float32{1, 1},
		color: [3]float32{255, 255, 255},
		alphaTrack: m2ParticleTrack{
			times:      []float32{0, 0.5, 1},
			values:     []float32{0, 0.3137, 0},
			components: 1,
		},
		colorTrack: m2ParticleTrack{
			times:      []float32{0, 1},
			values:     []float32{204, 236, 253, 143, 208, 253},
			components: 3,
		},
	}
	_, _, alpha, _ := particleAppearance(emitter, m2Particle{age: 0.5, life: 1})
	if alpha < 0.3 || alpha > 0.33 {
		t.Fatalf("mid-life snow alpha=%v want ~0.3137", alpha)
	}
	_, _, start, _ := particleAppearance(emitter, m2Particle{age: 0, life: 1})
	if start != 0 {
		t.Fatalf("start alpha=%v want 0", start)
	}
}

func TestLiveNorthrendSnowParticleParity(t *testing.T) {
	dataPath := os.Getenv("WOW_TEST_DATA")
	if dataPath == "" {
		t.Skip("WOW_TEST_DATA not set")
	}
	rt := ui.NewRuntime(nil)
	defer rt.Close()
	loader, err := ui.NewMPQLoader(dataPath, "enUS", rt)
	if err != nil {
		t.Fatal(err)
	}
	defer loader.Close()
	data, err := loader.ReadFile(`Interface\Glues\Models\UI_MainMenu_Northrend\UI_MainMenu_Northrend.m2`)
	if err != nil {
		t.Fatal(err)
	}
	model, err := parseM2(data)
	if err != nil {
		t.Fatal(err)
	}
	snow := 0
	additive := 0
	for _, emitter := range model.particles {
		path := ""
		if int(emitter.texture) < len(model.textures) {
			path = model.textures[emitter.texture]
		}
		if strings.Contains(strings.ToUpper(path), "SNOW") {
			snow++
			if emitter.blend != 2 {
				t.Fatalf("snow emitter blend=%d want alpha blend 2", emitter.blend)
			}
			_, _, start, _ := particleAppearance(emitter, m2Particle{age: 0, life: emitter.life})
			_, _, end, _ := particleAppearance(emitter, m2Particle{age: emitter.life, life: emitter.life})
			if start != 0 || end != 0 {
				t.Fatalf("snow alpha track should fade 0..peak..0, got start=%v end=%v", start, end)
			}
			if emitter.rate > 0 {
				_, _, mid, _ := particleAppearance(emitter, m2Particle{age: emitter.life * 0.5, life: emitter.life})
				if mid > 0.45 {
					t.Fatalf("active snow mid alpha=%v too opaque for authored track", mid)
				}
			}
		}
		if emitter.blend == 3 || emitter.blend == 4 {
			additive++
			if m2ParticleBlending(emitter.blend) != material.BlendAdditive {
				t.Fatalf("additive blend %d mapped to %v", emitter.blend, m2ParticleBlending(emitter.blend))
			}
		}
	}
	if snow == 0 {
		t.Fatal("expected SNOW1 particle emitters on login backdrop")
	}
	if additive == 0 {
		t.Fatal("expected additive particle emitters on login backdrop")
	}
	t.Logf("northrend snowEmitters=%d additiveEmitters=%d total=%d", snow, additive, len(model.particles))
}

func TestLiveNorthrendGlowTexturesUseRGBFalloff(t *testing.T) {
	dataPath := os.Getenv("WOW_TEST_DATA")
	if dataPath == "" {
		t.Skip("WOW_TEST_DATA not set")
	}
	rt := ui.NewRuntime(nil)
	defer rt.Close()
	loader, err := ui.NewMPQLoader(dataPath, "enUS", rt)
	if err != nil {
		t.Fatal(err)
	}
	defer loader.Close()
	for _, path := range []string{
		`SPELLS\T_VFX_FLAR_BLUR_64.BLP`,
		`WORLD\KALIMDOR\MAURADON\PASSIVEDOODADS\SATYRHANGINGBRAZIERS\FIRE1WHITE.BLP`,
	} {
		raw, err := loader.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		img, err := ui.DecodeBLP(raw)
		if err != nil {
			t.Fatal(err)
		}
		b := img.Bounds()
		var minA, maxA uint8 = 255, 0
		var minL, maxL float64 = 1, 0
		for y := b.Min.Y; y < b.Max.Y; y++ {
			for x := b.Min.X; x < b.Max.X; x++ {
				r, g, bv, a := img.At(x, y).RGBA()
				av := uint8(a >> 8)
				if av < minA {
					minA = av
				}
				if av > maxA {
					maxA = av
				}
				l := (float64(r) + float64(g) + float64(bv)) / (3 * 65535)
				if l < minL {
					minL = l
				}
				if l > maxL {
					maxL = l
				}
			}
		}
		if minA != 255 || maxA != 255 {
			t.Fatalf("%s unexpected alpha range %d..%d", path, minA, maxA)
		}
		if minL > 0.05 || maxL < 0.9 {
			t.Fatalf("%s RGB falloff minL=%v maxL=%v", path, minL, maxL)
		}
	}
}
