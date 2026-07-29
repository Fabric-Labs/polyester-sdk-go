//go:build integration

package integration_test

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"testing"
	"time"

	polyester "github.com/Fabric-Labs/polyester-sdk-go"
	"github.com/Fabric-Labs/polyester-sdk-go/internal/testutil"
	"github.com/Fabric-Labs/polyester-sdk-go/models"
)

// TestBatchReplaceFiveRoundsOfForty is F-01/M1: BatchReplace admission + status regression.
func TestBatchReplaceFiveRoundsOfForty(t *testing.T) {
	testutil.RequireAccountWideCleanup(t)
	testutil.RequireFunded(t)
	testutil.RequireMutation(t)

	client, ok, err := testutil.LiveClientFromEnv()
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		testutil.SoftSkip(t, "POLYESTER_API_KEY_ID and POLYESTER_API_PRIVATE_KEY required")
	}
	defer client.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	symbol := testutil.TradeSymbol(t, client, ctx)
	testutil.RequireTradingBalanceForSymbol(t, ctx, client, symbol)
	spotRaw, err := testutil.HydrateSpotRaw(ctx, client)
	if err != nil {
		t.Fatal(err)
	}
	pair := testutil.PairForSymbol(spotRaw, symbol)
	if pair == nil {
		testutil.SoftSkipf(t, "symbol %s missing from spot config", symbol)
	}

	// Static far-below post-only price — live best-bid-1tick gets drained mid-batch.
	price := testutil.FarBelowBuyLimitPrice(symbol, pair)
	qty := testutil.MinBaseQtyForPair(pair, price)
	tif := "gtc"
	postOnly := true

	const batchSize = 40
	const rounds = 5
	cids := make([]string, 0, batchSize)
	cidSet := make(map[string]struct{}, batchSize)
	defer func() {
		if _, err := client.Orders.CancelAll(ctx, nil, nil, &symbol, nil, false, nil); err != nil {
			if testutil.StrictLiveEnabled() {
				t.Errorf("cleanup cancel_all failed: %v", err)
			} else {
				t.Logf("cleanup cancel_all warning: %v", err)
			}
		}
		if err := waitNoOpenCIDs(ctx, client, symbol, cidSet, 30*time.Second); err != nil {
			if testutil.StrictLiveEnabled() {
				t.Errorf("cleanup verification failed: %v", err)
			} else {
				t.Logf("cleanup verification warning: %v", err)
			}
		}
	}()

	for i := 0; i < batchSize; i++ {
		cid := testutil.UniqueClientOrderID(fmt.Sprintf("br-%d", i))
		_, err := client.Orders.Create(ctx, models.CreateOrderRequest{
			Symbol: &symbol, Side: "buy", OrderType: "limit", TIF: &tif,
			Qty: models.QtyFromDecimal(qty), Price: pricePtr(models.PriceFromDecimal(price)),
			ClientOrderID: &cid, PostOnly: postOnly,
		}, nil)
		if err != nil {
			if testutil.IsDevnetOrderInternalError(err) || testutil.DevnetUnavailable(err) {
				testutil.SoftSkipf(t, "create unavailable: %v", err)
			}
			t.Fatal(err)
		}
		if _, err := testutil.WaitForOpenOrder(ctx, client, cid, 50, 15*time.Second); err != nil {
			if _, ok := err.(*testutil.DevnetOrderNotIndexedError); ok {
				testutil.SoftSkipf(t, "create not indexed: %v", err)
			}
			msg := strings.ToLower(err.Error())
			if strings.Contains(msg, "terminal status") && strings.Contains(msg, "filled") {
				testutil.SoftSkipf(t, "post-only resting order filled by book activity before batch: %v", err)
			}
			t.Fatal(err)
		}
		cids = append(cids, cid)
		cidSet[cid] = struct{}{}
	}

	allCIDs := make(map[string]struct{}, batchSize*(rounds+1))
	for cid := range cidSet {
		allCIDs[cid] = struct{}{}
	}
	basePrice, _ := strconv.ParseFloat(price, 64)
	for round := 0; round < rounds; round++ {
		modifyPriceDec := fmt.Sprintf("%f", basePrice*0.99-float64(round)*0.001*basePrice)
		modifyPrice := models.PriceFromDecimal(modifyPriceDec)
		newCIDs := make([]string, batchSize)
		requested := make(map[string]struct{}, batchSize)
		items := make([]models.BatchReplaceItem, 0, batchSize)
		for i, cid := range cids {
			cid := cid
			requested[cid] = struct{}{}
			newCID := testutil.UniqueClientOrderID(fmt.Sprintf("br-r%d-%d", round, i))
			newCIDs[i] = newCID
			allCIDs[newCID] = struct{}{}
			p := modifyPrice
			nc := newCID
			items = append(items, models.BatchReplaceItem{
				Key:              models.OrderKeyByClientID(cid),
				NewPrice:         &p,
				NewClientOrderID: &nc,
			})
		}
		reqID := testutil.UniqueClientOrderID(fmt.Sprintf("br-req-%d", round))

		beforeCount := 0
		for attempt := 0; attempt < 20; attempt++ {
			limit := 100
			beforeOpen, err := client.Orders.ListOpen(ctx, nil, nil, nil, &limit, false, false)
			if err != nil {
				t.Fatal(err)
			}
			beforeCount = 0
			for _, order := range beforeOpen.Orders {
				if _, ok := requested[order.ClientOrderID]; ok {
					beforeCount++
				}
			}
			if beforeCount == batchSize {
				break
			}
			time.Sleep(500 * time.Millisecond)
		}
		if beforeCount != batchSize {
			testutil.SoftSkipf(t, "round %d: only %d/%d test orders still open (book activity drained resting post-only bids)",
				round, beforeCount, batchSize)
		}

		result, err := client.Orders.BatchReplace(ctx, nil, items, symbol, nil, &reqID)
		if err != nil {
			if testutil.DevnetUnavailable(err) {
				testutil.SoftSkipf(t, "batch_replace unavailable: %v", err)
			}
			// Timeout / ambiguous commit: reconcile then same request_id retry.
			limit := 100
			afterOpen, listErr := client.Orders.ListOpen(ctx, nil, nil, nil, &limit, false, false)
			if listErr != nil {
				t.Fatalf("round %d reconcile list_open: %v (first=%v)", round, listErr, err)
			}
			afterCount := 0
			for _, order := range afterOpen.Orders {
				if _, ok := allCIDs[order.ClientOrderID]; ok {
					afterCount++
				}
			}
			if afterCount != batchSize {
				t.Fatalf("round %d: open set changed during failed attempt (%d); first=%v", round, afterCount, err)
			}
			time.Sleep(200 * time.Millisecond)
			result, err = client.Orders.BatchReplace(ctx, nil, items, symbol, nil, &reqID)
			if err != nil {
				t.Fatalf("batch_replace round %d: %v", round, err)
			}
		}
		assertCompleteBatchReplaceAdmission(t, result, round)
		status, err := waitBatchReplaceSettled(ctx, client, result.BatchRequestID, 30*time.Second)
		if err != nil {
			t.Fatalf("round %d status wait: %v", round, err)
		}
		assertBatchReplaceStatusComplete(t, status, round)
		fingerprint := batchReplaceFingerprint(result)

		// Intentional identical request_id retry after success — prove no double-apply.
		retryOK, err := client.Orders.BatchReplace(ctx, nil, items, symbol, nil, &reqID)
		if err != nil {
			t.Fatalf("idempotent retry round %d: %v", round, err)
		}
		assertCompleteBatchReplaceAdmission(t, retryOK, round)
		if got := batchReplaceFingerprint(retryOK); fmt.Sprint(got) != fmt.Sprint(fingerprint) {
			t.Fatalf("round %d: idempotent retry changed admission receipt: before=%v after=%v", round, fingerprint, got)
		}
		// Resolve live key per item: prefer newCID when open.
		nextCIDs := make([]string, 0, batchSize)
		for i, oldCID := range cids {
			newCID := newCIDs[i]
			chosen := ""
			if _, err := testutil.WaitForOpenOrder(ctx, client, newCID, 50, 3*time.Second); err == nil {
				chosen = newCID
			} else if _, err := testutil.WaitForOpenOrder(ctx, client, oldCID, 50, 3*time.Second); err == nil {
				chosen = oldCID
			} else {
				t.Fatalf("round %d: neither %q nor %q open after replace", round, oldCID, newCID)
			}
			nextCIDs = append(nextCIDs, chosen)
		}
		cids = nextCIDs
		cidSet = make(map[string]struct{}, batchSize)
		for _, cid := range cids {
			cidSet[cid] = struct{}{}
		}
	}
	// Cleanup uses allCIDs via deferred waitNoOpenCIDs on cidSet — expand defer target.
	cidSet = allCIDs
}

