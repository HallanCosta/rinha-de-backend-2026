package search

import (
	"container/heap"
	"context"
	"encoding/json"
	"os"
	"sort"
	"strconv"
	"testing"

	"rinha-backend-2026/internal/dataset"
	"rinha-backend-2026/internal/model"
	"rinha-backend-2026/internal/vectorizer"
)

type officialBenchmarkData struct {
	Entries []officialBenchmarkEntry `json:"entries"`
}

type officialBenchmarkEntry struct {
	Request          model.FraudRequest `json:"request"`
	ExpectedApproved bool               `json:"expected_approved"`
}

const officialNeighborsCount = 5

// BenchmarkKDTreeOfficialDataset mede a busca usando os 50 mil payloads
// rotulados da suíte oficial, em vez de repetir uma única transação sintética.
// Isso revela consultas que percorrem mais nós e representam melhor a fila
// observada no teste k6.
func BenchmarkKDTreeOfficialDataset(b *testing.B) {
	path := os.Getenv("OFFICIAL_TEST_DATA")
	if path == "" {
		b.Skip("OFFICIAL_TEST_DATA is not set")
	}

	data, err := loadOfficialBenchmarkData(path)
	if err != nil {
		b.Fatalf("load official test data: %v", err)
	}
	if len(data.Entries) == 0 {
		b.Fatal("official test data has no entries")
	}
	entries := sampleOfficialEntries(data.Entries)

	metadata, err := dataset.LoadMetadata("../../resources/mcc_risk.json", "../../resources/normalization.json")
	if err != nil {
		b.Fatalf("load metadata: %v", err)
	}
	store, err := dataset.LoadReferences(context.Background(), "../../resources/references.json.gz")
	if err != nil {
		b.Fatalf("load references: %v", err)
	}
	tree, err := NewKDTree(store)
	if err != nil {
		b.Fatalf("build kd-tree: %v", err)
	}
	vectorizerEngine := vectorizer.New(metadata)

	queries := make([]model.Vector, len(entries))
	for index, entry := range entries {
		queries[index], err = vectorizerEngine.Vectorize(entry.Request)
		if err != nil {
			b.Fatalf("vectorize official entry %d: %v", index, err)
		}
	}

	b.ReportMetric(float64(len(queries)), "official_queries")
	b.ReportAllocs()
	b.ResetTimer()

	totalVisited := 0
	maxVisited := 0
	for iteration := 0; iteration < b.N; iteration++ {
		for _, query := range queries {
			queryPacked := dataset.EncodeVector(query)
			candidates := make(candidateHeap, 0, officialNeighborsCount)
			heap.Init(&candidates)
			visited := 0
			if err := tree.visit(context.Background(), tree.root, queryPacked, officialNeighborsCount, &candidates, &visited); err != nil {
				b.Fatalf("search official query: %v", err)
			}
			if _, err := finishCandidates(context.Background(), candidates); err != nil {
				b.Fatalf("finish official query: %v", err)
			}
			totalVisited += visited
			if visited > maxVisited {
				maxVisited = visited
			}
		}
	}

	b.ReportMetric(float64(totalVisited)/float64(b.N*len(queries)), "visited/query")
	b.ReportMetric(float64(maxVisited), "max_visited")
}

func BenchmarkLSHOfficialDataset(b *testing.B) {
	path := os.Getenv("OFFICIAL_TEST_DATA")
	if path == "" {
		b.Skip("OFFICIAL_TEST_DATA is not set")
	}

	data, err := loadOfficialBenchmarkData(path)
	if err != nil {
		b.Fatalf("load official test data: %v", err)
	}
	entries := sampleOfficialEntries(data.Entries)
	metadata, err := dataset.LoadMetadata("../../resources/mcc_risk.json", "../../resources/normalization.json")
	if err != nil {
		b.Fatalf("load metadata: %v", err)
	}
	store, err := dataset.LoadReferences(context.Background(), "../../resources/references.json.gz")
	if err != nil {
		b.Fatalf("load references: %v", err)
	}
	index, err := NewLSH(store)
	if err != nil {
		b.Fatalf("build LSH: %v", err)
	}
	vectorizerEngine := vectorizer.New(metadata)
	queries := make([]model.Vector, len(entries))
	for entryIndex, entry := range entries {
		queries[entryIndex], err = vectorizerEngine.Vectorize(entry.Request)
		if err != nil {
			b.Fatalf("vectorize official entry %d: %v", entryIndex, err)
		}
	}

	b.ReportMetric(float64(len(queries)), "official_queries")
	b.ReportAllocs()
	b.ResetTimer()
	for iteration := 0; iteration < b.N; iteration++ {
		for _, query := range queries {
			if _, err := index.TopK(context.Background(), query, officialNeighborsCount); err != nil {
				b.Fatalf("search official query: %v", err)
			}
		}
	}
}

func sampleOfficialEntries(entries []officialBenchmarkEntry) []officialBenchmarkEntry {
	limit, err := strconv.Atoi(os.Getenv("OFFICIAL_QUERY_LIMIT"))
	if err != nil || limit <= 0 || limit >= len(entries) {
		return entries
	}

	sample := make([]officialBenchmarkEntry, limit)
	for index := range sample {
		position := index * len(entries) / limit
		sample[index] = entries[position]
	}
	return sample
}

