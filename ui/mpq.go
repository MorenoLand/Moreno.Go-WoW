package ui

import (
	"bytes"
	"compress/bzip2"
	"compress/zlib"
	"encoding/binary"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

const (
	mpqMagic            = "MPQ\x1a"
	mpqHashEmpty        = 0xffffffff
	mpqHashDeleted      = 0xfffffffe
	mpqFlagImplode      = 0x00000100
	mpqFlagCompress     = 0x00000200
	mpqFlagEncrypted    = 0x00010000
	mpqFlagFixKey       = 0x00020000
	mpqFlagPatch        = 0x00100000
	mpqFlagSingleUnit   = 0x01000000
	mpqFlagDeleteMarker = 0x02000000
	mpqFlagSectorCrc    = 0x04000000
	mpqFlagExists       = 0x80000000
	maxMPQTableEntries  = 1 << 24
	maxMPQFileSize      = 1 << 30
)

type mpqHashEntry struct {
	hashA    uint32
	hashB    uint32
	locale   uint16
	platform uint16
	block    uint32
}

type mpqBlockEntry struct {
	position       uint32
	compressedSize uint32
	fileSize       uint32
	flags          uint32
}

type mpqArchive struct {
	file         *os.File
	path         string
	headerOffset int64
	blockShift   uint16
	index        map[mpqHashKey][]mpqHashEntry
	blocks       []mpqBlockEntry
}

type mpqHashKey struct {
	hashA uint32
	hashB uint32
}

type mpqFileRef struct {
	archive *mpqArchive
	block   mpqBlockEntry
}

type mpqSet struct {
	archives   []*mpqArchive
	locale     uint16
	files      map[string]mpqFileRef
	loose      map[string]string
	looseRoots []string
	missing    map[string]struct{}
}

func openMPQSet(dataPath, locale string) (*mpqSet, error) {
	root, err := findMPQDataRoot(dataPath)
	if err != nil {
		return nil, err
	}
	paths, err := discoverMPQArchives(root, locale)
	if err != nil {
		return nil, err
	}
	if len(paths) == 0 {
		return nil, fmt.Errorf("no recognized MPQ archives under %s", dataPath)
	}
	set := &mpqSet{locale: mpqLocaleID(locale), files: make(map[string]mpqFileRef), loose: make(map[string]string), missing: make(map[string]struct{})}
	set.looseRoots = append(set.looseRoots, root)
	if localeDir := findLocaleDirectory(root, locale); localeDir != "" {
		set.looseRoots = append(set.looseRoots, localeDir)
	}
	seen := make(map[string]struct{}, len(paths))
	for _, path := range paths {
		key := strings.ToLower(filepath.Clean(path))
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		archive, err := openMPQArchive(path)
		if err != nil {
			set.Close()
			return nil, err
		}
		set.archives = append(set.archives, archive)
	}
	if len(set.archives) == 0 {
		return nil, fmt.Errorf("no MPQ archives found under %s", dataPath)
	}
	return set, nil
}

func (set *mpqSet) Close() error {
	var first error
	for _, archive := range set.archives {
		if err := archive.file.Close(); err != nil && first == nil {
			first = err
		}
	}
	set.archives = nil
	return first
}

func (set *mpqSet) ReadFile(name string) ([]byte, error) {
	name = normalizeMPQPath(name)
	if ref, ok := set.files[name]; ok {
		return ref.archive.readBlock(name, ref.block)
	}
	if _, ok := set.missing[name]; ok {
		return nil, os.ErrNotExist
	}
	var lastReadErr error
	for index := len(set.archives) - 1; index >= 0; index-- {
		archive := set.archives[index]
		block, ok := archive.findBlock(name, set.locale)
		if !ok {
			continue
		}
		data, err := archive.readBlock(name, block)
		if err != nil {
			lastReadErr = err
			continue
		}
		set.files[name] = mpqFileRef{archive: archive, block: block}
		return data, nil
	}
	if path, ok := set.loose[name]; ok {
		return os.ReadFile(path)
	}
	for _, root := range set.looseRoots {
		if path, ok := resolveLoosePath(root, name); ok {
			set.loose[name] = path
			return os.ReadFile(path)
		}
	}
	if lastReadErr != nil {
		return nil, lastReadErr
	}
	set.missing[name] = struct{}{}
	return nil, os.ErrNotExist
}

func findMPQDataRoot(dataPath string) (string, error) {
	dataPath = strings.TrimSpace(dataPath)
	if dataPath == "" {
		return "", fmt.Errorf("MPQ data path is empty")
	}
	info, err := os.Stat(dataPath)
	if err != nil {
		return "", fmt.Errorf("MPQ data path %s: %w", dataPath, err)
	}
	root := dataPath
	if !info.IsDir() {
		root = filepath.Dir(dataPath)
	}
	if !strings.EqualFold(filepath.Base(root), "Data") {
		dataRoot := filepath.Join(root, "Data")
		dataInfo, dataErr := os.Stat(dataRoot)
		if dataErr != nil || !dataInfo.IsDir() {
			return "", fmt.Errorf("MPQ data path must point to a Data directory: %s", dataPath)
		}
		root = dataRoot
	}
	if !hasMPQFile(root) {
		return "", fmt.Errorf("no MPQ archives found under Data directory %s", root)
	}
	return root, nil
}

func findMPQDataRoots(dataPath string) ([]string, error) {
	root, err := findMPQDataRoot(dataPath)
	if err != nil {
		return nil, err
	}
	return []string{root}, nil
}

func hasMPQFile(root string) bool {
	found := false
	_ = filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !entry.IsDir() && strings.EqualFold(filepath.Ext(entry.Name()), ".mpq") {
			found = true
			return fs.SkipAll
		}
		return nil
	})
	return found
}