func waitBatchReplaceSettled(ctx context.Context, client *polyester.Client, batchRequestID string, timeout time.Duration) (models.BatchReplaceStatusResult, error) {
	deadline := time.Now().Add(timeout)
	var last models.BatchReplaceStatusResult
	var lastErr error
	for {
		status, err := client.Orders.GetBatchReplaceStatus(ctx, nil, batchRequestID, nil)
		if err != nil {
			lastErr = err
			// Status projection can lag the admission receipt briefly.
			if !strings.Contains(strings.ToLower(err.Error()), "not found") {
				return last, err
			}
		} else {
			last = status
			lastErr = nil
			if batchReplaceStatusSettled(status) {
				return status, nil
			}
		}
		if time.Now().After(deadline) {
			if lastErr != nil {
				return last, fmt.Errorf("batch %s not settled within %s: %w", batchRequestID, timeout, lastErr)
			}
			return last, fmt.Errorf("batch %s not settled within %s: %+v", batchRequestID, timeout, last)
		}
		select {
		case <-ctx.Done():
			return last, ctx.Err()
		case <-time.After(200 * time.Millisecond):
		}
	}
}

func batchReplaceStatusSettled(status models.BatchReplaceStatusResult) bool {
	if len(status.Items) == 0 {
		return false
	}
	for _, item := range status.Items {
		switch item.Phase {
		case "working", "rejected", "terminal":
			continue
		default:
			return false
		}
	}
	return true
}

