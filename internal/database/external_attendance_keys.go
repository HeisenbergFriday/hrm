package database

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"
)

// BuildExternalApproveItemKey builds a non-null unique key for approve_list items.
// Used by runtime upserts and by schema backfill migration — keep one implementation.
func BuildExternalApproveItemKey(procInstID, tagName, bizType, subType, begin, end, duration, durationUnit string) string {
	return hashExternalParts(
		strings.TrimSpace(procInstID),
		strings.TrimSpace(tagName),
		strings.TrimSpace(bizType),
		strings.TrimSpace(subType),
		strings.TrimSpace(begin),
		strings.TrimSpace(end),
		strings.TrimSpace(duration),
		strings.TrimSpace(durationUnit),
	)
}

func hashExternalParts(parts ...string) string {
	h := sha256.New()
	for i, p := range parts {
		if i > 0 {
			h.Write([]byte{0})
		}
		h.Write([]byte(p))
	}
	return hex.EncodeToString(h.Sum(nil))
}
