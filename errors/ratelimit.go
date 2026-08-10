package errors

// RateLimitDetail is the public, client-safe quota rejection payload from
// polyester.ratelimit.v1.RateLimitDetail.
//
// Optional numeric fields use pointers so absent proto oneofs stay distinguishable
// from zero values.
type RateLimitDetail struct {
	Reason        string  `json:"reason,omitempty"`
	Limit         *uint64 `json:"limit,omitempty"`
	Remaining     *uint64 `json:"remaining,omitempty"`
	RetryAfterMs  *uint64 `json:"retry_after_ms,omitempty"`
	PolicyVersion *uint64 `json:"policy_version,omitempty"`
	OperationID   string  `json:"operation_id,omitempty"`
	PolicyClass   string  `json:"policy_class,omitempty"`
	Scope         string  `json:"scope,omitempty"`
	RefillModel   string  `json:"refill_model,omitempty"`
}

// RetryAfterSeconds returns detail.retry_after_ms converted to seconds when set.
func (d *RateLimitDetail) RetryAfterSeconds() *float64 {
	if d == nil || d.RetryAfterMs == nil {
		return nil
	}
	seconds := float64(*d.RetryAfterMs) / 1000.0
	return &seconds
}