func findLocaleDirectory(root, locale string) string {
	entries, err := os.ReadDir(root)
	if err != nil {
		return ""
	}
	for _, entry := range entries {
		if entry.IsDir() && strings.EqualFold(entry.Name(), locale) {
			return filepath.Join(root, entry.Name())
		}
	}
	return ""
}

func resolveLoosePath(root, name string) (string, bool) {
	parts := strings.Split(strings.Trim(strings.ReplaceAll(normalizeMPQPath(name), "\\", "/"), "/"), "/")
	dir := root
	for _, part := range parts {
		entries, err := os.ReadDir(dir)
		if err != nil {
			return "", false
		}
		found := ""
		for _, entry := range entries {
			if strings.EqualFold(entry.Name(), part) {
				found = filepath.Join(dir, entry.Name())
				break
			}
		}
		if found == "" {
			return "", false
		}
		dir = found
	}
	info, err := os.Stat(dir)
	return dir, err == nil && !info.IsDir()
}

// fixedMPQArchives lists non-patch archives lowest priority first (later overrides).
var fixedMPQArchives = []string{"alternate.MPQ", "interface.MPQ", "misc.MPQ", "model.MPQ", "texture.MPQ", "terrain.MPQ", "wmo.MPQ", "sound.MPQ", "fonts.MPQ", "dbc.MPQ", "speech.MPQ", "common.MPQ", "common-2.MPQ", "expansion.MPQ", "lichking.MPQ", "expansionloc.MPQ", "lichkingloc.MPQ", "expansionspeech.MPQ", "lichkingspeech.MPQ"}

