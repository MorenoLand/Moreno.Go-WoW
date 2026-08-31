package render

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
	"log"
	"math"
	"strings"
	"sync"

	"github.com/MorenoLand/Moreno.WoW/ui"
	"github.com/ebitengine/oto/v3"
)

const audioSampleRate = 44100

type soundEntry struct {
	directory string
	files     []string
}

type audioManager struct {
	loader          *ui.Loader
	context         *oto.Context
	catalog         map[string]soundEntry
	pcm             map[string][]byte
	failed          map[string]struct{}
	music           *oto.Player
	ambience        *oto.Player
	sfx             []*oto.Player
	allEnabled      bool
	musicEnabled    bool
	sfxEnabled      bool
	ambienceEnabled bool
	musicVolume     float64
	sfxVolume       float64
	ambienceVolume  float64
	debug           bool
	mu              sync.Mutex
}

func newAudioManager(loader *ui.Loader, debug bool) (*audioManager, error) {
	manager := &audioManager{loader: loader, catalog: make(map[string]soundEntry), pcm: make(map[string][]byte), failed: make(map[string]struct{}), allEnabled: true, musicEnabled: true, sfxEnabled: true, ambienceEnabled: true, musicVolume: 1, sfxVolume: 1, ambienceVolume: 1, debug: debug}
	if data, err := loader.ReadFile(`DBFilesClient\SoundEntries.dbc`); err == nil {
		manager.catalog = parseSoundEntries(data)
	}
	context, ready, err := oto.NewContext(&oto.NewContextOptions{SampleRate: audioSampleRate, ChannelCount: 2, Format: oto.FormatSignedInt16LE})
	if err != nil {
		return nil, err
	}
	<-ready
	manager.context = context
	return manager, nil
}

