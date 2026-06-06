package repo

import (
	"math"
	"testing"
)

func TestNftCounterStateUpsertUpdatesExistingKey(t *testing.T) {
	r, err := Open(":memory:")
	if err != nil {
		t.Fatalf("open repo: %v", err)
	}
	defer r.Close()

	first := []NftCounterStateInput{
		{
			NodeID:        11,
			ForwardID:     42,
			Protocol:      "tcp",
			Direction:     "to-target",
			RuleHash:      "hash-a",
			Bytes:         100,
			Packets:       10,
			CollectedTime: 1000,
		},
		{
			NodeID:    0,
			ForwardID: 42,
			Protocol:  "tcp",
			Direction: "to-target",
			Bytes:     999,
		},
	}
	if err := r.UpsertNftCounterStates(first, 2000); err != nil {
		t.Fatalf("first UpsertNftCounterStates: %v", err)
	}

	second := []NftCounterStateInput{
		{
			NodeID:        11,
			ForwardID:     42,
			Protocol:      "tcp",
			Direction:     "to-target",
			RuleHash:      "hash-b",
			Bytes:         250,
			Packets:       25,
			CollectedTime: 3000,
		},
	}
	if err := r.UpsertNftCounterStates(second, 4000); err != nil {
		t.Fatalf("second UpsertNftCounterStates: %v", err)
	}

	rows, err := r.GetNftCounterStatesByNode(11)
	if err != nil {
		t.Fatalf("GetNftCounterStatesByNode: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("expected one counter state row, got %d: %+v", len(rows), rows)
	}
	got := rows[0]
	if got.ForwardID != 42 || got.Protocol != "tcp" || got.Direction != "to-target" {
		t.Fatalf("unexpected counter state key: %+v", got)
	}
	if got.RuleHash != "hash-b" || got.Bytes != 250 || got.Packets != 25 || got.CollectedTime != 3000 {
		t.Fatalf("counter state was not updated: %+v", got)
	}
	if got.CreatedTime != 2000 || got.UpdatedTime != 4000 {
		t.Fatalf("unexpected timestamps after upsert: %+v", got)
	}
}

func TestNftCounterStateDeleteByForwardRemovesOnlyMatchingRows(t *testing.T) {
	r, err := Open(":memory:")
	if err != nil {
		t.Fatalf("open repo: %v", err)
	}
	defer r.Close()

	inputs := []NftCounterStateInput{
		{NodeID: 11, ForwardID: 42, Protocol: "tcp", Direction: "to-target", RuleHash: "a", Bytes: 100, Packets: 10, CollectedTime: 1000},
		{NodeID: 11, ForwardID: 43, Protocol: "udp", Direction: "from-target", RuleHash: "b", Bytes: 200, Packets: 20, CollectedTime: 1000},
		{NodeID: 12, ForwardID: 42, Protocol: "tcp", Direction: "to-target", RuleHash: "c", Bytes: 300, Packets: 30, CollectedTime: 1000},
	}
	if err := r.UpsertNftCounterStates(inputs, 2000); err != nil {
		t.Fatalf("UpsertNftCounterStates: %v", err)
	}
	if err := r.DeleteNftCounterStatesByForward(42); err != nil {
		t.Fatalf("DeleteNftCounterStatesByForward: %v", err)
	}

	node11, err := r.GetNftCounterStatesByNode(11)
	if err != nil {
		t.Fatalf("GetNftCounterStatesByNode(11): %v", err)
	}
	if len(node11) != 1 || node11[0].ForwardID != 43 {
		t.Fatalf("expected only forward 43 for node 11, got %+v", node11)
	}
	node12, err := r.GetNftCounterStatesByNode(12)
	if err != nil {
		t.Fatalf("GetNftCounterStatesByNode(12): %v", err)
	}
	if len(node12) != 0 {
		t.Fatalf("expected forward 42 state removed from node 12, got %+v", node12)
	}
}

func TestNftCounterStateUpsertSkipsInvalidProtocolAndDirection(t *testing.T) {
	r, err := Open(":memory:")
	if err != nil {
		t.Fatalf("open repo: %v", err)
	}
	defer r.Close()

	inputs := []NftCounterStateInput{
		{NodeID: 11, ForwardID: 42, Protocol: "icmp", Direction: "to-target", RuleHash: "bad-protocol", Bytes: 100, Packets: 10, CollectedTime: 1000},
		{NodeID: 11, ForwardID: 43, Protocol: "tcp", Direction: "sideways", RuleHash: "bad-direction", Bytes: 200, Packets: 20, CollectedTime: 1000},
		{NodeID: 11, ForwardID: 44, Protocol: " UDP ", Direction: " FROM-TARGET ", RuleHash: "valid", Bytes: 300, Packets: 30, CollectedTime: 1000},
	}
	if err := r.UpsertNftCounterStates(inputs, 2000); err != nil {
		t.Fatalf("UpsertNftCounterStates: %v", err)
	}

	rows, err := r.GetNftCounterStatesByNode(11)
	if err != nil {
		t.Fatalf("GetNftCounterStatesByNode: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("expected only the valid counter state row, got %d: %+v", len(rows), rows)
	}
	if rows[0].ForwardID != 44 || rows[0].Protocol != "udp" || rows[0].Direction != "from-target" {
		t.Fatalf("unexpected valid counter state row: %+v", rows[0])
	}
}

func TestNftCounterStateUpsertSkipsCountersAboveInt64(t *testing.T) {
	r, err := Open(":memory:")
	if err != nil {
		t.Fatalf("open repo: %v", err)
	}
	defer r.Close()

	inputs := []NftCounterStateInput{
		{NodeID: 11, ForwardID: 42, Protocol: "tcp", Direction: "to-target", RuleHash: "too-large", Bytes: uint64(math.MaxInt64) + 1, Packets: 10, CollectedTime: 1000},
		{NodeID: 11, ForwardID: 43, Protocol: "udp", Direction: "from-target", RuleHash: "valid", Bytes: 300, Packets: 30, CollectedTime: 1000},
	}
	if err := r.UpsertNftCounterStates(inputs, 2000); err != nil {
		t.Fatalf("UpsertNftCounterStates: %v", err)
	}

	rows, err := r.GetNftCounterStatesByNode(11)
	if err != nil {
		t.Fatalf("GetNftCounterStatesByNode: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("expected only the valid counter state row, got %d: %+v", len(rows), rows)
	}
	if rows[0].ForwardID != 43 || rows[0].Bytes != 300 || rows[0].Packets != 30 {
		t.Fatalf("unexpected valid counter state row: %+v", rows[0])
	}
}
