package app

import (
	"time"

	"google.golang.org/protobuf/types/known/timestamppb"
)

// TimestampProto converts time.Time to protobuf Timestamp
func TimestampProto(t time.Time) *timestamppb.Timestamp {
	if t.IsZero() {
		return nil
	}
	return timestamppb.New(t)
}