func (m *audioManager) Close() {
	if m == nil {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.music != nil {
		m.music.Pause()
	}
	if m.ambience != nil {
		m.ambience.Pause()
	}
	for _, player := range m.sfx {
		player.Pause()
	}
	m.music = nil
	m.ambience = nil
	m.sfx = nil
}

func (m *audioManager) SetAudioCVar(name, value string) {
	if m == nil {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	switch strings.ToLower(name) {
	case "sound_enableallsound":
		m.allEnabled = value == "1" || strings.EqualFold(value, "true")
	case "sound_enablemusic":
		m.musicEnabled = value == "1" || strings.EqualFold(value, "true")
	case "sound_enablesfx":
		m.sfxEnabled = value == "1" || strings.EqualFold(value, "true")
	case "sound_enableambience":
		m.ambienceEnabled = value == "1" || strings.EqualFold(value, "true")
	case "sound_musicvolume":
		m.musicVolume = parseVolume(value)
	case "sound_sfxvolume":
		m.sfxVolume = parseVolume(value)
	case "sound_ambiencevolume":
		m.ambienceVolume = parseVolume(value)
	}
	m.applyVolumesLocked()
}

func (m *audioManager) PlaySound(name string) {
	if m == nil {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if !m.allEnabled || !m.sfxEnabled || m.context == nil {
		return
	}
	m.pruneSFXLocked()
	data, key, err := m.pcmLocked(name)
	if err != nil {
		m.reportFailure(name, err)
		return
	}
	player := m.context.NewPlayer(bytes.NewReader(data))
	player.SetVolume(m.sfxVolume)
	player.Play()
	m.sfx = append(m.sfx, player)
	_ = key
}

func (m *audioManager) PlayMusic(name string) {
	if m == nil {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if !m.allEnabled || !m.musicEnabled || m.context == nil {
		return
	}
	data, _, err := m.pcmLocked(name)
	if err != nil {
		m.reportFailure(name, err)
		return
	}
	if m.music != nil {
		m.music.Pause()
	}
	m.music = m.context.NewPlayer(newLoopReader(data))
	m.music.SetVolume(m.musicVolume)
	m.music.Play()
}

func (m *audioManager) PlayAmbience(name string) {
	if m == nil {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if !m.allEnabled || !m.ambienceEnabled || m.context == nil {
		return
	}
	data, _, err := m.pcmLocked(name)
	if err != nil {
		m.reportFailure(name, err)
		return
	}
	if m.ambience != nil {
		m.ambience.Pause()
	}
	m.ambience = m.context.NewPlayer(newLoopReader(data))
	m.ambience.SetVolume(m.ambienceVolume)
	m.ambience.Play()
}

func (m *audioManager) StopMusic() {
	if m == nil {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.music != nil {
		m.music.Pause()
		m.music = nil
	}
}

func (m *audioManager) StopAmbience() {
	if m == nil {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.ambience != nil {
		m.ambience.Pause()
		m.ambience = nil
	}
}

func (m *audioManager) StopAllSFX() {
	if m == nil {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, player := range m.sfx {
		player.Pause()
	}
	m.sfx = nil
}

func (m *audioManager) applyVolumesLocked() {
	if m.music != nil {
		m.music.SetVolume(m.effectiveVolume(m.musicEnabled, m.musicVolume))
	}
	if m.ambience != nil {
		m.ambience.SetVolume(m.effectiveVolume(m.ambienceEnabled, m.ambienceVolume))
	}
	for _, player := range m.sfx {
		player.SetVolume(m.effectiveVolume(m.sfxEnabled, m.sfxVolume))
	}
}

func (m *audioManager) effectiveVolume(enabled bool, volume float64) float64 {
	if !m.allEnabled || !enabled {
		return 0
	}
	return volume
}

func (m *audioManager) pruneSFXLocked() {
	kept := m.sfx[:0]
	for _, player := range m.sfx {
		if player.IsPlaying() {
			kept = append(kept, player)
		}
	}
	m.sfx = kept
}

func (m *audioManager) reportFailure(name string, err error) {
	if m.debug {
		key := strings.ToLower(strings.TrimSpace(name))
		if _, reported := m.failed[key]; !reported {
			m.failed[key] = struct{}{}
			log.Printf("audio %s: %v", name, err)
		}
	}
}

func (m *audioManager) pcmLocked(name string) ([]byte, string, error) {
	candidates := m.soundCandidates(name)
	if len(candidates) == 0 {
		return nil, "", fmt.Errorf("sound kit %q is not in SoundEntries.dbc", name)
	}
	var lastErr error
	for _, candidate := range candidates {
		key := strings.ToLower(strings.ReplaceAll(candidate, "/", "\\"))
		if data, ok := m.pcm[key]; ok {
			return data, key, nil
		}
		raw, err := m.loader.ReadFile(candidate)
		if err != nil {
			lastErr = err
			for _, extension := range []string{".wav", ".ogg", ".mp3"} {
				raw, err = m.loader.ReadFile(candidate + extension)
				if err == nil {
					break
				}
			}
		}
		if err != nil {
			lastErr = err
			continue
		}
		pcm, err := decodeWAVToPCM(raw)
		if err != nil {
			lastErr = err
			continue
		}
		m.pcm[key] = pcm
		return pcm, key, nil
	}
	if lastErr == nil {
		lastErr = osErrNotExist{}
	}
	return nil, "", lastErr
}

func (m *audioManager) soundCandidates(name string) []string {
	name = strings.TrimSpace(name)
	if name == "" {
		return nil
	}
	if entry, ok := m.catalog[strings.ToLower(name)]; ok {
		candidates := make([]string, 0, len(entry.files))
		for _, file := range entry.files {
			if entry.directory != "" && !strings.ContainsAny(file, `\\/`) {
				candidates = append(candidates, entry.directory+"\\"+file)
			} else {
				candidates = append(candidates, file)
			}
		}
		return candidates
	}
	return []string{name}
}

func parseSoundEntries(data []byte) map[string]soundEntry {
	result := make(map[string]soundEntry)
	if len(data) < 20 || string(data[:4]) != "WDBC" {
		return result
	}
	records := int(binary.LittleEndian.Uint32(data[4:8]))
	fields := int(binary.LittleEndian.Uint32(data[8:12]))
	stride := int(binary.LittleEndian.Uint32(data[12:16]))
	stringSize := int(binary.LittleEndian.Uint32(data[16:20]))
	if fields < 24 || stride < fields*4 || records < 0 || 20+records*stride < 20 || 20+records*stride+stringSize > len(data) {
		return result
	}
	stringStart := 20 + records*stride
	readString := func(offset uint32) string {
		start := int(offset)
		if start < 0 || start >= stringSize {
			return ""
		}
		end := bytes.IndexByte(data[stringStart+start:stringStart+stringSize], 0)
		if end < 0 {
			return ""
		}
		return string(data[stringStart+start : stringStart+start+end])
	}
	for record := 0; record < records; record++ {
		base := 20 + record*stride
		name := readString(binary.LittleEndian.Uint32(data[base+2*4:]))
		if name == "" {
			continue
		}
		entry := soundEntry{directory: readString(binary.LittleEndian.Uint32(data[base+23*4:]))}
		for field := 3; field <= 12; field++ {
			if file := readString(binary.LittleEndian.Uint32(data[base+field*4:])); file != "" {
				entry.files = append(entry.files, file)
			}
		}
		if len(entry.files) > 0 {
			result[strings.ToLower(name)] = entry
		}
	}
	return result
}

func decodeWAVToPCM(data []byte) ([]byte, error) {
	if len(data) < 12 || string(data[:4]) != "RIFF" || string(data[8:12]) != "WAVE" {
		return nil, fmt.Errorf("unsupported audio format")
	}
	var format, channels, sampleRate, bits int
	var payload []byte
	for offset := 12; offset+8 <= len(data); {
		kind := string(data[offset : offset+4])
		size := int(binary.LittleEndian.Uint32(data[offset+4 : offset+8]))
		start := offset + 8
		end := start + size
		if size < 0 || end > len(data) {
			return nil, fmt.Errorf("truncated WAV chunk")
		}
		switch kind {
		case "fmt ":
			if size < 16 {
				return nil, fmt.Errorf("truncated WAV format")
			}
			format = int(binary.LittleEndian.Uint16(data[start : start+2]))
			channels = int(binary.LittleEndian.Uint16(data[start+2 : start+4]))
			sampleRate = int(binary.LittleEndian.Uint32(data[start+4 : start+8]))
			bits = int(binary.LittleEndian.Uint16(data[start+14 : start+16]))
		case "data":
			payload = data[start:end]
		}
		offset = end + size&1
	}
	if format != 1 || channels < 1 || channels > 2 || sampleRate < 1 || len(payload) == 0 {
		return nil, fmt.Errorf("unsupported WAV parameters")
	}
	if bits != 8 && bits != 16 && bits != 24 && bits != 32 {
		return nil, fmt.Errorf("unsupported WAV sample width %d", bits)
	}
	bytesPerSample := bits / 8
	frameSize := channels * bytesPerSample
	if frameSize == 0 || len(payload)%frameSize != 0 {
		return nil, fmt.Errorf("invalid WAV sample data")
	}
	frames := len(payload) / frameSize
	targetFrames := int((int64(frames)*audioSampleRate + int64(sampleRate)/2) / int64(sampleRate))
	pcm := make([]byte, targetFrames*2*2)
	for frame := 0; frame < targetFrames; frame++ {
		sourceFrame := int((int64(frame) * int64(sampleRate)) / audioSampleRate)
		if sourceFrame >= frames {
			sourceFrame = frames - 1
		}
		for channel := 0; channel < 2; channel++ {
			sourceChannel := channel
			if sourceChannel >= channels {
				sourceChannel = channels - 1
			}
			position := sourceFrame*frameSize + sourceChannel*bytesPerSample
			sample := wavSample(payload[position:position+bytesPerSample], bits)
			at := (frame*2 + channel) * 2
			binary.LittleEndian.PutUint16(pcm[at:at+2], uint16(sample))
		}
	}
	return pcm, nil
}

func wavSample(data []byte, bits int) int16 {
	switch bits {
	case 8:
		return int16((int(data[0]) - 128) << 8)
	case 16:
		return int16(binary.LittleEndian.Uint16(data))
	case 24:
		value := int32(data[0]) | int32(data[1])<<8 | int32(data[2])<<16
		if value&0x800000 != 0 {
			value |= ^0xffffff
		}
		return int16(value >> 8)
	default:
		return int16(int32(binary.LittleEndian.Uint32(data)) >> 16)
	}
}

func parseVolume(value string) float64 {
	volume := 0.0
	if _, err := fmt.Sscanf(value, "%f", &volume); err != nil {
		return 0
	}
	return math.Max(0, math.Min(1, volume))
}

type loopReader struct {
	data     []byte
	position int
}

func newLoopReader(data []byte) *loopReader { return &loopReader{data: data} }

func (r *loopReader) Read(output []byte) (int, error) {
	if len(r.data) == 0 {
		return 0, io.EOF
	}
	read := 0
	for read < len(output) {
		count := copy(output[read:], r.data[r.position:])
		read += count
		r.position = (r.position + count) % len(r.data)
	}
	return read, nil
}

func (r *loopReader) Seek(offset int64, whence int) (int64, error) {
	if len(r.data) == 0 {
		return 0, io.EOF
	}
	var position int64
	switch whence {
	case io.SeekStart:
		position = offset
	case io.SeekCurrent:
		position = int64(r.position) + offset
	case io.SeekEnd:
		position = int64(len(r.data)) + offset
	default:
		return 0, fmt.Errorf("invalid seek origin")
	}
	position %= int64(len(r.data))
	if position < 0 {
		position += int64(len(r.data))
	}
	r.position = int(position)
	return position, nil
}

type osErrNotExist struct{}

func (osErrNotExist) Error() string { return "sound asset not found" }
