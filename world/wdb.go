package world

import (
	"encoding/binary"
	"os"
	"path/filepath"
	"sync"
	"time"
)

const (
	wdbBuild      = 12340
	wdbFormat     = 0x6c
	wdbFormatPart = 1
)

// wdbCacheKind is one of the files of query answers the client keeps: the
// answer to a kind of query, kept by the number the query asked about.
type wdbCacheKind struct {
	file     string
	magic    uint32
	query    ClientOpcode
	response uint16
	// player marks the cache of player names, asked for by a whole guid and
	// answered with a packed one.
	player bool
}

var wdbKinds = []wdbCacheKind{
	{"creaturecache.wdb", 0x574d4f42, CreatureQueryOpcode, CreatureQueryResponse, false},
	{"gameobjectcache.wdb", 0x57474f42, GameObjectQueryOpcode, GameObjectQueryResponse, false},
	{"itemcache.wdb", 0x57494442, ItemQuerySingleOpcode, ItemQuerySingleResponse, false},
	{"npccache.wdb", 0x574e5043, ClientNPCTextQuery, NPCTextUpdate, false},
	{"questcache.wdb", 0x57515354, ClientQuestQuery, QuestQueryResponse, false},
	{"pagetextcache.wdb", 0x57505458, ClientPageTextQuery, PageTextQueryResponse, false},
	{"itemnamecache.wdb", 0x574e4442, 0x02C4, 0x02C5, false},
	{"guildcache.wdb", 0x57474c44, 0x0054, 0x0055, false},
	{"petnamecache.wdb", 0x57504e4d, 0x0052, 0x0053, false},
	{"petitioncache.wdb", 0x5750544e, 0x01C6, 0x01C7, false},
	{"arenateamcache.wdb", 0x5741544d, 0x034B, 0x034C, false},
	{"namecache.wdb", 0x574e414d, 0x0050, 0x0051, true},
}

// wdbUnknown marks the answer that says the thing asked about is not known.
const wdbUnknown = 0x80000000

type wdbFile struct {
	kind    wdbCacheKind
	records map[uint32][]byte
}

// QueryCache keeps what the server answered to queries between runs, so the
// same thing is not asked twice.
type QueryCache struct {
	mu     sync.Mutex
	dir    string
	locale uint32
	files  map[uint16]*wdbFile
	byAsk  map[ClientOpcode]*wdbFile
	dirty  bool
	stop   chan struct{}
}

var (
	wdbRoot   string
	wdbLocale string
)

// ConfigureQueryCache says where the client keeps its query answers: the
// folder of the game and the locale. Connections opened afterwards use it.
func ConfigureQueryCache(gameFolder, locale string) {
	wdbRoot, wdbLocale = gameFolder, locale
}

func openQueryCache() *QueryCache {
	if wdbRoot == "" || len(wdbLocale) != 4 {
		return nil
	}
	cache := &QueryCache{
		dir:    filepath.Join(wdbRoot, "Cache", "WDB", wdbLocale),
		locale: uint32(wdbLocale[0])<<24 | uint32(wdbLocale[1])<<16 | uint32(wdbLocale[2])<<8 | uint32(wdbLocale[3]),
		files:  map[uint16]*wdbFile{},
		byAsk:  map[ClientOpcode]*wdbFile{},
		stop:   make(chan struct{}),
	}
	for _, kind := range wdbKinds {
		file := &wdbFile{kind: kind, records: map[uint32][]byte{}}
		cache.files[kind.response] = file
		cache.byAsk[kind.query] = file
		cache.read(file)
	}
	go cache.flushLoop()
	return cache
}

func (cache *QueryCache) path(file *wdbFile) string { return filepath.Join(cache.dir, file.kind.file) }

