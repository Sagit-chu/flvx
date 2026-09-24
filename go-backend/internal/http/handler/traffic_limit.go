package handler

import (
	"fmt"
	"math"
	"strconv"
)

// flowMiB is optional so older clients can keep sending the GB-based flow field.
// A positive value takes precedence and preserves sub-GB limits exactly.
func parseTrafficLimit(req map[string]interface{}, defaultGB int64) (flowGB, flowMiB int64, err error) {
	flowGB = asInt64(req["flow"], defaultGB)
	if flowGB < 0 {
		return 0, 0, fmt.Errorf("流量限制不能小于0")
	}
	raw, present := req["flowMiB"]
	if !present {
		return flowGB, 0, nil
	}
	flowMiB, err = strconv.ParseInt(asString(raw), 10, 64)
	if err != nil || flowMiB < 0 || flowMiB > math.MaxInt64/bytesPerMiB {
		return 0, 0, fmt.Errorf("流量限制超出范围")
	}
	if flowMiB > 0 {
		flowGB = (flowMiB-1)/1024 + 1
	}
	return flowGB, flowMiB, nil
}
