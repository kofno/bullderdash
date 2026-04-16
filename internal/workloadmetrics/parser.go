package workloadmetrics

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"
)

var (
	errMissingJob       = errors.New("missing job")
	errInvalidTimestamp = errors.New("invalid timestamp")
)

func parseJobSample(values []interface{}) (jobSample, error) {
	if len(values) != 3 {
		return jobSample{}, fmt.Errorf("%w: expected 3 values, got %d", errMissingJob, len(values))
	}
	if values[0] == nil && values[1] == nil && values[2] == nil {
		return jobSample{}, errMissingJob
	}

	name := valueString(values[0])
	processedOn, err := parseMillis(values[1])
	if err != nil {
		return jobSample{Name: name}, err
	}
	finishedOn, err := parseMillis(values[2])
	if err != nil {
		return jobSample{Name: name}, err
	}
	if processedOn <= 0 || finishedOn <= 0 || finishedOn < processedOn {
		return jobSample{Name: name}, errInvalidTimestamp
	}

	return jobSample{
		Name:            name,
		DurationSeconds: float64(finishedOn-processedOn) / 1000,
		HasDuration:     true,
	}, nil
}

func parseMillis(value interface{}) (int64, error) {
	raw := valueString(value)
	if raw == "" {
		return 0, errInvalidTimestamp
	}
	parsed, err := strconv.ParseInt(raw, 10, 64)
	if err != nil {
		return 0, errInvalidTimestamp
	}
	return parsed, nil
}

func valueString(value interface{}) string {
	switch v := value.(type) {
	case nil:
		return ""
	case string:
		return v
	case []byte:
		return string(v)
	case fmt.Stringer:
		return v.String()
	default:
		return fmt.Sprintf("%v", v)
	}
}

func eventLagSeconds(id string, now time.Time) (float64, bool) {
	millisPart, _, ok := strings.Cut(id, "-")
	if !ok {
		return 0, false
	}
	millis, err := strconv.ParseInt(millisPart, 10, 64)
	if err != nil || millis <= 0 {
		return 0, false
	}
	lag := now.Sub(time.UnixMilli(millis)).Seconds()
	if lag < 0 {
		return 0, true
	}
	return lag, true
}
