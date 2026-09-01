package ui

import (
	"os"
	"testing"
)

func TestLiveMovieClipDecodesMultipleFrames(t *testing.T) {
	dataPath := os.Getenv("WOW_TEST_DATA")
	if dataPath == "" {
		t.Skip("WOW_TEST_DATA not set; skipped")
	}
	rt := NewRuntime(nil)
	defer rt.Close()
	loader, err := NewMPQLoader(dataPath, "enUS", rt)
	if err != nil {
		t.Fatal(err)
	}
	defer loader.Close()
	data, err := loader.ReadFile(`Interface\Cinematics\WOW_Intro_LK_1024.avi`)
	if err != nil {
		t.Fatal(err)
	}
	clip, err := parseMovieClip(data)
	if err != nil {
		t.Fatal(err)
	}
	if len(clip.frames) < 2 || len(clip.keyframes) < 2 {
		t.Fatalf("movie frames=%d keyframes=%d", len(clip.frames), len(clip.keyframes))
	}
	limit := len(clip.keyframes)
	if limit > 12 {
		limit = 12
	}
	t.Logf("frames=%d keyframes=%v", len(clip.frames), clip.keyframes[:limit])
	playback := &moviePlayback{file: "test", clip: clip}
	if playback.image, err = playback.decode(0); err != nil {
		t.Fatal(err)
	}
	if _, err := playback.decode(clip.keyframes[1]); err != nil {
		t.Fatal(err)
	}
	if !playback.advance(float64(clip.keyframes[1]+1)/clip.fps) || playback.image == nil {
		t.Fatalf("movie did not advance at %.3f fps", clip.fps)
	}
}

func TestLiveMovieFrameRendersDecodedVideo(t *testing.T) {
	dataPath := os.Getenv("WOW_TEST_DATA")
	if dataPath == "" {
		t.Skip("WOW_TEST_DATA not set; skipped")
	}
	engine, err := LoadUIEngineFromMPQ(dataPath, "enUS", "")
	if err != nil {
		t.Fatal(err)
	}
	defer engine.Close()
	if engine.Rt.widgets["MovieFrame"] == nil {
		t.Fatal("MovieFrame missing from live Glue UI")
	}
	if !engine.Rt.Execute(`MovieFrame:Show(); MovieFrame:StartMovie("Interface\\Cinematics\\WOW_Intro_LK_1024", 0)`, "@movie-test.lua") {
		t.Fatalf("start movie failed: %v", engine.Rt.ScriptErrors())
	}
	engine.Update(0.1)
	if engine.movie == nil || engine.movieImage == nil {
		t.Fatal("MovieFrame started without a decoded video frame")
	}
	frame := engine.Render(640, 480)
	colored := 0
	for y := 0; y < frame.Bounds().Dy(); y += 16 {
		for x := 0; x < frame.Bounds().Dx(); x += 16 {
			_, _, _, alpha := frame.At(x, y).RGBA()
			if alpha != 0 {
				colored++
			}
		}
	}
	if colored == 0 {
		t.Fatal("decoded MovieFrame did not reach the rendered canvas")
	}
}