func TestKDTreeOfficialVisitLimitQuality(t *testing.T) {
	dataPath := os.Getenv("OFFICIAL_TEST_DATA")
	budget, err := strconv.Atoi(os.Getenv("OFFICIAL_VISIT_LIMIT"))
	if dataPath == "" || err != nil || budget <= 0 {
		t.Skip("OFFICIAL_TEST_DATA and positive OFFICIAL_VISIT_LIMIT are required")
	}

	data, err := loadOfficialBenchmarkData(dataPath)
	if err != nil {
		t.Fatalf("load official test data: %v", err)
	}
	entries := sampleOfficialEntries(data.Entries)
	metadata, err := dataset.LoadMetadata("../../resources/mcc_risk.json", "../../resources/normalization.json")
	if err != nil {
		t.Fatalf("load metadata: %v", err)
	}
	store, err := dataset.LoadReferences(context.Background(), "../../resources/references.json.gz")
	if err != nil {
		t.Fatalf("load references: %v", err)
	}
	tree, err := NewKDTreeWithVisitLimit(store, budget)
	if err != nil {
		t.Fatalf("build limited kd-tree: %v", err)
	}
	vectorizerEngine := vectorizer.New(metadata)

	var mismatches, errors int
	for index, entry := range entries {
		query, err := vectorizerEngine.Vectorize(entry.Request)
		if err != nil {
			t.Fatalf("vectorize official entry %d: %v", index, err)
		}
		neighbors, err := tree.TopK(context.Background(), query, officialNeighborsCount)
		if err != nil || len(neighbors) != officialNeighborsCount {
			errors++
			continue
		}

		fraudCount := 0
		for _, neighbor := range neighbors {
			if neighbor.IsFraud() {
				fraudCount++
			}
		}
		approved := float64(fraudCount)/float64(officialNeighborsCount) < 0.6
		if approved != entry.ExpectedApproved {
			mismatches++
		}
	}

	total := len(entries)
	t.Logf("budget=%d entries=%d mismatches=%d (%.2f%%) errors=%d (%.2f%%)",
		budget, total, mismatches, percentage(mismatches, total), errors, percentage(errors, total))
}

func TestLSHOfficialQuality(t *testing.T) {
	dataPath := os.Getenv("OFFICIAL_TEST_DATA")
	radius, err := strconv.Atoi(os.Getenv("OFFICIAL_LSH_RADIUS"))
	if dataPath == "" || err != nil || radius < 0 {
		t.Skip("OFFICIAL_TEST_DATA and non-negative OFFICIAL_LSH_RADIUS are required")
	}

	data, err := loadOfficialBenchmarkData(dataPath)
	if err != nil {
		t.Fatalf("load official test data: %v", err)
	}
	entries := sampleOfficialEntries(data.Entries)
	metadata, err := dataset.LoadMetadata("../../resources/mcc_risk.json", "../../resources/normalization.json")
	if err != nil {
		t.Fatalf("load metadata: %v", err)
	}
	store, err := dataset.LoadReferences(context.Background(), "../../resources/references.json.gz")
	if err != nil {
		t.Fatalf("load references: %v", err)
	}
	lshIndex, err := NewLSHWithProbeRadius(store, radius)
	if err != nil {
		t.Fatalf("build LSH: %v", err)
	}
	vectorizerEngine := vectorizer.New(metadata)

	var mismatches, errors, totalCandidates int
	candidateCounts := make([]int, 0, len(entries))
	for entryIndex, entry := range entries {
		query, err := vectorizerEngine.Vectorize(entry.Request)
		if err != nil {
			t.Fatalf("vectorize official entry %d: %v", entryIndex, err)
		}
		neighbors, err := lshIndex.TopK(context.Background(), query, officialNeighborsCount)
		if err != nil || len(neighbors) != officialNeighborsCount {
			errors++
			continue
		}
		queryPacked := dataset.EncodeVector(query)
		signature := lshSignature(queryPacked, &lshIndex.planes)
		projection := lshProjection(queryPacked)
		for _, mask := range lshIndex.probes {
			start, end := lshIndex.rangeForQuery(signature^mask, projection)
			totalCandidates += end - start
			candidateCounts = append(candidateCounts, end-start)
		}

		fraudCount := 0
		for _, neighbor := range neighbors {
			if neighbor.IsFraud() {
				fraudCount++
			}
		}
		approved := float64(fraudCount)/float64(officialNeighborsCount) < 0.6
		if approved != entry.ExpectedApproved {
			mismatches++
		}
	}

	total := len(entries)
	t.Logf("LSH radius=%d entries=%d mismatches=%d (%.2f%%) errors=%d (%.2f%%)",
		radius, total, mismatches, percentage(mismatches, total), errors, percentage(errors, total))
	t.Logf("LSH average candidates/query=%.0f", float64(totalCandidates)/float64(total))
	if len(candidateCounts) > 0 {
		sort.Ints(candidateCounts)
		p95 := candidateCounts[(len(candidateCounts)*95)/100]
		t.Logf("LSH candidates p95=%d max=%d", p95, candidateCounts[len(candidateCounts)-1])
	}
}

func percentage(value, total int) float64 {
	if total == 0 {
		return 0
	}
	return float64(value) * 100 / float64(total)
}

func loadOfficialBenchmarkData(path string) (officialBenchmarkData, error) {
	contents, err := os.ReadFile(path)
	if err != nil {
		return officialBenchmarkData{}, err
	}

	var data officialBenchmarkData
	if err := json.Unmarshal(contents, &data); err != nil {
		return officialBenchmarkData{}, err
	}
	return data, nil
}
