package contracts

import "time"

type Notification struct {
	ID           string     `json:"id"`
	RequestID    string     `json:"requestId"`
	AssignmentID string     `json:"assignmentId"`
	ResourceID   string     `json:"resourceId"`
	Recipient    string     `json:"recipient"`
	CreatedAt    time.Time  `json:"createdAt"`
	ReadAt       *time.Time `json:"readAt,omitempty"`
}
type NotificationPage struct {
	Items     []Notification `json:"items"`
	NextAfter string         `json:"nextAfter,omitempty"`
}
