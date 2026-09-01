package ui

import (
	"encoding/binary"
	"fmt"
	"image"
	"strings"

	"github.com/mgvs/go-mpeg4/m4v"
)

type movieSample struct {
	offset int
	size   int
}

type movieClip struct {
	data          []byte
	prefix        []byte
	frames        []movieSample
	audio         []byte
	keyframes     []int
	fps           float64
	width         int
	height        int
	videoStream   int
	audioStream   int
	audioFormat   uint16
	audioChannels int
	audioRate     int
}

type moviePlayback struct {
	file       string
	clip       *movieClip
	frameIndex int
	elapsed    float64
	image      image.Image
	finished   bool
}

type movieHeaderInfo struct {
	videoStream   int
	audioStream   int
	width         int
	height        int
	microseconds  uint32
	rate          uint32
	scale         uint32
	audioFormat   uint16
	audioChannels int
	audioRate     int
}

func (eng *UIEngine) ensureMovie(file string, volume float64) {
	if eng.movieFile == file {
		return
	}
	if eng.movie != nil {
		if host, ok := eng.Rt.Host.(MovieAudioHost); ok {
			host.StopMovieAudio()
		}
	}
	eng.movieFile = file
	eng.movieImage = nil
	eng.movie = nil
	if file == "" || eng.AssetLoader == nil {
		return
	}
	path := file
	if !strings.HasSuffix(strings.ToLower(path), ".avi") {
		path += ".avi"
	}
	data, err := eng.AssetLoader.ReadFile(path)
	if err != nil {
		return
	}
	clip, err := parseMovieClip(data)
	if err != nil {
		return
	}
	playback := &moviePlayback{file: file, clip: clip}
	if frame, err := playback.decode(0); err == nil {
		playback.image = frame
	}
	eng.movie = playback
	eng.movieImage = playback.image
	if len(clip.audio) > 0 && clip.audioFormat == 85 && clip.audioRate > 0 && clip.audioChannels > 0 {
		if host, ok := eng.Rt.Host.(MovieAudioHost); ok {
			host.PlayMovieAudio(clip.audio, clip.audioRate, clip.audioChannels, volume)
		}
	}
}

func (eng *UIEngine) updateMovie(elapsed float64) bool {
	var active *widget
	var visit func(*widget)
	visit = func(w *widget) {
		if active != nil || !w.shown {
			return
		}
		if w.kind == kindMovieFrame && w.movieActive {
			active = w
			return
		}
		for _, child := range w.children {
			visit(child)
		}
	}
	if root := eng.Rt.widgets["GlueParent"]; root != nil {
		for _, child := range root.children {
			visit(child)
		}
	}
	if active == nil {
		if eng.movie != nil {
			if host, ok := eng.Rt.Host.(MovieAudioHost); ok {
				host.StopMovieAudio()
			}
			eng.movie = nil
			eng.movieFile = ""
			eng.movieImage = nil
			return true
		}
		return false
	}
	eng.ensureMovie(active.movieFile, float64(active.movieVolume)/255)
	if eng.movie == nil || eng.movie.finished {
		return false
	}
	if eng.movie.advance(elapsed) {
		eng.movieImage = eng.movie.image
		return true
	}
	return false
}

func (p *moviePlayback) advance(elapsed float64) bool {
	if p == nil || p.clip == nil || p.finished || len(p.clip.frames) < 2 || elapsed <= 0 {
		return false
	}
	p.elapsed += elapsed
	frameDuration := 1 / p.clip.fps
	if frameDuration <= 0 {
		frameDuration = 1.0 / 30
	}
	changed := false
	for p.elapsed >= frameDuration && !p.finished {
		p.elapsed -= frameDuration
		p.frameIndex++
		if p.frameIndex >= len(p.clip.frames) {
			p.frameIndex = len(p.clip.frames) - 1
			p.finished = true
			break
		}
		if movieVOPType(p.clip.data[p.clip.frames[p.frameIndex].offset:p.clip.frames[p.frameIndex].offset+p.clip.frames[p.frameIndex].size]) != 0 {
			continue
		}
		frame, err := p.decode(p.frameIndex)
		if err != nil {
			continue
		}
		p.image = frame
		changed = true
	}
	return changed
}

