package contracts

import "time"

type CandidateQuery struct {
	ResourceID string    `json:"resourceId"`
	Type       string    `json:"type"`
	Location   *Location `json:"location"`
}

type WorkflowCommand struct {
	Delivery     Delivery `json:"delivery"`
	Action       string   `json:"action"`
	AssignmentID string   `json:"assignmentId,omitempty"`
	Reason       string   `json:"reason,omitempty"`
}

func (c WorkflowCommand) Validate() error {
	d := c.Delivery
	if d.Event.Validate() != nil || d.RetryNumber < 0 || d.Destination != "request.match" || d.DeliveryID != DeliveryID(d.Event.EventID, d.RetryNumber, d.Destination) {
		return ErrInvalid
	}
	switch c.Action {
	case "inspect", "finalize", "reject", "unmatched", "retry", "pause", "redrive":
	default:
		return ErrInvalid
	}
	if len(c.Reason) > 256 {
		return ErrInvalid
	}
	return nil
}

type WorkflowState struct {
	Request       Request   `json:"request"`
	Intent        *Intent   `json:"intent,omitempty"`
	State         string    `json:"state"`
	ExpectedRetry int       `json:"expectedRetry"`
	DueAt         time.Time `json:"dueAt"`
	Ack           bool      `json:"ack"`
}
