package dingtalk

import "time"

var approvalBusinessLocation = time.FixedZone("UTC+8", 8*60*60)

// ApprovalBusinessLocation is the explicit business timezone for approval dates.
func ApprovalBusinessLocation() *time.Location {
	return approvalBusinessLocation
}
