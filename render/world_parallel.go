package render

import (
	"math"
	"os"
	"runtime"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"github.com/MorenoLand/Moreno.WoW/ui"
)

// worldAnimJob is one unit's turn in a frame: its pose is worked out away from
// the others, then what depends on it runs in order.
type worldAnimJob struct {
	entity  *worldEntity
	info    glueModelInfo
	hasInfo bool
	step    float64
	sounds  []uint32
}

// worldWorkers is how many goroutines share out poses; the main one is busy
// drawing the rest of the time, so one core is left to it.
var worldWorkers = func() int {
	workers := runtime.NumCPU() - 1
	if workers > worldWorkerCap {
		workers = worldWorkerCap
	}
	if workers < 1 {
		workers = 1
	}
	return workers
}()

// runParallel calls work(0..count-1), spread over the workers, and returns when
// all are done. A few calls, or one worker, run in place.
func runParallel(count int, work func(index int)) {
	if count == 0 {
		return
	}
	workers := worldWorkers
	if workers <= 1 || count < 16 {
		for index := 0; index < count; index++ {
			work(index)
		}
		return
	}
	if workers > count {
		workers = count
	}
	var next int64
	var wait sync.WaitGroup
	wait.Add(workers)
	for worker := 0; worker < workers; worker++ {
		go func() {
			defer wait.Done()
			for {
				index := int(atomic.AddInt64(&next, 1)) - 1
				if index >= count {
					return
				}
				work(index)
			}
		}()
	}
	wait.Wait()
}

// worldWorkerCap bounds the workers: more than a few leave the drawing thread
// and the graphics driver short of cores.
var worldWorkerCap = func() int {
	if value, err := strconv.Atoi(os.Getenv("WOW_WORKERS")); err == nil && value > 0 {
		return value
	}
	return 4
}()

// How far the world is drawn follows the frame, as the "view distance" setting
// of the original does by hand: a scene that takes longer to draw than the
// target is drawn a little less far (and fogged to match), and drawn farther
// again as it allows. Beyond the fog nothing shows anyway, so that is never
// drawn at all.
const (
	worldViewMinFar    = 150
	worldViewMinFactor = 0.2
)

// worldViewLimiter holds how much of the view distance is used.
type worldViewLimiter struct {
	factor  float64
	average float64
	frames  int
}

// next gives the factor (0..1] for the coming frame, from how long the last
// scene draw took and what it may take (0 turns the limit off).
func (l *worldViewLimiter) next(sceneMS, targetMS float64) float64 {
	if l.factor == 0 {
		l.factor = 1
	}
	if targetMS <= 0 {
		l.factor = 1
		return 1
	}
	if l.average == 0 {
		l.average = sceneMS
	}
	l.average += (sceneMS - l.average) * 0.2
	l.frames++
	if l.frames%2 != 0 {
		return l.factor
	}
	switch {
	case l.average > targetMS*1.1:
		// What is drawn grows with the area seen, so the distance shrinks by the
		// root of how far over the time is, at most a tenth at a time.
		step := math.Sqrt(targetMS / l.average)
		if step < 0.9 {
			step = 0.9
		}
		if step > 0.99 {
			step = 0.99
		}
		l.factor *= step
	case l.average < targetMS*0.75 && l.frames%4 == 0:
		l.factor *= 1.02
	}
	if l.factor < worldViewMinFactor {
		l.factor = worldViewMinFactor
	}
	if l.factor > 1 {
		l.factor = 1
	}
	return l.factor
}

// worldSceneTarget is what drawing the scene may take, from the target frame
// rate (cvar targetFPS, 100 unless set; 0 keeps the whole view).
func worldSceneTarget(rt *ui.Runtime) float64 {
	fps := 100.0
	if rt != nil {
		if value, ok := rt.GetCVar("targetFPS"); ok {
			if parsed, err := strconv.ParseFloat(value, 64); err == nil {
				fps = parsed
			}
		}
	}
	if fps <= 0 {
		return 0
	}
	target := 1000/fps - 3.5
	if target < 2.5 {
		target = 2.5
	}
	return target
}

// worldViewFar is the far plane for a frame: the configured distance scaled by
// factor, and no farther than the fog lets anything be seen. It also pulls the
// fog in to the plane when the plane is inside it.
func worldViewFar(base float32, factor float64, atmo *worldAtmosphere) float32 {
	far := base * float32(factor)
	if far < worldViewMinFar {
		far = worldViewMinFar
	}
	if atmo == nil || !atmo.valid || atmo.fogEnd <= 0 {
		return far
	}
	if limit := atmo.fogEnd*1.05 + 10; limit < far {
		far = limit
	}
	if end := far / 1.05; end < atmo.fogEnd {
		atmo.fogStart *= end / atmo.fogEnd
		atmo.fogEnd = end
	}
	return far
}

const (
	// worldMinSizeRatio is the height over distance below which a far unit or
	// object is left out (about six pixels at 1400 lines).
	worldMinSizeRatio = 0.012
	// worldShadowRange is how far a unit's blob shadow is drawn.
	worldShadowRange = 45
)

// worldEntityDrawn says whether a unit or object is drawn: it is within the
// radius, and not so far that it would cover only a few pixels.
func worldEntityDrawn(entity *worldEntity, camera [3]float32, radius float32) bool {
	dx := entity.movement.Position.X - camera[0]
	dy := entity.movement.Position.Y - camera[1]
	distanceSquared := dx*dx + dy*dy
	if distanceSquared > radius*radius {
		return false
	}
	if distanceSquared <= 40*40 {
		return true
	}
	height := float32(1.5)
	if info, ok := entity.node.UserData().(glueModelInfo); ok && info.modelTop > 0 {
		height = info.modelTop * entity.node.Scale().Y
	}
	return height*height >= worldMinSizeRatio*worldMinSizeRatio*distanceSquared
}

// worldUIRepaintInterval is the shortest time between two repaints of the
// interface while in the world.
const worldUIRepaintInterval = 33 * time.Millisecond
