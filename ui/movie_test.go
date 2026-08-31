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
