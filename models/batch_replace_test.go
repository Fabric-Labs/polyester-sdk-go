package models

import "testing"

func TestBatchReplaceStatusSettled(t *testing.T) {
	settled := BatchReplaceStatusResult{Items: []BatchReplaceStatusItem{
		{Phase: "working"},
		{Phase: "rejected"},
		{Phase: "terminal"},
	}}
	if !BatchReplaceStatusSettled(settled) || !IsBatchReplaceSettled(settled) || !settled.BatchReplaceStatusSettled() {
		t.Fatalf("expected settled: %+v", settled)
	}
	for _, status := range []BatchReplaceStatusResult{
		{},
		{Items: []BatchReplaceStatusItem{{Phase: "admitted"}}},
		{Items: []BatchReplaceStatusItem{{Phase: "unknown"}}},
	} {
		if BatchReplaceStatusSettled(status) {
			t.Fatalf("expected unsettled: %+v", status)
		}
	}
}
