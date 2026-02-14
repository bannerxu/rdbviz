package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/hdt3213/rdb/parser"
)

type Meta struct {
	Source       string            `json:"source"`
	GeneratedAt  string            `json:"generated_at"`
	RedisVersion string            `json:"redis_version,omitempty"`
	RedisBits    string            `json:"redis_bits,omitempty"`
	CTime        string            `json:"ctime,omitempty"`
	UsedMem      string            `json:"used_mem,omitempty"`
	AOFBase      string            `json:"aof_base,omitempty"`
	Aux          map[string]string `json:"aux,omitempty"`
}

type Summary struct {
	TotalKeys  int64          `json:"total_keys"`
	TotalSize  int64          `json:"total_size"`
	DBCount    int            `json:"db_count"`
	DBKeys     map[int]int64  `json:"db_keys"`
	WithTTL    int64          `json:"with_ttl"`
	NoTTL      int64          `json:"no_ttl"`
	Expired    int64          `json:"expired"`
	NowISO     string         `json:"now"`
	TypeCounts map[string]int `json:"type_counts"`
}

type TypeStat struct {
	Type  string `json:"type"`
	Count int64  `json:"count"`
	Size  int64  `json:"size"`
}

type Bucket struct {
	Label string `json:"label"`
	Count int64  `json:"count"`
}

type PrefixStat struct {
	Prefix string `json:"prefix"`
	Count  int64  `json:"count"`
	Size   int64  `json:"size"`
}

type PrefixTypeGroup struct {
	Type     string       `json:"type"`
	Prefixes []PrefixStat `json:"prefixes"`
}

type BigKey struct {
	DB          int       `json:"db"`
	Key         string    `json:"key"`
	Type        string    `json:"type"`
	Size        int64     `json:"size"`
	Encoding    string    `json:"encoding"`
	Elements    int64     `json:"elements"`
	Expiration *time.Time `json:"expiration,omitempty"`
}

type Report struct {
	Meta        Meta         `json:"meta"`
	Summary     Summary      `json:"summary"`
	Types       []TypeStat   `json:"types"`
	TTLBuckets  []Bucket     `json:"ttl_buckets"`
	SizeBuckets []Bucket     `json:"size_buckets"`
	Prefixes    []PrefixStat `json:"prefixes"`
	PrefixesByType []PrefixTypeGroup `json:"prefixes_by_type"`
	BigKeys     []BigKey     `json:"bigkeys"`
}

type ttlBucket struct {
	Label string
	Max   time.Duration
}

var ttlBuckets = []ttlBucket{
	{Label: "<=1h", Max: time.Hour},
	{Label: "1h-1d", Max: 24 * time.Hour},
	{Label: "1d-7d", Max: 7 * 24 * time.Hour},
	{Label: "7d-30d", Max: 30 * 24 * time.Hour},
	{Label: "30d-90d", Max: 90 * 24 * time.Hour},
	{Label: ">90d", Max: 36500 * 24 * time.Hour},
}

var sizeBuckets = []struct {
	Label string
	Max   int64
}{
	{Label: "0-1KB", Max: 1 * 1024},
	{Label: "1KB-10KB", Max: 10 * 1024},
	{Label: "10KB-100KB", Max: 100 * 1024},
	{Label: "100KB-1MB", Max: 1 * 1024 * 1024},
	{Label: "1MB-10MB", Max: 10 * 1024 * 1024},
	{Label: "10MB-100MB", Max: 100 * 1024 * 1024},
	{Label: ">100MB", Max: 1<<63 - 1},
}

type prefixAgg struct {
	Count int64
	Size  int64
}

type bigKeyHeap []BigKey

func (h bigKeyHeap) Len() int           { return len(h) }
func (h bigKeyHeap) Less(i, j int) bool { return h[i].Size < h[j].Size }
func (h bigKeyHeap) Swap(i, j int)      { h[i], h[j] = h[j], h[i] }

func (h *bigKeyHeap) Push(x interface{}) {
	*h = append(*h, x.(BigKey))
}

func (h *bigKeyHeap) Pop() interface{} {
	old := *h
	n := len(old)
	x := old[n-1]
	*h = old[:n-1]
	return x
}