func assertCompleteBatchReplaceAdmission(t *testing.T, result models.BatchReplaceOrdersResult, round int) {
	t.Helper()
	if result.BatchRequestID == "" || result.BatchRequestID == "0" {
		t.Fatalf("round %d: missing batch_request_id", round)
	}
	if len(result.Results) != 40 {
		t.Fatalf("round %d: expected 40 result items, got %d", round, len(result.Results))
	}
	if allAdmissionInternalError(result) {
		testutil.SoftSkip(t, "Devnet order placement returned INTERNAL_ERROR for USDT-funded buys; check OMS on devnet")
	}
	if result.RejectedCount != 0 {
		t.Fatalf("round %d: expected no rejected, got %d", round, result.RejectedCount)
	}
	if result.AcceptedCount != 40 {
		t.Fatalf("round %d: accepted=%d != 40", round, result.AcceptedCount)
	}
	if result.Status != "admitted" && result.Status != "partially_admitted" {
		t.Fatalf("round %d: unexpected admission status %q", round, result.Status)
	}
	for _, item := range result.Results {
		if item.Status == "rejected" {
			t.Fatalf("round %d: rejected item %+v", round, item)
		}
	}
}

func assertBatchReplaceStatusComplete(t *testing.T, status models.BatchReplaceStatusResult, round int) {
	t.Helper()
	if len(status.Items) != 40 {
		t.Fatalf("round %d: expected 40 status items, got %d", round, len(status.Items))
	}
	if status.RejectedCount != 0 {
		t.Fatalf("round %d: status rejected_count=%d", round, status.RejectedCount)
	}
	for _, item := range status.Items {
		if item.Phase == "rejected" {
			t.Fatalf("round %d: rejected status item %+v", round, item)
		}
	}
}

func allAdmissionInternalError(result models.BatchReplaceOrdersResult) bool {
	if len(result.Results) == 0 {
		return false
	}
	for _, item := range result.Results {
		if !strings.EqualFold(item.Code, "INTERNAL_ERROR") {
			return false
		}
	}
	return true
}

func batchReplaceFingerprint(result models.BatchReplaceOrdersResult) []string {
	out := make([]string, 0, len(result.Results)+2)
	out = append(out, result.BatchRequestID, result.Status)
	for _, item := range result.Results {
		out = append(out, fmt.Sprintf("%d|%s|%s|%s", item.ItemIndex, item.Status, item.ClientOrderID, item.ReplacementOrderID))
	}
	for i := 0; i < len(out); i++ {
		for j := i + 1; j < len(out); j++ {
			if out[j] < out[i] {
				out[i], out[j] = out[j], out[i]
			}
		}
	}
	return out
}
