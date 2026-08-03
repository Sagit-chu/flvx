package repo

import (
	"sync"
	"testing"
	"time"

	"go-backend/internal/store/model"
)

func TestGetTunnelMetricsAggregatedSumsAcrossNodes(t *testing.T) {
	r, err := Open(":memory:")
	if err != nil {
		t.Fatalf("open repo: %v", err)
	}
	defer r.Close()

	ts := time.Now().UnixMilli()

	if err := r.InsertTunnelMetric(&model.TunnelMetric{
		TunnelID:  1,
		NodeID:    1,
		Timestamp: ts,
		BytesIn:   100,
		BytesOut:  200,
	}); err != nil {
		t.Fatalf("insert tunnel metric n1: %v", err)
	}
	if err := r.InsertTunnelMetric(&model.TunnelMetric{
		TunnelID:  1,
		NodeID:    2,
		Timestamp: ts,
		BytesIn:   300,
		BytesOut:  400,
	}); err != nil {
		t.Fatalf("insert tunnel metric n2: %v", err)
	}

	metrics, err := r.GetTunnelMetricsAggregated(1, ts-1000, ts+1000)
	if err != nil {
		t.Fatalf("get aggregated tunnel metrics: %v", err)
	}
	if len(metrics) != 1 {
		t.Fatalf("expected 1 aggregated point, got %d", len(metrics))
	}
	if metrics[0].Timestamp != ts {
		t.Fatalf("expected timestamp %d, got %d", ts, metrics[0].Timestamp)
	}
	if metrics[0].BytesIn != 400 {
		t.Fatalf("expected bytesIn 400, got %d", metrics[0].BytesIn)
	}
	if metrics[0].BytesOut != 600 {
		t.Fatalf("expected bytesOut 600, got %d", metrics[0].BytesOut)
	}
}

func TestUpsertTunnelMetricBucketsAggregatesDuplicateKeysInBatch(t *testing.T) {
	r, err := Open(":memory:")
	if err != nil {
		t.Fatalf("open repo: %v", err)
	}
	defer r.Close()

	ts := time.Now().UnixMilli()

	items := []*model.TunnelMetric{
		{TunnelID: 1, NodeID: 1, Timestamp: ts, BytesIn: 10, BytesOut: 20},
		{TunnelID: 1, NodeID: 1, Timestamp: ts, BytesIn: 30, BytesOut: 40},
	}
	if err := r.UpsertTunnelMetricBuckets(items); err != nil {
		t.Fatalf("upsert buckets: %v", err)
	}

	rows, err := r.GetTunnelMetrics(1, ts-1000, ts+1000)
	if err != nil {
		t.Fatalf("get tunnel metrics: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("expected 1 stored row, got %d", len(rows))
	}
	if rows[0].BytesIn != 40 {
		t.Fatalf("expected bytesIn 40, got %d", rows[0].BytesIn)
	}
	if rows[0].BytesOut != 60 {
		t.Fatalf("expected bytesOut 60, got %d", rows[0].BytesOut)
	}
}

func TestUpsertTunnelMetricBucketsIsSafeUnderConcurrency(t *testing.T) {
	r, err := Open(":memory:")
	if err != nil {
		t.Fatalf("open repo: %v", err)
	}
	defer r.Close()

	ts := time.Now().UnixMilli()

	const workers = 20
	const perWorkerIn = int64(5)
	const perWorkerOut = int64(7)

	var wg sync.WaitGroup
	wg.Add(workers)
	for i := 0; i < workers; i++ {
		go func() {
			defer wg.Done()
			_ = r.UpsertTunnelMetricBuckets([]*model.TunnelMetric{{
				TunnelID:  1,
				NodeID:    1,
				Timestamp: ts,
				BytesIn:   perWorkerIn,
				BytesOut:  perWorkerOut,
			}})
		}()
	}
	wg.Wait()

	rows, err := r.GetTunnelMetrics(1, ts-1000, ts+1000)
	if err != nil {
		t.Fatalf("get tunnel metrics: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("expected 1 stored row, got %d", len(rows))
	}

	wantIn := int64(workers) * perWorkerIn
	wantOut := int64(workers) * perWorkerOut
	if rows[0].BytesIn != wantIn {
		t.Fatalf("expected bytesIn %d, got %d", wantIn, rows[0].BytesIn)
	}
	if rows[0].BytesOut != wantOut {
		t.Fatalf("expected bytesOut %d, got %d", wantOut, rows[0].BytesOut)
	}
}

func TestGetLatestTunnelQualitiesIncludesChainDetails(t *testing.T) {
	r, err := Open(":memory:")
	if err != nil {
		t.Fatalf("open repo: %v", err)
	}
	defer r.Close()

	if err := r.InsertTunnelQuality(&model.TunnelQuality{
		TunnelID:     7,
		Timestamp:    time.Now().UnixMilli(),
		Success:      1,
		ChainDetails: `{"primaryPath":[],"candidateHops":[{"fromNodeId":10,"toNodeId":31}]}`,
	}); err != nil {
		t.Fatalf("insert tunnel quality: %v", err)
	}

	items, err := r.GetLatestTunnelQualities()
	if err != nil {
		t.Fatalf("get latest tunnel qualities: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("expected one latest tunnel quality, got %+v", items)
	}
	if items[0].ChainDetails == "" {
		t.Fatalf("expected chain details in latest quality row, got %+v", items[0])
	}
}
