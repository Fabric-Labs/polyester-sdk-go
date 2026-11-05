package errors

import (
	"encoding/json"
	"strings"
)

// HTTPProblem is REST error metadata. Code is the stable discriminator.
type HTTPProblem struct {
	Type   string `json:"type"`
	Status int    `json:"status"`
	Code   string `json:"code"`
	Detail string `json:"detail"`
}

// ProblemType derives the canonical problem URI for every endpoint.
func ProblemType(code string) string {
	return "https://polyester.dev/problems/" + strings.ReplaceAll(strings.ToLower(code), "_", "-")
}

// DecodeHTTPProblem reads a REST problem and preserves even an empty detail.
// The actual HTTP status is authoritative.
func DecodeHTTPProblem(body []byte, status int) *HTTPProblem {
	var problem HTTPProblem
	if json.Unmarshal(body, &problem) != nil || problem.Code == "" {
		return nil
	}
	var payload map[string]json.RawMessage
	if json.Unmarshal(body, &payload) == nil {
		if _, present := payload["detail"]; !present {
			_ = json.Unmarshal(payload["message"], &problem.Detail)
		}
	}
	problem.Type = ProblemType(problem.Code)
	problem.Status = status
	return &problem
}
