package handler

import (
	"log"
	"strings"
	"time"

	"go-backend/internal/store/model"
	"go-backend/internal/store/repo"
)

type tunnelTrafficDelta struct {
	bytesIn  int64
	bytesOut int64
}

func unixMilliBucketMinute(nowMs int64) int64 {
	if nowMs <= 0 {
		return 0
	}
	const minuteMs = int64(time.Minute / time.Millisecond)
	return nowMs - (nowMs % minuteMs)
}

func collectFlowUploadForwardIDs(items []flowItem) []int64 {
	ids := make([]int64, 0, len(items))
	seen := make(map[int64]struct{}, len(items))
	for _, item := range items {
		forwardID, _, _, ok := parseFlowServiceIDs(strings.TrimSpace(item.N))
		if !ok || forwardID <= 0 {
			continue
		}
		if _, exists := seen[forwardID]; exists {
			continue
		}
		seen[forwardID] = struct{}{}
		ids = append(ids, forwardID)
	}
	return ids
}

func (h *Handler) recordTunnelMetricsFromForwardBatch(nodeID int64, forwardDeltas map[int64]tunnelTrafficDelta, metas map[int64]repo.FlowUploadForwardMeta, nowMs int64) {
	if h == nil || h.repo == nil || nodeID <= 0 || len(forwardDeltas) == 0 {
		return
	}
	bucketTs := unixMilliBucketMinute(nowMs)
	if bucketTs <= 0 {
		return
	}

	tunnelAgg := make(map[int64]tunnelTrafficDelta)
	for forwardID, delta := range forwardDeltas {
		meta, ok := metas[forwardID]
		if !ok || meta.TunnelID <= 0 {
			continue
		}
		current := tunnelAgg[meta.TunnelID]
		current.bytesIn += delta.bytesIn
		current.bytesOut += delta.bytesOut
		tunnelAgg[meta.TunnelID] = current
	}

	metrics := make([]*model.TunnelMetric, 0, len(tunnelAgg))
	for tunnelID, delta := range tunnelAgg {
		if delta.bytesIn == 0 && delta.bytesOut == 0 {
			continue
		}
		metrics = append(metrics, &model.TunnelMetric{
			TunnelID:  tunnelID,
			NodeID:    nodeID,
			Timestamp: bucketTs,
			BytesIn:   delta.bytesIn,
			BytesOut:  delta.bytesOut,
		})
	}
	if len(metrics) == 0 {
		return
	}

	if err := h.repo.UpsertTunnelMetricBuckets(metrics); err != nil {
		log.Printf("monitoring write failed op=tunnel_metric.upsert_buckets node_id=%d bucket_ts=%d count=%d err=%v", nodeID, bucketTs, len(metrics), err)
		return
	}
	log.Printf("monitoring ok op=tunnel_metric.upsert_buckets node_id=%d bucket_ts=%d count=%d", nodeID, bucketTs, len(metrics))
}