func discoverMPQArchives(root, locale string) ([]string, error) {
	locale = strings.ToLower(strings.TrimSpace(locale))
	if locale == "" {
		locale = "enus"
	}
	rootEntries, err := mpqDirectoryRecursive(root, locale)
	if err != nil {
		return nil, err
	}
	localeDir := findLocaleDirectory(root, locale)
	localeEntries := map[string]string{}
	if localeDir != "" {
		localeEntries, err = mpqDirectoryRecursive(localeDir, "")
		if err != nil {
			return nil, err
		}
	}
	paths := make([]string, 0, len(fixedMPQArchives)+16)
	appendIfPresent := func(entries map[string]string, name string) {
		if path, ok := entries[strings.ToLower(name)]; ok {
			paths = append(paths, path)
		}
	}
	for _, name := range fixedMPQArchives {
		appendIfPresent(rootEntries, name)
	}
	for _, name := range []string{"base-" + locale + ".MPQ", "locale-" + locale + ".MPQ", "speech-" + locale + ".MPQ", "expansion-locale-" + locale + ".MPQ", "lichking-locale-" + locale + ".MPQ", "expansion-speech-" + locale + ".MPQ", "lichking-speech-" + locale + ".MPQ"} {
		appendIfPresent(rootEntries, name)
	}
	for _, name := range []string{"base-" + locale + ".MPQ", "locale-" + locale + ".MPQ", "speech-" + locale + ".MPQ", "expansion-locale-" + locale + ".MPQ", "lichking-locale-" + locale + ".MPQ", "expansion-speech-" + locale + ".MPQ", "lichking-speech-" + locale + ".MPQ"} {
		appendIfPresent(localeEntries, name)
	}
	patches := make([]mpqPatchPath, 0)
	addPatches := func(entries map[string]string) {
		for name, path := range entries {
			if number, localePatch, ok := classifyPatchArchive(name, locale); ok {
				patches = append(patches, mpqPatchPath{path: path, number: number, locale: localePatch})
			}
		}
	}
	addPatches(rootEntries)
	addPatches(localeEntries)
	sort.SliceStable(patches, func(i, j int) bool {
		if patches[i].number != patches[j].number {
			return patches[i].number < patches[j].number
		}
		if patches[i].locale != patches[j].locale {
			return !patches[i].locale && patches[j].locale
		}
		return strings.ToLower(patches[i].path) < strings.ToLower(patches[j].path)
	})
	for _, patch := range patches {
		paths = append(paths, patch.path)
	}
	if len(paths) == 0 {
		return nil, fmt.Errorf("no recognized MPQ archives found under %s", root)
	}
	return paths, nil
}

type mpqPatchPath struct {
	path   string
	number int
	locale bool
}

func mpqDirectory(root string) (map[string]string, error) {
	return mpqDirectoryRecursive(root, "")
}

func mpqDirectoryRecursive(root, excludedDir string) (map[string]string, error) {
	result := make(map[string]string)
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			if path != root && excludedDir != "" && strings.EqualFold(entry.Name(), excludedDir) {
				return fs.SkipDir
			}
			return nil
		}
		if strings.EqualFold(filepath.Ext(entry.Name()), ".mpq") {
			name := strings.ToLower(entry.Name())
			if _, exists := result[name]; !exists {
				result[name] = path
			}
		}
		return nil
	})
	if os.IsNotExist(err) {
		return result, nil
	}
	if err != nil {
		return nil, err
	}
	return result, nil
}

// classifyPatchArchive recognizes patch.MPQ, patch-N.MPQ, patch-{locale}.MPQ,
// patch-{locale}-N.MPQ, and lettered private-server patches (patch-A / patch-enUS-A).
// Numeric generations share one ladder so patch-4 overrides patch-enUS-3.
func classifyPatchArchive(name, locale string) (number int, localePatch bool, ok bool) {
	base := strings.ToLower(strings.TrimSuffix(name, filepath.Ext(name)))
	locale = strings.ToLower(strings.TrimSpace(locale))
	if locale != "" {
		prefix := "patch-" + locale
		if base == prefix {
			return 0, true, true
		}
		if strings.HasPrefix(base, prefix+"-") {
			rest := strings.TrimPrefix(base, prefix+"-")
			if n, parsed := parsePatchGeneration(rest); parsed {
				return n, true, true
			}
		}
	}
	if base == "patch" {
		return 0, false, true
	}
	if !strings.HasPrefix(base, "patch-") {
		return 0, false, false
	}
	rest := strings.TrimPrefix(base, "patch-")
	if locale != "" && (rest == locale || strings.HasPrefix(rest, locale+"-")) {
		return 0, false, false
	}
	if strings.Contains(rest, "-") {
		return 0, false, false
	}
	if n, parsed := parsePatchGeneration(rest); parsed {
		return n, false, true
	}
	return 0, false, false
}

func parsePatchGeneration(rest string) (int, bool) {
	if rest == "" {
		return 0, false
	}
	if number, err := strconv.Atoi(rest); err == nil && number >= 1 {
		return number, true
	}
	if len(rest) == 1 {
		letter := rest[0]
		if letter >= 'a' && letter <= 'z' {
			return 1000 + int(letter-'a'), true
		}
	}
	return 0, false
}

func rootPatchNumber(name string) (int, bool) {
	number, isLocale, ok := classifyPatchArchive(name, "")
	return number, ok && !isLocale
}

