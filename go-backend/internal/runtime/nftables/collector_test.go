package nftables

import "testing"

func TestParseCounterCommentAcceptsValidToTargetTCP(t *testing.T) {
	sample, ok := ParseCounterComment("flvx forward:42 to-target tcp")
	if !ok {
		t.Fatal("expected comment to parse")
	}
	if sample.ForwardID != 42 ||
		sample.Direction != CounterDirectionToTarget ||
		sample.Protocol != "tcp" {
		t.Fatalf("unexpected sample: %+v", sample)
	}
}

func TestParseCounterCommentRejectsDNAT(t *testing.T) {
	if sample, ok := ParseCounterComment("flvx forward:42 dnat tcp"); ok {
		t.Fatalf("expected dnat comment to be rejected, got %+v", sample)
	}
}

func TestParseCounterSamplesParsesForwardBillableCounters(t *testing.T) {
	raw := []byte(`{
		"nftables": [
			{"metainfo": {"json_schema_version": 1}},
			{"rule": {
				"family": "inet",
				"table": "flvx",
				"chain": "forward",
				"handle": 10,
				"comment": "flvx forward:42 to-target tcp",
				"expr": [
					{"match": {"left": {"payload": {"protocol": "ip", "field": "daddr"}}, "op": "==", "right": "198.51.100.20"}},
					{"counter": {"packets": 7, "bytes": 4096}}
				]
			}},
			{"rule": {
				"family": "inet",
				"table": "flvx",
				"chain": "forward",
				"handle": 11,
				"comment": "flvx forward:42 from-target udp",
				"expr": [
					{"counter": {"packets": 9, "bytes": 8192}}
				]
			}},
			{"rule": {
				"family": "inet",
				"table": "flvx",
				"chain": "prerouting",
				"handle": 12,
				"comment": "flvx forward:42 dnat tcp",
				"expr": [
					{"counter": {"packets": 100, "bytes": 65536}}
				]
			}}
		]
	}`)

	samples, err := ParseCounterSamples(raw)
	if err != nil {
		t.Fatalf("ParseCounterSamples: %v", err)
	}
	if len(samples) != 2 {
		t.Fatalf("expected 2 samples, got %d: %+v", len(samples), samples)
	}

	want := []CounterSample{
		{ForwardID: 42, Direction: CounterDirectionToTarget, Protocol: "tcp", Bytes: 4096, Packets: 7},
		{ForwardID: 42, Direction: CounterDirectionFromTarget, Protocol: "udp", Bytes: 8192, Packets: 9},
	}
	for i := range want {
		if samples[i] != want[i] {
			t.Fatalf("sample %d: expected %+v, got %+v", i, want[i], samples[i])
		}
	}
}

func TestParseCounterSamplesUsesRuleLevelComment(t *testing.T) {
	raw := []byte(`{
		"nftables": [
			{"rule": {
				"table": "flvx",
				"chain": "forward",
				"comment": "flvx forward:77 to-target udp",
				"expr": [
					{"counter": {"packets": 3, "bytes": 2048}}
				]
			}}
		]
	}`)

	samples, err := ParseCounterSamples(raw)
	if err != nil {
		t.Fatalf("ParseCounterSamples: %v", err)
	}
	if len(samples) != 1 {
		t.Fatalf("expected 1 sample, got %d: %+v", len(samples), samples)
	}
	want := CounterSample{
		ForwardID: 77,
		Direction: CounterDirectionToTarget,
		Protocol:  "udp",
		Bytes:     2048,
		Packets:   3,
	}
	if samples[0] != want {
		t.Fatalf("expected %+v, got %+v", want, samples[0])
	}
}

func TestParseCounterSamplesUsesExprLevelComment(t *testing.T) {
	raw := []byte(`{
		"nftables": [
			{"rule": {
				"table": "flvx",
				"chain": "forward",
				"expr": [
					{"counter": {"packets": 4, "bytes": 3072}},
					{"comment": "flvx forward:78 from-target tcp"}
				]
			}}
		]
	}`)

	samples, err := ParseCounterSamples(raw)
	if err != nil {
		t.Fatalf("ParseCounterSamples: %v", err)
	}
	if len(samples) != 1 {
		t.Fatalf("expected 1 sample, got %d: %+v", len(samples), samples)
	}
	want := CounterSample{
		ForwardID: 78,
		Direction: CounterDirectionFromTarget,
		Protocol:  "tcp",
		Bytes:     3072,
		Packets:   4,
	}
	if samples[0] != want {
		t.Fatalf("expected %+v, got %+v", want, samples[0])
	}
}

func TestParseCounterSamplesMalformedJSONReturnsError(t *testing.T) {
	if _, err := ParseCounterSamples([]byte(`{"nftables": [`)); err == nil {
		t.Fatal("expected malformed JSON error")
	}
}

func TestParseCounterSamplesMalformedRuleJSONReturnsError(t *testing.T) {
	raw := []byte(`{
		"nftables": [
			{"rule": {
				"table": "flvx",
				"chain": "forward",
				"comment": "flvx forward:42 to-target tcp",
				"expr": [
					{"counter": {"packets": "bad", "bytes": 4096}}
				]
			}}
		]
	}`)
	if _, err := ParseCounterSamples(raw); err == nil {
		t.Fatal("expected malformed rule JSON error")
	}
}