func (p *moviePlayback) decode(index int) (image.Image, error) {
	if p == nil || p.clip == nil || index < 0 || index >= len(p.clip.frames) {
		return nil, fmt.Errorf("movie frame %d out of range", index)
	}
	sample := p.clip.frames[index]
	if sample.offset < 0 || sample.size <= 0 || sample.offset+sample.size > len(p.clip.data) {
		return nil, fmt.Errorf("movie frame %d data out of range", index)
	}
	data := p.clip.data[sample.offset : sample.offset+sample.size]
	stream := data
	if !containsMovieVOL(data) && len(p.clip.prefix) > 0 {
		stream = make([]byte, 0, len(p.clip.prefix)+len(data))
		stream = append(stream, p.clip.prefix...)
		stream = append(stream, data...)
	}
	vol, vop, reader, err := m4v.ParseHeaders(stream)
	if err != nil {
		return nil, err
	}
	if vop == nil || vop.PredictionType != 0 {
		return nil, fmt.Errorf("movie frame %d is not an intra frame", index)
	}
	frame, err := m4v.DecodeIVOP(reader, vol, vop)
	if frame == nil {
		return nil, err
	}
	return frame.YCbCr(), err
}

func parseMovieClip(data []byte) (*movieClip, error) {
	if len(data) < 12 || string(data[:4]) != "RIFF" || string(data[8:12]) != "AVI " {
		return nil, fmt.Errorf("movie is not AVI")
	}
	info := movieHeaderInfo{videoStream: -1, audioStream: -1}
	var moviStart, moviEnd int
	for offset := 12; offset < len(data); {
		id, _, payload, end, next, ok := movieChunk(data, offset)
		if !ok {
			break
		}
		if id == "LIST" && payload+4 <= end {
			listType := string(data[payload : payload+4])
			switch listType {
			case "hdrl":
				scanMovieHeaders(data, payload+4, end, &info)
			case "movi":
				moviStart, moviEnd = payload+4, end
			}
		}
		offset = next
	}
	if moviStart == 0 || moviEnd <= moviStart {
		return nil, fmt.Errorf("movie has no movi list")
	}
	clip := &movieClip{data: data, videoStream: info.videoStream, width: info.width, height: info.height, audioStream: info.audioStream, audioFormat: info.audioFormat, audioChannels: info.audioChannels, audioRate: info.audioRate}
	scanMovieFrames(data, moviStart, moviEnd, info.videoStream, info.audioStream, clip)
	if len(clip.frames) == 0 {
		return nil, fmt.Errorf("movie has no video frames")
	}
	first := clip.frames[0]
	clip.prefix = movieVOLPrefix(data[first.offset : first.offset+first.size])
	for index, sample := range clip.frames {
		if movieVOPType(data[sample.offset:sample.offset+sample.size]) == 0 {
			clip.keyframes = append(clip.keyframes, index)
		}
	}
	if info.rate > 0 && info.scale > 0 {
		clip.fps = float64(info.rate) / float64(info.scale)
	} else if info.microseconds > 0 {
		clip.fps = 1000000 / float64(info.microseconds)
	}
	if clip.fps <= 0 || clip.fps > 240 {
		clip.fps = 30
	}
	return clip, nil
}

func scanMovieHeaders(data []byte, start, end int, info *movieHeaderInfo) {
	stream := 0
	for offset := start; offset < end; {
		id, _, payload, chunkEnd, next, ok := movieChunk(data, offset)
		if !ok || chunkEnd > end {
			return
		}
		if id == "avih" && payload+20 <= chunkEnd {
			info.microseconds = binary.LittleEndian.Uint32(data[payload : payload+4])
		}
		if id == "LIST" && payload+4 <= chunkEnd {
			listType := string(data[payload : payload+4])
			if listType == "strl" {
				scanMovieStream(data, payload+4, chunkEnd, stream, info)
				stream++
			} else {
				scanMovieHeaders(data, payload+4, chunkEnd, info)
			}
		}
		offset = next
	}
}