func localePatchNumber(name, locale string) (int, bool) {
	number, isLocale, ok := classifyPatchArchive(name, locale)
	return number, ok && isLocale
}

func mpqLocaleID(locale string) uint16 {
	switch strings.ToLower(strings.TrimSpace(locale)) {
	case "enus":
		return 0x0409
	case "engb":
		return 0x0809
	case "de de", "dede":
		return 0x0407
	case "frfr":
		return 0x040c
	case "eses":
		return 0x0c0a
	case "esmx":
		return 0x080a
	case "itit":
		return 0x0410
	case "ptbr":
		return 0x0416
	case "ruru":
		return 0x0419
	case "kokr":
		return 0x0412
	case "zhcn":
		return 0x0804
	case "zhtw":
		return 0x0404
	default:
		return localeNeutral
	}
}

func openMPQArchive(path string) (*mpqArchive, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	archive := &mpqArchive{file: file, path: path}
	closeOnError := func(err error) (*mpqArchive, error) {
		_ = file.Close()
		return nil, err
	}
	headerOffset, err := findMPQHeader(file)
	if err != nil {
		return closeOnError(fmt.Errorf("%s: %w", path, err))
	}
	archive.headerOffset = headerOffset
	header := make([]byte, 32)
	if _, err := file.ReadAt(header, headerOffset); err != nil {
		return closeOnError(err)
	}
	if string(header[:4]) != mpqMagic {
		return closeOnError(fmt.Errorf("invalid MPQ signature"))
	}
	headerSize := binary.LittleEndian.Uint32(header[4:8])
	formatVersion := binary.LittleEndian.Uint16(header[12:14])
	archive.blockShift = binary.LittleEndian.Uint16(header[14:16])
	hashPosition := binary.LittleEndian.Uint32(header[16:20])
	blockPosition := binary.LittleEndian.Uint32(header[20:24])
	hashEntries := binary.LittleEndian.Uint32(header[24:28])
	blockEntries := binary.LittleEndian.Uint32(header[28:32])
	if headerSize < 32 || formatVersion > 1 || archive.blockShift > 20 || hashEntries == 0 || blockEntries == 0 || hashEntries > maxMPQTableEntries || blockEntries > maxMPQTableEntries {
		return closeOnError(fmt.Errorf("unsupported MPQ header version=%d header_size=%d hash_entries=%d block_entries=%d", formatVersion, headerSize, hashEntries, blockEntries))
	}
	hashData, err := readAt(file, headerOffset+int64(hashPosition), int(hashEntries)*16)
	if err != nil {
		return closeOnError(err)
	}
	decryptMPQ(hashData, hashString("(hash table)", hashTypeFileKey))
	archive.index = make(map[mpqHashKey][]mpqHashEntry, hashEntries/4)
	for i := uint32(0); i < hashEntries; i++ {
		at := i * 16
		entry := mpqHashEntry{hashA: binary.LittleEndian.Uint32(hashData[at:]), hashB: binary.LittleEndian.Uint32(hashData[at+4:]), locale: binary.LittleEndian.Uint16(hashData[at+8:]), platform: binary.LittleEndian.Uint16(hashData[at+10:]), block: binary.LittleEndian.Uint32(hashData[at+12:])}
		if entry.block != mpqHashEmpty && entry.block != mpqHashDeleted {
			key := mpqHashKey{hashA: entry.hashA, hashB: entry.hashB}
			archive.index[key] = append(archive.index[key], entry)
		}
	}
	blockData, err := readAt(file, headerOffset+int64(blockPosition), int(blockEntries)*16)
	if err != nil {
		return closeOnError(err)
	}
	decryptMPQ(blockData, hashString("(block table)", hashTypeFileKey))
	archive.blocks = make([]mpqBlockEntry, blockEntries)
	for i := range archive.blocks {
		at := i * 16
		archive.blocks[i] = mpqBlockEntry{position: binary.LittleEndian.Uint32(blockData[at:]), compressedSize: binary.LittleEndian.Uint32(blockData[at+4:]), fileSize: binary.LittleEndian.Uint32(blockData[at+8:]), flags: binary.LittleEndian.Uint32(blockData[at+12:])}
	}
	return archive, nil
}

