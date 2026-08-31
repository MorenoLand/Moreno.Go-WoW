package ui

type AssetStats struct {
	Archives     int
	CachedFiles  int
	MissingFiles int
}

func (l *Loader) AssetStats() AssetStats {
	if l == nil || l.mpq == nil {
		return AssetStats{}
	}
	return AssetStats{Archives: len(l.mpq.archives), CachedFiles: len(l.mpq.files), MissingFiles: len(l.mpq.missing)}
}
