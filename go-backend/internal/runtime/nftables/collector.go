package nftables

import (
	"encoding/json"
	"strconv"
	"strings"
)

type CounterSample struct {
	ForwardID int64
	Direction string
	Protocol  string
	Bytes     uint64
	Packets   uint64
}

func ParseCounterComment(comment string) (CounterSample, bool) {
	parts := strings.Split(comment, " ")
	if len(parts) != 4 || parts[0] != "flvx" {
		return CounterSample{}, false
	}
	if !strings.HasPrefix(parts[1], "forward:") {
		return CounterSample{}, false
	}
	forwardText := strings.TrimPrefix(parts[1], "forward:")
	forwardID, err := strconv.ParseInt(forwardText, 10, 64)
	if err != nil || forwardID <= 0 {
		return CounterSample{}, false
	}
	direction := parts[2]
	if direction != CounterDirectionToTarget && direction != CounterDirectionFromTarget {
		return CounterSample{}, false
	}
	protocol := parts[3]
	if protocol != "tcp" && protocol != "udp" {
		return CounterSample{}, false
	}
	return CounterSample{
		ForwardID: forwardID,
		Direction: direction,
		Protocol:  protocol,
	}, true
}

func ParseCounterSamples(raw []byte) ([]CounterSample, error) {
	var doc nftListTable
	if err := json.Unmarshal(raw, &doc); err != nil {
		return nil, err
	}

	samples := make([]CounterSample, 0)
	for _, item := range doc.Nftables {
		ruleRaw, ok := item["rule"]
		if !ok {
			continue
		}
		var rule nftCounterRule
		if err := json.Unmarshal(ruleRaw, &rule); err != nil {
			return nil, err
		}
		if rule.Table != "flvx" || rule.Chain != "forward" {
			continue
		}

		sample, ok, err := parseCounterRule(rule)
		if err != nil {
			return nil, err
		}
		if !ok {
			continue
		}
		samples = append(samples, sample)
	}
	return samples, nil
}

type nftListTable struct {
	Nftables []map[string]json.RawMessage `json:"nftables"`
}

type nftCounterRule struct {
	Table   string                       `json:"table"`
	Chain   string                       `json:"chain"`
	Comment string                       `json:"comment"`
	Expr    []map[string]json.RawMessage `json:"expr"`
}

type nftCounter struct {
	Bytes   uint64 `json:"bytes"`
	Packets uint64 `json:"packets"`
}

func parseCounterRule(rule nftCounterRule) (CounterSample, bool, error) {
	var (
		counter    nftCounter
		hasCounter bool
	)

	for _, expr := range rule.Expr {
		if rawCounter, ok := expr["counter"]; ok {
			if err := json.Unmarshal(rawCounter, &counter); err != nil {
				return CounterSample{}, false, err
			}
			hasCounter = true
			continue
		}
	}
	if !hasCounter {
		return CounterSample{}, false, nil
	}

	sample, ok := ParseCounterComment(rule.Comment)
	if !ok {
		return CounterSample{}, false, nil
	}
	sample.Bytes = counter.Bytes
	sample.Packets = counter.Packets
	return sample, true, nil
}