func findMPQHeader(file *os.File) (int64, error) {
	info, err := file.Stat()
	if err != nil {
		return 0, err
	}
	limit := info.Size()
	if limit > 1<<20 {
		limit = 1 << 20
	}
	for offset := int64(0); offset+4 <= limit; offset += 0x200 {
		var magic [4]byte
		if _, err := file.ReadAt(magic[:], offset); err != nil {
			return 0, err
		}
		if string(magic[:]) == mpqMagic {
			return offset, nil
		}
	}
	return 0, fmt.Errorf("MPQ header not found")
}

func (archive *mpqArchive) findBlock(name string, locale uint16) (mpqBlockEntry, bool) {
	if len(archive.index) == 0 {
		return mpqBlockEntry{}, false
	}
	name = normalizeMPQPath(name)
	key := mpqHashKey{hashA: hashString(name, hashTypeA), hashB: hashString(name, hashTypeB)}
	entries := archive.index[key]
	if len(entries) == 0 {
		return mpqBlockEntry{}, false
	}
	for _, wantedLocale := range []uint16{locale, localeNeutral, localeAny} {
		for _, entry := range entries {
			if entry.block >= uint32(len(archive.blocks)) {
				continue
			}
			if wantedLocale != localeAny && entry.locale != wantedLocale {
				continue
			}
			return archive.blocks[entry.block], true
		}
	}
	return mpqBlockEntry{}, false
}

