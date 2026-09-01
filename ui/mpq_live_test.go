package ui

import (
	"os"
	"testing"
)

func TestLiveGlueMusicAsset(t *testing.T) {
	dataPath := os.Getenv("WOW_TEST_DATA")
	if dataPath == "" {
		t.Skip("WOW_TEST_DATA not set")
	}
	set, err := openMPQSet(dataPath, "enUS")
	if err != nil {
		t.Fatal(err)
	}
	defer set.Close()
	name := normalizeMPQPath(`Sound\Music\GlueScreenMusic\WotLK_main_title.mp3`)
	for index := len(set.archives) - 1; index >= 0; index-- {
		archive := set.archives[index]
		if block, ok := archive.findBlock(name, set.locale); ok {
			t.Logf("music archive=%s block position=%d compressed=%d file=%d flags=%08x shift=%d", archive.path, block.position, block.compressedSize, block.fileSize, block.flags, archive.blockShift)
			break
		}
	}
	data, err := set.ReadFile(name)
	if err != nil {
		t.Fatal(err)
	}
	if len(data) == 0 {
		t.Fatal("music file is empty")
	}
}