// read takes the answers a file holds; a file of another build or locale is
// left alone.
func (cache *QueryCache) read(file *wdbFile) {
	data, err := os.ReadFile(cache.path(file))
	if err != nil || len(data) < 24 {
		return
	}
	word := func(offset int) uint32 { return binary.LittleEndian.Uint32(data[offset:]) }
	if word(0) != file.kind.magic || word(4) != wdbBuild || word(8) != cache.locale || word(12) != wdbFormat || word(16) != wdbFormatPart {
		return
	}
	for offset := 24; offset+8 <= len(data); {
		key, size := word(offset), int(word(offset+4))
		offset += 8
		if key == 0 || offset+size > len(data) {
			break
		}
		file.records[key] = append([]byte(nil), data[offset:offset+size]...)
		offset += size
	}
}

func (cache *QueryCache) write(file *wdbFile) {
	out := make([]byte, 24, 24+64*len(file.records))
	for index, value := range []uint32{file.kind.magic, wdbBuild, cache.locale, wdbFormat, wdbFormatPart, 0} {
		binary.LittleEndian.PutUint32(out[index*4:], value)
	}
	for key, payload := range file.records {
		out = binary.LittleEndian.AppendUint32(out, key)
		out = binary.LittleEndian.AppendUint32(out, uint32(len(payload)))
		out = append(out, payload...)
	}
	out = append(out, make([]byte, 8)...)
	_ = os.MkdirAll(cache.dir, 0o755)
	_ = os.WriteFile(cache.path(file), out, 0o644)
}

func (cache *QueryCache) flush() {
	cache.mu.Lock()
	defer cache.mu.Unlock()
	if !cache.dirty {
		return
	}
	cache.dirty = false
	for _, file := range cache.files {
		cache.write(file)
	}
}

func (cache *QueryCache) flushLoop() {
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			cache.flush()
		case <-cache.stop:
			cache.flush()
			return
		}
	}
}

func (cache *QueryCache) close() {
	close(cache.stop)
}

// remember keeps an answer from the server.
func (cache *QueryCache) remember(packet Packet) {
	file := cache.files[packet.Opcode]
	if file == nil || len(packet.Body) <= 4 {
		return
	}
	key, rest := binary.LittleEndian.Uint32(packet.Body), packet.Body[4:]
	if file.kind.player {
		var guid uint64
		guid, rest = unpackGUID(packet.Body)
		// An answer that says the player is not known is not kept.
		if guid == 0 || guid>>32 != 0 || len(rest) == 0 || rest[0] != 0 {
			return
		}
		key = uint32(guid)
	}
	if key == 0 || (!file.kind.player && key&wdbUnknown != 0) {
		return
	}
	cache.mu.Lock()
	file.records[key] = append([]byte(nil), rest...)
	cache.dirty = true
	cache.mu.Unlock()
}

// answer is the answer already kept for a query, as the server would send it.
func (cache *QueryCache) answer(opcode ClientOpcode, body []byte) (Packet, bool) {
	file := cache.byAsk[opcode]
	if file == nil || len(body) < 4 {
		return Packet{}, false
	}
	key := binary.LittleEndian.Uint32(body)
	if file.kind.player {
		if len(body) < 8 || binary.LittleEndian.Uint32(body[4:]) != 0 {
			return Packet{}, false
		}
	}
	cache.mu.Lock()
	payload, ok := file.records[key]
	cache.mu.Unlock()
	if !ok {
		return Packet{}, false
	}
	var reply []byte
	if file.kind.player {
		reply = AppendPackedGUID(nil, uint64(key))
	} else {
		reply = binary.LittleEndian.AppendUint32(nil, key)
	}
	return Packet{Opcode: file.kind.response, Body: append(reply, payload...)}, true
}

// unpackGUID reads a guid written in the packed form and gives what follows it.
func unpackGUID(body []byte) (uint64, []byte) {
	if len(body) == 0 {
		return 0, nil
	}
	mask, body := body[0], body[1:]
	var guid uint64
	for bit := 0; bit < 8; bit++ {
		if mask&(1<<bit) == 0 {
			continue
		}
		if len(body) == 0 {
			return 0, nil
		}
		guid |= uint64(body[0]) << (8 * bit)
		body = body[1:]
	}
	return guid, body
}