func (archive *mpqArchive) readBlock(name string, block mpqBlockEntry) ([]byte, error) {
	if block.flags&mpqFlagDeleteMarker != 0 || block.flags&mpqFlagPatch != 0 {
		return nil, os.ErrNotExist
	}
	if block.fileSize > maxMPQFileSize || block.compressedSize > maxMPQFileSize {
		return nil, fmt.Errorf("MPQ file %s is too large", name)
	}
	// MPQ file-data keys hash only the base name (Storm/MPQ tooling), not the full path.
	keyName := normalizeMPQPath(name)
	if i := strings.LastIndex(keyName, `\`); i >= 0 {
		keyName = keyName[i+1:]
	}
	key := hashString(keyName, hashTypeFileKey)
	if block.flags&mpqFlagFixKey != 0 {
		key = (key + block.position) ^ block.fileSize
	}
	if block.flags&mpqFlagSingleUnit != 0 {
		data, err := readAt(archive.file, archive.headerOffset+int64(block.position), int(block.compressedSize))
		if err != nil {
			return nil, err
		}
		if block.flags&mpqFlagEncrypted != 0 {
			decryptMPQ(data, key)
		}
		return decodeMPQSector(data, int(block.fileSize), block.flags&mpqFlagCompress != 0 || block.flags&mpqFlagImplode != 0)
	}
	if block.compressedSize == block.fileSize && block.flags&(mpqFlagCompress|mpqFlagImplode|mpqFlagEncrypted|mpqFlagSectorCrc) == 0 {
		return readAt(archive.file, archive.headerOffset+int64(block.position), int(block.fileSize))
	}
	sectorSize := 512 << archive.blockShift
	sectorCount := (int(block.fileSize) + sectorSize - 1) / sectorSize
	offsetDataSize := (sectorCount + 1) * 4
	if block.flags&mpqFlagSectorCrc != 0 {
		offsetDataSize += sectorCount * 4
	}
	offsetData, err := readAt(archive.file, archive.headerOffset+int64(block.position), offsetDataSize)
	if err != nil {
		return nil, err
	}
	if block.flags&mpqFlagEncrypted != 0 {
		decryptMPQ(offsetData, key-1)
	}
	offsets := make([]uint32, sectorCount+1)
	for i := range offsets {
		offsets[i] = binary.LittleEndian.Uint32(offsetData[i*4:])
		if offsets[i] > block.compressedSize || (i > 0 && offsets[i] < offsets[i-1]) {
			return nil, fmt.Errorf("invalid MPQ sector offsets for %s", name)
		}
	}
	result := make([]byte, 0, block.fileSize)
	for i := 0; i < sectorCount; i++ {
		length := int(offsets[i+1] - offsets[i])
		sector, err := readAt(archive.file, archive.headerOffset+int64(block.position)+int64(offsets[i]), length)
		if err != nil {
			return nil, err
		}
		if block.flags&mpqFlagEncrypted != 0 {
			decryptMPQ(sector, key+uint32(i))
		}
		expected := sectorSize
		if remain := int(block.fileSize) - i*sectorSize; remain < expected {
			expected = remain
		}
		decoded, err := decodeMPQSector(sector, expected, block.flags&mpqFlagCompress != 0 || block.flags&mpqFlagImplode != 0)
		if err != nil {
			return nil, fmt.Errorf("decode MPQ sector %d of %s: %w", i, name, err)
		}
		result = append(result, decoded...)
	}
	if len(result) > int(block.fileSize) {
		result = result[:block.fileSize]
	}
	return result, nil
}

func decodeMPQSector(data []byte, expected int, compressed bool) ([]byte, error) {
	if !compressed {
		if len(data) < expected {
			return nil, io.ErrUnexpectedEOF
		}
		return data[:expected], nil
	}
	if len(data) == expected {
		return data, nil
	}
	if len(data) < 1 {
		return nil, io.ErrUnexpectedEOF
	}
	mask := data[0]
	payload := data[1:]
	var reader io.Reader
	switch {
	case mask&0x02 != 0:
		zreader, err := zlib.NewReader(bytes.NewReader(payload))
		if err != nil {
			return nil, err
		}
		defer zreader.Close()
		reader = zreader
	case mask&0x10 != 0:
		reader = bzip2.NewReader(bytes.NewReader(payload))
	case mask&0x08 != 0:
		return nil, fmt.Errorf("PKWARE implode compression is not supported")
	default:
		zreader, err := zlib.NewReader(bytes.NewReader(data))
		if err != nil {
			return nil, err
		}
		defer zreader.Close()
		reader = zreader
	}
	decoded, err := io.ReadAll(io.LimitReader(reader, int64(expected)+1))
	if err != nil {
		return nil, err
	}
	if len(decoded) < expected {
		return nil, io.ErrUnexpectedEOF
	}
	if len(decoded) > expected {
		decoded = decoded[:expected]
	}
	return decoded, nil
}

func readAt(file *os.File, offset int64, size int) ([]byte, error) {
	if size < 0 || size > maxMPQFileSize {
		return nil, fmt.Errorf("invalid MPQ read size %d", size)
	}
	data := make([]byte, size)
	if _, err := file.ReadAt(data, offset); err != nil {
		return nil, err
	}
	return data, nil
}

const (
	hashTypeName    = 0
	hashTypeA       = 1
	hashTypeB       = 2
	hashTypeFileKey = 3
	localeNeutral   = uint16(0)
	localeAny       = uint16(0xffff)
)

var mpqCryptTable = buildMPQCryptTable()

func buildMPQCryptTable() [0x500]uint32 {
	var table [0x500]uint32
	seed := uint32(0x00100001)
	for index := 0; index < 0x100; index++ {
		for row := 0; row < 5; row++ {
			seed = (seed*125 + 3) % 0x2aaaab
			temp1 := (seed & 0xffff) << 16
			seed = (seed*125 + 3) % 0x2aaaab
			temp2 := seed & 0xffff
			table[row*0x100+index] = temp1 | temp2
		}
	}
	return table
}

func hashString(name string, hashType int) uint32 {
	name = normalizeMPQPath(name)
	seed1 := uint32(0x7fed7fed)
	seed2 := uint32(0xeeeeeeee)
	for index := 0; index < len(name); index++ {
		value := byte(strings.ToUpper(name[index : index+1])[0])
		seed1 = mpqCryptTable[hashType*0x100+int(value)] ^ (seed1 + seed2)
		seed2 = uint32(value) + seed1 + seed2 + (seed2 << 5) + 3
	}
	return seed1
}

func decryptMPQ(data []byte, key uint32) {
	seed := uint32(0xeeeeeeee)
	for offset := 0; offset+4 <= len(data); offset += 4 {
		seed += mpqCryptTable[0x400+int(key&0xff)]
		value := binary.LittleEndian.Uint32(data[offset:]) ^ (key + seed)
		binary.LittleEndian.PutUint32(data[offset:], value)
		key = ((^key << 21) + 0x11111111) | (key >> 11)
		seed = value + seed + (seed << 5) + 3
	}
}

func normalizeMPQPath(name string) string {
	name = strings.ReplaceAll(name, "/", "\\")
	return strings.TrimPrefix(name, "\\")
}