func main() {
	rdbPath := flag.String("rdb", "", "path to dump.rdb")
	outPath := flag.String("out", "", "output report.json")
	sep := flag.String("prefix-sep", ":", "prefix separator")
	maxDepth := flag.Int("prefix-depth", 3, "max prefix depth")
	topN := flag.Int("topn", 50, "top N for prefixes and bigkeys")
	progressEvery := flag.Duration("progress", 5*time.Second, "progress interval (0 to disable)")
	flag.Parse()

	if *rdbPath == "" || *outPath == "" {
		fmt.Println("usage: rdbviz-tool -rdb dump.rdb -out report.json [-prefix-sep :] [-prefix-depth 3] [-topn 50]")
		os.Exit(2)
	}

	rdbAbs, _ := filepath.Abs(*rdbPath)
	now := time.Now()

	meta := Meta{
		Source:      rdbAbs,
		GeneratedAt: now.Format(time.RFC3339),
		Aux:         map[string]string{},
	}

	summary := Summary{
		DBKeys:     map[int]int64{},
		TypeCounts: map[string]int{},
		NowISO:     now.Format(time.RFC3339),
	}

	typeCount := map[string]int64{}
	typeSize := map[string]int64{}
	prefixes := map[string]prefixAgg{}
	prefixesByType := map[string]map[string]prefixAgg{}
	bigKeys := make(bigKeyHeap, 0, *topN)

	ttlCounts := map[string]int64{
		"no-expire": 0,
		"expired":   0,
	}
	for _, b := range ttlBuckets {
		ttlCounts[b.Label] = 0
	}

	sizeCounts := map[string]int64{}
	for _, b := range sizeBuckets {
		sizeCounts[b.Label] = 0
	}

	expireCount := int64(0)
	noExpireCount := int64(0)
	expiredCount := int64(0)

	rdbFile, err := os.Open(rdbAbs)
	if err != nil {
		fmt.Fprintf(os.Stderr, "open rdb error: %v\n", err)
		os.Exit(1)
	}
	defer rdbFile.Close()
	stat, err := rdbFile.Stat()
	if err != nil {
		fmt.Fprintf(os.Stderr, "stat rdb error: %v\n", err)
		os.Exit(1)
	}
	fileSize := stat.Size()

	dec := parser.NewDecoder(rdbFile).WithSpecialOpCode()
	lastPrint := time.Now()
	err = dec.Parse(func(o parser.RedisObject) bool {
		switch obj := o.(type) {
		case *parser.AuxObject:
			key := strings.TrimSpace(obj.Key)
			val := strings.TrimSpace(obj.Value)
			meta.Aux[key] = val
			switch key {
			case "redis-ver":
				meta.RedisVersion = val
			case "redis-bits":
				meta.RedisBits = val
			case "ctime":
				meta.CTime = val
			case "used-mem":
				meta.UsedMem = val
			case "aof-base":
				meta.AOFBase = val
			}
			return true
		case *parser.DBSizeObject:
			return true
		}

		key := o.GetKey()
		db := o.GetDBIndex()
		objType := o.GetType()
		encoding := o.GetEncoding()
		expiration := o.GetExpiration()
		if key == "" {
			return true
		}

		size := getSize(o)
		summary.TotalKeys++
		summary.TotalSize += size
		summary.DBKeys[db]++
		sizeCounts[getSizeBucket(size)]++

		typeCount[objType]++
		typeSize[objType] += size
		summary.TypeCounts[objType]++

		if expiration == nil {
			noExpireCount++
			ttlCounts["no-expire"]++
		} else {
			expireCount++
			if expiration.Before(now) {
				expiredCount++
				ttlCounts["expired"]++
			} else {
				ttl := expiration.Sub(now)
				placed := false
				for _, b := range ttlBuckets {
					if ttl <= b.Max {
						ttlCounts[b.Label]++
						placed = true
						break
					}
				}
				if !placed {
					ttlCounts[">90d"]++
				}
			}
		}

		applyPrefixes(prefixes, key, size, *sep, *maxDepth)
		applyPrefixesByType(prefixesByType, objType, key, size, *sep, *maxDepth)

		bk := BigKey{
			DB:          db,
			Key:         key,
			Type:        objType,
			Size:        size,
			Encoding:    encoding,
			Elements:    getElementCount(o),
			Expiration: expiration,
		}
		pushBigKey(&bigKeys, bk, *topN)

		if *progressEvery > 0 && time.Since(lastPrint) >= *progressEvery {
			read := int64(dec.GetReadCount())
			percent := float64(0)
			if fileSize > 0 {
				percent = float64(read) / float64(fileSize) * 100
			}
			fmt.Fprintf(os.Stderr, "[progress] keys=%d read=%s/%s (%.1f%%)\n",
				summary.TotalKeys, formatBytes(read), formatBytes(fileSize), percent)
			lastPrint = time.Now()
		}
		return true
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "parse error: %v\n", err)
		os.Exit(1)
	}

	summary.WithTTL = expireCount
	summary.NoTTL = noExpireCount
	summary.Expired = expiredCount
	summary.DBCount = len(summary.DBKeys)

	types := make([]TypeStat, 0, len(typeCount))
	for t, c := range typeCount {
		types = append(types, TypeStat{Type: t, Count: c, Size: typeSize[t]})
	}
	sort.Slice(types, func(i, j int) bool { return types[i].Size > types[j].Size })

	ttlList := make([]Bucket, 0, len(ttlCounts))
	order := []string{"no-expire", "expired"}
	for _, b := range ttlBuckets {
		order = append(order, b.Label)
	}
	for _, label := range order {
		if v, ok := ttlCounts[label]; ok {
			ttlList = append(ttlList, Bucket{Label: label, Count: v})
		}
	}

	prefixList := make([]PrefixStat, 0, len(prefixes))
	for p, a := range prefixes {
		prefixList = append(prefixList, PrefixStat{Prefix: p, Count: a.Count, Size: a.Size})
	}
	sort.Slice(prefixList, func(i, j int) bool { return prefixList[i].Size > prefixList[j].Size })
	if *topN > 0 && len(prefixList) > *topN {
		prefixList = prefixList[:*topN]
	}

	byType := make([]PrefixTypeGroup, 0, len(prefixesByType))
	for t, pm := range prefixesByType {
		items := make([]PrefixStat, 0, len(pm))
		for p, a := range pm {
			items = append(items, PrefixStat{Prefix: p, Count: a.Count, Size: a.Size})
		}
		sort.Slice(items, func(i, j int) bool { return items[i].Size > items[j].Size })
		if *topN > 0 && len(items) > *topN {
			items = items[:*topN]
		}
		byType = append(byType, PrefixTypeGroup{Type: t, Prefixes: items})
	}
	sort.Slice(byType, func(i, j int) bool { return byType[i].Type < byType[j].Type })

	sort.Slice(bigKeys, func(i, j int) bool { return bigKeys[i].Size > bigKeys[j].Size })

	sizeList := make([]Bucket, 0, len(sizeBuckets))
	for _, b := range sizeBuckets {
		sizeList = append(sizeList, Bucket{Label: b.Label, Count: sizeCounts[b.Label]})
	}

	report := Report{
		Meta:        meta,
		Summary:     summary,
		Types:       types,
		TTLBuckets:  ttlList,
		SizeBuckets: sizeList,
		Prefixes:    prefixList,
		PrefixesByType: byType,
		BigKeys:     bigKeys,
	}

	if err := os.MkdirAll(filepath.Dir(*outPath), 0o755); err != nil {
		fmt.Fprintf(os.Stderr, "mkdir error: %v\n", err)
		os.Exit(1)
	}

	f, err := os.Create(*outPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "create error: %v\n", err)
		os.Exit(1)
	}
	defer f.Close()

	enc := json.NewEncoder(f)
	enc.SetIndent("", "  ")
	if err := enc.Encode(report); err != nil {
		fmt.Fprintf(os.Stderr, "write error: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("report written: %s\n", *outPath)
}

func getSize(o parser.RedisObject) int64 {
	return int64(o.GetSize())
}

func getSizeBucket(size int64) string {
	for _, b := range sizeBuckets {
		if size <= b.Max {
			return b.Label
		}
	}
	return ">100MB"
}

func formatBytes(bytes int64) string {
	if bytes < 0 {
		return "0 B"
	}
	units := []string{"B", "KB", "MB", "GB", "TB"}
	v := float64(bytes)
	i := 0
	for v >= 1024 && i < len(units)-1 {
		v /= 1024
		i++
	}
	if v < 10 && i > 0 {
		return fmt.Sprintf("%.2f %s", v, units[i])
	}
	return fmt.Sprintf("%.1f %s", v, units[i])
}

func getElementCount(o parser.RedisObject) int64 {
	return int64(o.GetElemCount())
}

func applyPrefixes(agg map[string]prefixAgg, key string, size int64, sep string, maxDepth int) {
	if sep == "" || maxDepth <= 0 {
		return
	}
	parts := strings.Split(key, sep)
	if len(parts) == 0 {
		return
	}
	if len(parts) < maxDepth {
		maxDepth = len(parts)
	}
	for i := 1; i <= maxDepth; i++ {
		p := strings.Join(parts[:i], sep)
		if i < len(parts) {
			p = p + sep
		}
		a := agg[p]
		a.Count++
		a.Size += size
		agg[p] = a
	}
}

func applyPrefixesByType(agg map[string]map[string]prefixAgg, objType, key string, size int64, sep string, maxDepth int) {
	if sep == "" || maxDepth <= 0 {
		return
	}
	m, ok := agg[objType]
	if !ok {
		m = map[string]prefixAgg{}
		agg[objType] = m
	}
	applyPrefixes(m, key, size, sep, maxDepth)
}
func pushBigKey(h *bigKeyHeap, bk BigKey, topN int) {
	if topN <= 0 {
		return
	}
	if len(*h) < topN {
		*h = append(*h, bk)
		return
	}
	minIdx := 0
	for i := 1; i < len(*h); i++ {
		if (*h)[i].Size < (*h)[minIdx].Size {
			minIdx = i
		}
	}
	if bk.Size > (*h)[minIdx].Size {
		(*h)[minIdx] = bk
	}
}
