package ui

import (
	"bytes"
	"compress/bzip2"
	"compress/zlib"
	"encoding/binary"
	"fmt"
	"io"
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
	hashes       []mpqHashEntry
	blocks       []mpqBlockEntry
}

type mpqSet struct {
	archives []*mpqArchive
	locale   uint16
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
	set := &mpqSet{locale: mpqLocaleID(locale)}
	for _, path := range paths {
		archive, err := openMPQArchive(path)
		if err != nil {
			set.Close()
			return nil, err
		}
		set.archives = append(set.archives, archive)
	}
	if len(set.archives) == 0 {
		return nil, fmt.Errorf("no MPQ archives found under %s", root)
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
	for index := len(set.archives) - 1; index >= 0; index-- {
		archive := set.archives[index]
		block, ok := archive.findBlock(name, set.locale)
		if !ok {
			continue
		}
		return archive.readBlock(name, block)
	}
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
	if !info.IsDir() {
		return filepath.Dir(dataPath), nil
	}
	for _, candidate := range []string{dataPath, filepath.Join(dataPath, "Data")} {
		if hasMPQFile(candidate) {
			return candidate, nil
		}
	}
	return "", fmt.Errorf("no MPQ archives found under %s or its Data directory", dataPath)
}

func hasMPQFile(root string) bool {
	entries, err := os.ReadDir(root)
	if err != nil {
		return false
	}
	for _, entry := range entries {
		if !entry.IsDir() && strings.EqualFold(filepath.Ext(entry.Name()), ".mpq") {
			return true
		}
	}
	return false
}

type mpqPathRank struct {
	path string
	rank int
}

func discoverMPQArchives(root, locale string) ([]string, error) {
	locale = strings.ToLower(strings.TrimSpace(locale))
	if locale == "" {
		locale = "enus"
	}
	paths := make([]mpqPathRank, 0)
	appendDir := func(dir string, localized bool) error {
		entries, err := os.ReadDir(dir)
		if os.IsNotExist(err) {
			return nil
		}
		if err != nil {
			return err
		}
		for _, entry := range entries {
			if entry.IsDir() || !strings.EqualFold(filepath.Ext(entry.Name()), ".mpq") {
				continue
			}
			paths = append(paths, mpqPathRank{path: filepath.Join(dir, entry.Name()), rank: archiveRank(strings.TrimSuffix(entry.Name(), filepath.Ext(entry.Name())), localized)})
		}
		return nil
	}
	if err := appendDir(root, false); err != nil {
		return nil, err
	}
	localeDir := ""
	if entries, err := os.ReadDir(root); err == nil {
		for _, entry := range entries {
			if entry.IsDir() && strings.EqualFold(entry.Name(), locale) {
				localeDir = filepath.Join(root, entry.Name())
				break
			}
		}
	}
	if err := appendDir(localeDir, true); err != nil {
		return nil, err
	}
	if len(paths) == 0 {
		return nil, fmt.Errorf("no MPQ archives found under %s", root)
	}
	sort.SliceStable(paths, func(i, j int) bool {
		if paths[i].rank != paths[j].rank {
			return paths[i].rank < paths[j].rank
		}
		return strings.ToLower(paths[i].path) < strings.ToLower(paths[j].path)
	})
	result := make([]string, len(paths))
	for i, path := range paths {
		result[i] = path.path
	}
	return result, nil
}

func archiveRank(name string, localized bool) int {
	name = strings.ToLower(name)
	if strings.HasPrefix(name, "backup") {
		return 0
	}
	if localized {
		switch {
		case strings.HasPrefix(name, "base-"):
			return 4000
		case strings.HasPrefix(name, "locale-"):
			return 5000
		case strings.HasPrefix(name, "expansion-locale-"):
			return 5200
		case strings.HasPrefix(name, "lichking-locale-"):
			return 5300
		case strings.HasPrefix(name, "patch-"):
			return 7000 + archiveNumber(name, "patch-")
		case strings.HasPrefix(name, "speech-"):
			return 100
		case strings.Contains(name, "speech-"):
			return 100
		}
		return 4500
	}
	switch {
	case strings.HasPrefix(name, "common"):
		return 1000 + archiveNumber(name, "common")
	case strings.HasPrefix(name, "expansion"):
		return 2000 + archiveNumber(name, "expansion")
	case strings.HasPrefix(name, "lichking"):
		return 3000 + archiveNumber(name, "lichking")
	case strings.HasPrefix(name, "start"):
		return 3500 + archiveNumber(name, "start")
	case name == "patch" || strings.HasPrefix(name, "patch-"):
		return 6000 + archiveNumber(name, "patch")
	}
	return 5500
}

func archiveNumber(name, prefix string) int {
	suffix := strings.TrimPrefix(name, prefix)
	suffix = strings.TrimPrefix(suffix, "-")
	if suffix == "" {
		return 0
	}
	if index := strings.LastIndexByte(suffix, '-'); index >= 0 {
		suffix = suffix[index+1:]
	}
	value, err := strconv.Atoi(suffix)
	if err != nil || value < 0 {
		return 0
	}
	return value
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
	archive.hashes = make([]mpqHashEntry, hashEntries)
	for i := range archive.hashes {
		at := i * 16
		archive.hashes[i] = mpqHashEntry{hashA: binary.LittleEndian.Uint32(hashData[at:]), hashB: binary.LittleEndian.Uint32(hashData[at+4:]), locale: binary.LittleEndian.Uint16(hashData[at+8:]), platform: binary.LittleEndian.Uint16(hashData[at+10:]), block: binary.LittleEndian.Uint32(hashData[at+12:])}
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
	if len(archive.hashes) == 0 {
		return mpqBlockEntry{}, false
	}
	name = normalizeMPQPath(name)
	start := hashString(name, hashTypeName) % uint32(len(archive.hashes))
	for _, wantedLocale := range []uint16{locale, localeNeutral, localeAny} {
		for offset := uint32(0); offset < uint32(len(archive.hashes)); offset++ {
			entry := archive.hashes[(start+offset)%uint32(len(archive.hashes))]
			if entry.block == mpqHashEmpty || entry.block == mpqHashDeleted || entry.block >= uint32(len(archive.blocks)) || entry.hashA != hashString(name, hashTypeA) || entry.hashB != hashString(name, hashTypeB) {
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
	key := hashString(normalizeMPQPath(name), hashTypeFileKey)
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
	sectorSize := 512 << archive.blockShift
	sectorCount := (int(block.fileSize) + sectorSize - 1) / sectorSize
	offsetData, err := readAt(archive.file, archive.headerOffset+int64(block.position), (sectorCount+1)*4)
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
