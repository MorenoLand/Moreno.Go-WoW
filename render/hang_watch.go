package render

import (
	"fmt"
	"os"
	"runtime/pprof"
	"sync/atomic"
	"time"
)

// hangLimit is how long the frame loop may stand still before the watch
// writes what every goroutine is doing.
const hangLimit = 4 * time.Second

// startHangWatch (--debug only) notes the last frame the loop drew and, when
// it stops for hangLimit, writes every goroutine's stack to
// hang_goroutines.txt so a freeze shows what it is stuck on. The returned
// function is called once a frame.
func startHangWatch() func() {
	var last atomic.Int64
	last.Store(time.Now().UnixNano())
	go func() {
		dumped := false
		for range time.Tick(time.Second) {
			stale := time.Since(time.Unix(0, last.Load()))
			if stale < hangLimit {
				dumped = false
				continue
			}
			if dumped {
				continue
			}
			dumped = true
			file, err := os.Create("hang_goroutines.txt")
			if err != nil {
				continue
			}
			fmt.Fprintf(file, "frame loop stopped %s ago\n\n", stale.Round(time.Millisecond))
			_ = pprof.Lookup("goroutine").WriteTo(file, 2)
			file.Close()
			fmt.Println("hang watch: wrote hang_goroutines.txt")
		}
	}()
	return func() { last.Store(time.Now().UnixNano()) }
}