func scanMovieStream(data []byte, start, end, stream int, info *movieHeaderInfo) {
	isVideo := false
	isAudio := false
	for offset := start; offset < end; {
		id, _, payload, chunkEnd, next, ok := movieChunk(data, offset)
		if !ok || chunkEnd > end {
			return
		}
		if id == "strh" && payload+36 <= chunkEnd {
			streamType := string(data[payload : payload+4])
			isVideo = streamType == "vids"
			isAudio = streamType == "auds"
			if isVideo && info.videoStream < 0 {
				info.videoStream = stream
			}
			if isAudio && info.audioStream < 0 {
				info.audioStream = stream
			}
			if isVideo {
				info.scale = binary.LittleEndian.Uint32(data[payload+20 : payload+24])
				info.rate = binary.LittleEndian.Uint32(data[payload+24 : payload+28])
			}
		}
		if id == "strf" && isVideo && payload+20 <= chunkEnd {
			info.width = int(int32(binary.LittleEndian.Uint32(data[payload+4 : payload+8])))
			info.height = int(int32(binary.LittleEndian.Uint32(data[payload+8 : payload+12])))
		}
		if id == "strf" && isAudio && payload+16 <= chunkEnd {
			info.audioFormat = binary.LittleEndian.Uint16(data[payload : payload+2])
			info.audioChannels = int(binary.LittleEndian.Uint16(data[payload+2 : payload+4]))
			info.audioRate = int(binary.LittleEndian.Uint32(data[payload+4 : payload+8]))
		}
		offset = next
	}
}

func scanMovieFrames(data []byte, start, end, videoStream, audioStream int, clip *movieClip) {
	for offset := start; offset < end; {
		id, size, payload, chunkEnd, next, ok := movieChunk(data, offset)
		if !ok || chunkEnd > end {
			return
		}
		if id == "LIST" && payload+4 <= chunkEnd {
			scanMovieFrames(data, payload+4, chunkEnd, videoStream, audioStream, clip)
		} else if isMovieVideoChunk(id, videoStream) && size > 0 {
			clip.frames = append(clip.frames, movieSample{offset: payload, size: size})
		} else if isMovieAudioChunk(id, audioStream) && size > 0 {
			clip.audio = append(clip.audio, data[payload:chunkEnd]...)
		}
		offset = next
	}
}

func movieChunk(data []byte, offset int) (string, int, int, int, int, bool) {
	if offset < 0 || offset+8 > len(data) {
		return "", 0, 0, 0, 0, false
	}
	size64 := uint64(binary.LittleEndian.Uint32(data[offset+4 : offset+8]))
	payload64 := uint64(offset + 8)
	end64 := payload64 + size64
	if end64 > uint64(len(data)) || end64 < payload64 {
		return "", 0, 0, 0, 0, false
	}
	next64 := end64 + (size64 & 1)
	if next64 > uint64(len(data)) {
		next64 = uint64(len(data))
	}
	return string(data[offset : offset+4]), int(size64), int(payload64), int(end64), int(next64), true
}

func isMovieVideoChunk(id string, stream int) bool {
	if len(id) != 4 || (id[2:] != "dc" && id[2:] != "db") {
		return false
	}
	if stream < 0 {
		return true
	}
	return id[0] == byte('0'+stream/10) && id[1] == byte('0'+stream%10)
}

func isMovieAudioChunk(id string, stream int) bool {
	if len(id) != 4 || id[2:] != "wb" {
		return false
	}
	if stream < 0 {
		return true
	}
	return id[0] == byte('0'+stream/10) && id[1] == byte('0'+stream%10)
}

func containsMovieVOL(data []byte) bool {
	return strings.Contains(string(data), "\x00\x00\x01\x00")
}

func movieVOPType(data []byte) int {
	for index := 0; index+4 < len(data); index++ {
		if data[index] == 0 && data[index+1] == 0 && data[index+2] == 1 && data[index+3] == 0xb6 {
			return int(data[index+4] >> 6)
		}
	}
	return -1
}

func movieVOLPrefix(data []byte) []byte {
	for index := 0; index+3 < len(data); index++ {
		if data[index] == 0 && data[index+1] == 0 && data[index+2] == 1 && data[index+3] == 0xb6 {
			return append([]byte(nil), data[:index]...)
		}
	}
	return nil
}
