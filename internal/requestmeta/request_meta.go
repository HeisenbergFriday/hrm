package requestmeta

import (
	"context"
	"sync/atomic"
)

type contextKey string

const requestInfoKey contextKey = "peopleops_request_info"

type RequestInfo struct {
	RequestID string
	Route     string
	OrgID     string
	SQLCount  atomic.Int64
}

func WithRequestInfo(ctx context.Context, info *RequestInfo) context.Context {
	return context.WithValue(ctx, requestInfoKey, info)
}

func FromContext(ctx context.Context) *RequestInfo {
	if ctx == nil {
		return nil
	}
	info, _ := ctx.Value(requestInfoKey).(*RequestInfo)
	return info
}

func SetOrgID(ctx context.Context, orgID string) {
	if info := FromContext(ctx); info != nil {
		info.OrgID = orgID
	}
}
