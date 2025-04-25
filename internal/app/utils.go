package app

import (
	"time"
	
	"google.golang.org/protobuf/types/known/timestamppb"
)

// TimestampProto конвертирует время time.Time в protobuf Timestamp
func TimestampProto(t time.Time) *timestamppb.Timestamp {
	if t.IsZero() {
		return nil
	}
	return timestamppb.New(t)
}
