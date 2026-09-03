package decode

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/Fabric-Labs/polyester-sdk-go/codecs"
	sdkerrors "github.com/Fabric-Labs/polyester-sdk-go/errors"
	orderv1 "github.com/Fabric-Labs/polyester-sdk-go/gen/orders/v1"
	"github.com/Fabric-Labs/polyester-sdk-go/models"
)

// OrderFromProto decodes an order message.
func OrderFromProto(msg *orderv1.Order) models.Order {
	if msg == nil {
		return models.Order{}
	}
	sid := msg.GetSymbolId()
	return models.Order{
		OrderID:       codecs.FormatUint64ID(msg.GetOrderId()),
		SymbolID:      sid,
		ClientOrderID: msg.GetClientOrderId(),
		Side:          codecs.OrderSideName(msg.GetSide()),
		Status:        orderStatusName(msg.GetStatus()),
		OrderType:     codecs.OrderTypeName(msg.GetOrderType()),
		TIF:           codecs.TimeInForceName(msg.GetTimeInForce()),
		// Current accepted total; changes after a successful modify.
		OrigQty:     codecs.DecodeQtyScaled(msg.GetOrigQtyScaled(), -1, "", &sid),
		CumQty:      codecs.DecodeQtyScaled(msg.GetCumQtyScaled(), -1, "", &sid),
		LeavesQty:   codecs.DecodeQtyScaled(msg.GetLeavesQtyScaled(), -1, "", &sid),
		Price:       codecs.DecodePriceTicks(msg.GetPriceTicks(), ""),
		AvgPx:       codecs.DecodePriceTicks(msg.GetAvgPriceTicks(), ""),
		CreatedTsNs: strconv.FormatUint(msg.GetCreatedTsNs(), 10),
		Version:     msg.GetVersion(),
		PostOnly:    msg.GetPostOnly(),
		FeeAsset:    feeAssetName(msg.GetFeeAsset()),
		SubmittedMaxQuoteDebitScaled: optionalScaledString(
			msg.SubmittedMaxQuoteDebitScaled,
		),
		AttachedRisk: attachedRiskFromProto(msg.GetAttachedRisk()),
	}
}

func attachedRiskLegStateFromProto(state *orderv1.AttachedRiskLegState) *models.AttachedRiskLegState {
	if !hasMeaningfulAttachedRiskLegState(state) {
		return nil
	}
	out := &models.AttachedRiskLegState{}
	if status := state.GetStatus(); status != orderv1.AttachedRiskLegState_STATUS_UNSPECIFIED {
		out.Status = strings.ToLower(status.String())
	}
	if state.GetArmedTsNs() != 0 {
		out.ArmedTsNs = strconv.FormatUint(state.GetArmedTsNs(), 10)
	}
	if state.GetTerminalTsNs() != 0 {
		out.TerminalTsNs = strconv.FormatUint(state.GetTerminalTsNs(), 10)
	}
	if state.TriggerId != nil && state.GetTriggerId() != 0 {
		out.TriggerID = codecs.FormatUint64ID(state.GetTriggerId())
	}
	if state.ChildOrderId != nil && state.GetChildOrderId() != 0 {
		out.ChildOrderID = codecs.FormatUint64ID(state.GetChildOrderId())
	}
	return out
}

func hasMeaningfulAttachedRiskLegState(state *orderv1.AttachedRiskLegState) bool {
	return state != nil &&
		(state.GetStatus() != orderv1.AttachedRiskLegState_STATUS_UNSPECIFIED ||
			state.GetArmedTsNs() != 0 ||
			state.GetTerminalTsNs() != 0 ||
			(state.TriggerId != nil && state.GetTriggerId() != 0) ||
			(state.ChildOrderId != nil && state.GetChildOrderId() != 0))
}

// riskLegFromChild projects an attached take-profit/stop-loss policy and its
// runtime state onto the public RiskLeg. The child execution determines
// order_type/limit_price. TriggerPriceSource is no longer part of the policy
// wire and is left empty.
func riskLegFromChild(triggerPriceTicks int64, child *orderv1.RiskExecution, state *orderv1.AttachedRiskLegState, policyUsable bool) *models.RiskLeg {
	if !policyUsable && !hasMeaningfulAttachedRiskLegState(state) {
		return nil
	}
	leg := &models.RiskLeg{
		State: attachedRiskLegStateFromProto(state),
	}
	if !policyUsable {
		return leg
	}
	if triggerPriceTicks > 0 {
		leg.TriggerPrice = codecs.DecodePriceTicks(triggerPriceTicks, "")
	}
	if child != nil {
		switch {
		case child.GetMarketIoc() != nil:
			leg.OrderType = "market"
		case child.GetLimitGtc() != nil:
			leg.OrderType = "limit"
			leg.LimitPrice = codecs.DecodePriceTicks(child.GetLimitGtc().GetPriceTicks(), "")
		}
	}
	return leg
}

func riskLegFromTakeProfit(policy *orderv1.TakeProfitPolicy, state *orderv1.AttachedRiskLegState) *models.RiskLeg {
	if policy == nil {
		return riskLegFromChild(0, nil, state, false)
	}
	return riskLegFromChild(policy.GetTriggerPriceTicks(), policy.GetChild(), state, policy.GetTriggerPriceTicks() > 0)
}

func riskLegFromStopLoss(policy *orderv1.StopLossPolicy, state *orderv1.AttachedRiskLegState) *models.RiskLeg {
	if policy == nil {
		return riskLegFromChild(0, nil, state, false)
	}
	return riskLegFromChild(policy.GetTriggerPriceTicks(), policy.GetChild(), state, policy.GetTriggerPriceTicks() > 0)
}

func trailingStopFromPolicy(policy *orderv1.TrailingStopPolicy, state *orderv1.AttachedRiskLegState) *models.TrailingStop {
	if policy == nil {
		if !hasMeaningfulAttachedRiskLegState(state) {
			return nil
		}
		return &models.TrailingStop{State: attachedRiskLegStateFromProto(state)}
	}
	distanceTicks := policy.GetTrailingDistanceTicks()
	distanceBps := policy.GetTrailingDistanceBps()
	policyUsable := distanceTicks > 0 || distanceBps > 0
	if !policyUsable && !hasMeaningfulAttachedRiskLegState(state) {
		return nil
	}
	// OrderType/TriggerPriceSource were dropped from the trailing-stop policy
	// wire; the child is an implicit market execution.
	out := &models.TrailingStop{
		State: attachedRiskLegStateFromProto(state),
	}
	if !policyUsable {
		return out
	}
	out.DistanceTicks = distanceTicks
	out.DistanceBps = distanceBps
	if ticks := policy.GetMaxSlippageTicks(); ticks > 0 {
		out.MaxSlippageTicks = ticks
	}
	if bps := policy.GetMaxSlippageBps(); bps > 0 {
		out.MaxSlippageBps = bps
	}
	if policy.GetActivationPriceTicks() > 0 {
		out.ActivationPrice = codecs.DecodePriceTicks(policy.GetActivationPriceTicks(), "")
	}
	return out
}

func attachedRiskFromProto(msg *orderv1.AttachedRisk) *models.AttachedRisk {
	if msg == nil {
		return nil
	}
	var takeProfit *models.RiskLeg
	if tp := msg.GetTakeProfit(); tp != nil {
		takeProfit = riskLegFromTakeProfit(tp.GetPolicy(), tp.GetState())
	}
	var trailingStop *models.TrailingStop
	if ts := msg.GetTrailingStop(); ts != nil {
		trailingStop = trailingStopFromPolicy(ts.GetPolicy(), ts.GetState())
	}
	var stopLoss *models.RiskLeg
	if sl := msg.GetStopLoss(); sl != nil {
		stopLoss = riskLegFromStopLoss(sl.GetPolicy(), sl.GetState())
	}
	if takeProfit == nil && stopLoss == nil && trailingStop == nil {
		return nil
	}
	return &models.AttachedRisk{
		TakeProfit:   takeProfit,
		StopLoss:     stopLoss,
		TrailingStop: trailingStop,
		Oco:          msg.GetOco(),
	}
}

func orderStatusName(v orderv1.OrderStatus) string {
	switch v {
	case orderv1.OrderStatus_PENDING:
		return "pending"
	case orderv1.OrderStatus_PENDING_CANCEL:
		return "pending_cancel"
	case orderv1.OrderStatus_WORKING:
		return "working"
	case orderv1.OrderStatus_FILLED:
		return "filled"
	case orderv1.OrderStatus_CANCELED:
		return "canceled"
	case orderv1.OrderStatus_REJECTED:
		return "rejected"
	default:
		return ""
	}
}

// OrdersListFromOpen decodes open orders response.
func OrdersListFromOpen(msg *orderv1.GetOpenOrdersResponse) models.OrdersList {
	return ordersList(msg.GetOrders(), msg.GetNextPageToken())
}

// OrdersListFromHistory decodes order history response.
func OrdersListFromHistory(msg *orderv1.GetOrderHistoryResponse) models.OrdersList {
	return ordersList(msg.GetOrders(), msg.GetNextPageToken())
}

func ordersList(orders []*orderv1.Order, token string) models.OrdersList {
	out := make([]models.Order, 0, len(orders))
	for _, o := range orders {
		out = append(out, OrderFromProto(o))
	}
	return models.OrdersList{Orders: out, NextPageToken: token}
}

// UserTradeFromProto decodes a user trade.
func UserTradeFromProto(msg *orderv1.UserTrade) models.UserTrade {
	if msg == nil {
		return models.UserTrade{}
	}
	sid := msg.GetSymbolId()
	return models.UserTrade{
		SymbolID:               sid,
		MatchID:                strconv.FormatUint(msg.GetMatchId(), 10),
		OrderID:                codecs.FormatUint64ID(msg.GetOrderId()),
		Side:                   codecs.OrderSideName(msg.GetSide()),
		IsMaker:                msg.GetIsMaker(),
		Price:                  codecs.DecodePriceTicks(msg.GetPriceTicks(), ""),
		Qty:                    codecs.DecodeQtyScaled(msg.GetQtyScaled(), -1, "", &sid),
		FeeAmountE18:           u128(msg.GetFeeAmountE18()),
		FeeAsset:               feeAssetName(msg.GetFeeAsset()),
		ReferralShareAmountE18: u128(msg.GetReferralShareAmountE18()),
		TsNs:                   strconv.FormatUint(msg.GetTsNs(), 10),
		FeeIsRebate:            msg.GetFeeIsRebate(),
	}
}

func feeAssetName(asset orderv1.FeeAsset) string {
	switch asset {
	case orderv1.FeeAsset_FEE_ASSET_UNSPECIFIED:
		return ""
	case orderv1.FeeAsset_QUOTE:
		return "quote"
	case orderv1.FeeAsset_BASE:
		return "base"
	default:
		return fmt.Sprintf("unknown(%d)", asset)
	}
}

func optionalScaledString(value *int64) string {
	if value == nil {
		return ""
	}
	return strconv.FormatInt(*value, 10)
}

func qtyFromScaled(value int64) *models.QtyScaled {
	if value == 0 {
		return nil
	}
	qty := codecs.DecodeQtyScaled(value, -1, "", nil)
	return &qty
}

// UserTradesListFromProto decodes user trades list.
func UserTradesListFromProto(msg *orderv1.GetUserTradesResponse) models.UserTradesList {
	trades := make([]models.UserTrade, 0, len(msg.GetTrades()))
	for _, t := range msg.GetTrades() {
		trades = append(trades, UserTradeFromProto(t))
	}
	return models.UserTradesList{Trades: trades, NextPageToken: msg.GetNextPageToken()}
}

// GetOrderFromProto decodes get order response.
func GetOrderFromProto(msg *orderv1.GetOrderResponse) models.GetOrderResult {
	var order *models.Order
	if msg.GetOrder() != nil {
		o := OrderFromProto(msg.GetOrder())
		order = &o
	}
	trades := make([]models.UserTrade, 0, len(msg.GetTrades()))
	for _, t := range msg.GetTrades() {
		trades = append(trades, UserTradeFromProto(t))
	}
	return models.GetOrderResult{Order: order, Trades: trades}
}

// OrderMutationFromProto decodes order mutation response.
//
// CreateOrderResponse acknowledges admission only and no longer carries a
// status field; synthesize "accepted".
func OrderMutationFromProto(msg *orderv1.CreateOrderResponse) (models.OrderMutationResult, error) {
	if msg.GetOrderId() == 0 {
		return models.OrderMutationResult{}, &sdkerrors.ResponseContractError{Operation: "CreateOrder", Msg: "missing order_id"}
	}
	result := orderMutation("accepted", msg.GetOrderId(), msg.GetClientOrderId())
	if value := msg.GetResolvedBaseQtyScaled(); value != 0 {
		result.ResolvedBaseQtyScaled = strconv.FormatInt(value, 10)
		result.ResolvedBaseQty = qtyFromScaled(value)
	}
	result.SubmittedMaxQuoteDebitScaled = optionalScaledString(msg.SubmittedMaxQuoteDebitScaled)
	return result, nil
}

// OrderMutationFromCancel decodes cancel response.
func OrderMutationFromCancel(msg *orderv1.CancelOrderResponse) (models.OrderMutationResult, error) {
	status := cancelOrderStatusName(msg.GetStatus())
	if msg.GetOrderId() == 0 || status == "" {
		return models.OrderMutationResult{}, &sdkerrors.ResponseContractError{Operation: "CancelOrder", Msg: "missing order_id or status"}
	}
	return orderMutation(status, msg.GetOrderId(), ""), nil
}

func cancelOrderStatusName(status orderv1.CancelOrderResponse_Status) string {
	switch status {
	case orderv1.CancelOrderResponse_ACCEPTED:
		return "accepted"
	default:
		return ""
	}
}

func orderMutation(status string, orderID uint64, clientOrderID string) models.OrderMutationResult {
	return models.OrderMutationResult{
		Status:        status,
		OrderID:       codecs.FormatUint64ID(orderID),
		ClientOrderID: clientOrderID,
	}
}

// PreviewOrderFromProto decodes the advisory PreviewOrder admission response.
//
// baseScale / symbol come from the service layer (catalog lookup) so resolved
// base quantity can be typed when present. Fee/quote estimates are no longer on
// the wire.
func PreviewOrderFromProto(
	msg *orderv1.PreviewOrderResponse,
	baseScale int,
	symbol string,
	symbolID *uint32,
) (models.PreviewOrderResult, error) {
	if msg == nil {
		return models.PreviewOrderResult{}, nil
	}
	if msg.GetEvaluatedAt() == nil {
		return models.PreviewOrderResult{}, &sdkerrors.ResponseContractError{
			Operation: "PreviewOrder",
			Msg:       "successful response is missing evaluated_at",
		}
	}
	result := models.PreviewOrderResult{}
	if msg.Admissible != nil {
		admissible := msg.GetAdmissible()
		result.Admissible = &admissible
	}
	if rejection := msg.GetRejection(); rejection != nil {
		result.Rejection = orderErrorDetailFromProto(rejection)
	}
	if msg.ResolvedBaseQtyScaled != nil {
		value := msg.GetResolvedBaseQtyScaled()
		result.ResolvedBaseQtyScaled = strconv.FormatInt(value, 10)
		qty := codecs.DecodeQtyScaled(value, baseScale, symbol, symbolID)
		result.ResolvedBaseQty = &qty
	}
	if msg.ProtectedPriceBoundTicks != nil {
		price := codecs.DecodePriceTicks(msg.GetProtectedPriceBoundTicks(), symbol)
		result.ProtectedPriceBound = &price
	}
	result.EvaluatedAtMs = msg.GetEvaluatedAt().AsTime().UnixMilli()
	return result, nil
}

func orderErrorDetailFromProto(msg *orderv1.ErrorDetail) *models.OrderErrorDetail {
	if msg == nil {
		return nil
	}
	out := &models.OrderErrorDetail{
		Code:      errorCodeName(msg.GetCode()),
		RateLimit: RateLimitDetailFromProto(msg.GetRateLimit()),
	}
	if violations := msg.GetViolations(); len(violations) > 0 {
		out.Violations = make([]models.OrderFieldViolation, 0, len(violations))
		for _, v := range violations {
			if v == nil {
				continue
			}
			out.Violations = append(out.Violations, models.OrderFieldViolation{
				FieldPath: v.GetFieldPath(),
				RuleID:    v.GetRuleId(),
				Message:   v.GetMessage(),
			})
		}
	}
	return out
}

func errorCodeName(code orderv1.ErrorCode) string {
	raw := int32(code)
	if raw == 0 {
		return "UNSPECIFIED"
	}
	if name, ok := orderv1.ErrorCode_name[raw]; ok {
		return strings.TrimPrefix(name, "ERROR_CODE_")
	}
	return fmt.Sprintf("UNKNOWN_ERROR_CODE(%d)", raw)
}

// ModifyOrderFromProto decodes modify order response.
func ModifyOrderFromProto(msg *orderv1.ModifyOrderResponse) (models.ModifyOrderResult, error) {
	action := modifyActionName(msg.GetActionTaken())
	if action == "" || msg.GetOldOrderId() == 0 || msg.GetFinalOrderId() == 0 {
		return models.ModifyOrderResult{}, &sdkerrors.ResponseContractError{Operation: "ModifyOrder", Msg: "missing action_taken, old_order_id, or final_order_id"}
	}
	return models.ModifyOrderResult{
		ActionTaken:  action,
		OldOrderID:   codecs.FormatUint64ID(msg.GetOldOrderId()),
		FinalOrderID: codecs.FormatUint64ID(msg.GetFinalOrderId()),
		Code:         msg.GetCode(),
	}, nil
}

func modifyActionName(v orderv1.ModifyActionTaken) string {
	switch v {
	case orderv1.ModifyActionTaken_AMENDED:
		return "amended"
	case orderv1.ModifyActionTaken_REPLACED:
		return "replaced"
	default:
		return ""
	}
}

// CancelAllFromProto decodes cancel all response.
func CancelAllFromProto(msg *orderv1.CancelAllOrdersResponse) (models.CancelAllOrdersResult, error) {
	status := cancelAllStatusName(msg.GetStatus())
	if status == "" {
		return models.CancelAllOrdersResult{}, &sdkerrors.ResponseContractError{Operation: "CancelAllOrders",
			Msg: fmt.Sprintf("invalid CancelAllOrders response: unknown status %d", msg.GetStatus()),
		}
	}
	matched, submitted, failed := msg.GetMatchedOrders(), msg.GetSubmittedCancels(), msg.GetFailedCancels()
	if status == "submitted" && uint64(submitted)+uint64(failed) != uint64(matched) {
		return models.CancelAllOrdersResult{}, &sdkerrors.ResponseContractError{Operation: "CancelAllOrders", Msg: "submitted and failed counts do not equal matched_orders"}
	}
	if status == "dry_run" && (submitted != 0 || failed != 0) {
		return models.CancelAllOrdersResult{}, &sdkerrors.ResponseContractError{Operation: "CancelAllOrders", Msg: "dry_run reported submitted or failed cancels"}
	}
	return models.CancelAllOrdersResult{
		Status:           status,
		MatchedOrders:    int(msg.GetMatchedOrders()),
		SubmittedCancels: int(msg.GetSubmittedCancels()),
		FailedCancels:    int(msg.GetFailedCancels()),
	}, nil
}

func cancelAllStatusName(status orderv1.CancelAllOrdersResponse_Status) string {
	switch status {
	case orderv1.CancelAllOrdersResponse_SUBMITTED:
		return "submitted"
	case orderv1.CancelAllOrdersResponse_DRY_RUN:
		return "dry_run"
	default:
		return ""
	}
}

// BatchReplaceFromProto decodes batch replace admission receipt.
func BatchReplaceFromProto(msg *orderv1.BatchReplaceOrdersResponse) (models.BatchReplaceOrdersResult, error) {
	if msg.GetBatchRequestId() == 0 {
		return models.BatchReplaceOrdersResult{}, &sdkerrors.ResponseContractError{
			Operation: "BatchReplaceOrders", Msg: "missing batch_request_id",
		}
	}
	status := batchReplaceAdmissionStatusName(msg.GetStatus())
	if status == "" {
		return models.BatchReplaceOrdersResult{}, &sdkerrors.ResponseContractError{
			Operation: "BatchReplaceOrders",
			Msg:       fmt.Sprintf("unknown admission status: %d", msg.GetStatus()),
		}
	}
	results := make([]models.BatchReplaceAdmissionItem, 0, len(msg.GetResults()))
	decodedAccepted := 0
	decodedRejected := 0
	for _, item := range msg.GetResults() {
		itemStatus := batchReplaceItemAdmissionStatusName(item.GetStatus())
		switch itemStatus {
		case "admitted":
			decodedAccepted++
		case "rejected":
			decodedRejected++
		default:
			return models.BatchReplaceOrdersResult{}, &sdkerrors.ResponseContractError{
				Operation: "BatchReplaceOrders",
				Msg:       fmt.Sprintf("batch replace response has unknown item status: %d", item.GetStatus()),
			}
		}
		var rateLimit *models.RateLimitDetail
		if errDetail := item.GetError(); errDetail != nil {
			rateLimit = RateLimitDetailFromProto(errDetail.GetRateLimit())
		}
		results = append(results, models.BatchReplaceAdmissionItem{
			ItemIndex:          item.GetItemIndex(),
			Status:             itemStatus,
			OldOrderID:         codecs.FormatUint64ID(item.GetOldOrderId()),
			ReplacementOrderID: codecs.FormatUint64ID(item.GetReplacementOrderId()),
			ClientOrderID:      item.GetClientOrderId(),
			Code:               item.GetCode(),
			RateLimit:          rateLimit,
		})
	}
	accepted := int(msg.GetAcceptedCount())
	rejected := int(msg.GetRejectedCount())
	if accepted != decodedAccepted || rejected != decodedRejected || accepted+rejected != len(results) {
		return models.BatchReplaceOrdersResult{}, &sdkerrors.ResponseContractError{
			Operation: "BatchReplaceOrders",
			Msg: fmt.Sprintf(
				"batch replace response counts do not match decoded outcomes: accepted=%d/%d rejected=%d/%d results=%d",
				accepted, decodedAccepted, rejected, decodedRejected, len(results),
			),
		}
	}
	return models.BatchReplaceOrdersResult{
		BatchRequestID: codecs.FormatUint64ID(msg.GetBatchRequestId()),
		Status:         status,
		Results:        results,
		AcceptedCount:  accepted,
		RejectedCount:  rejected,
		AcceptedTsNs:   msg.GetAcceptedTsNs(),
	}, nil
}

// BatchReplaceStatusFromProto decodes get batch replace status response.
func BatchReplaceStatusFromProto(msg *orderv1.GetBatchReplaceStatusResponse) (models.BatchReplaceStatusResult, error) {
	if msg.GetBatchRequestId() == 0 {
		return models.BatchReplaceStatusResult{}, &sdkerrors.ResponseContractError{
			Operation: "GetBatchReplaceStatus", Msg: "missing batch_request_id",
		}
	}
	admission := batchReplaceAdmissionStatusName(msg.GetAdmissionStatus())
	if admission == "" {
		return models.BatchReplaceStatusResult{}, &sdkerrors.ResponseContractError{
			Operation: "GetBatchReplaceStatus",
			Msg:       fmt.Sprintf("unknown admission status: %d", msg.GetAdmissionStatus()),
		}
	}
	items := make([]models.BatchReplaceStatusItem, 0, len(msg.GetItems()))
	decodedAccepted := 0
	decodedRejected := 0
	for _, item := range msg.GetItems() {
		phase := batchReplacePhaseName(item.GetPhase())
		if phase == "" {
			return models.BatchReplaceStatusResult{}, &sdkerrors.ResponseContractError{
				Operation: "GetBatchReplaceStatus",
				Msg:       fmt.Sprintf("unknown batch replace phase: %d", item.GetPhase()),
			}
		}
		switch phase {
		case "rejected":
			decodedRejected++
		case "admitted", "working", "terminal":
			decodedAccepted++
		}
		items = append(items, models.BatchReplaceStatusItem{
			ItemIndex:          item.GetItemIndex(),
			Phase:              phase,
			OldOrderID:         codecs.FormatUint64ID(item.GetOldOrderId()),
			ReplacementOrderID: codecs.FormatUint64ID(item.GetReplacementOrderId()),
			OrderStatus:        orderStatusName(item.GetOrderStatus()),
			Code:               item.GetCode(),
			UpdatedTsNs:        item.GetUpdatedTsNs(),
		})
	}
	accepted := int(msg.GetAcceptedCount())
	rejected := int(msg.GetRejectedCount())
	if accepted != decodedAccepted || rejected != decodedRejected || accepted+rejected != len(items) {
		return models.BatchReplaceStatusResult{}, &sdkerrors.ResponseContractError{
			Operation: "GetBatchReplaceStatus",
			Msg: fmt.Sprintf(
				"batch replace status counts do not match decoded phases: accepted=%d/%d rejected=%d/%d items=%d",
				accepted, decodedAccepted, rejected, decodedRejected, len(items),
			),
		}
	}
	return models.BatchReplaceStatusResult{
		BatchRequestID:  codecs.FormatUint64ID(msg.GetBatchRequestId()),
		AdmissionStatus: admission,
		Items:           items,
		AcceptedCount:   accepted,
		RejectedCount:   rejected,
		AcceptedTsNs:    msg.GetAcceptedTsNs(),
		UpdatedTsNs:     msg.GetUpdatedTsNs(),
	}, nil
}

func batchReplaceAdmissionStatusName(v orderv1.BatchReplaceAdmissionStatus) string {
	switch v {
	case orderv1.BatchReplaceAdmissionStatus_BATCH_REPLACE_ADMISSION_STATUS_ADMITTED:
		return "admitted"
	case orderv1.BatchReplaceAdmissionStatus_BATCH_REPLACE_ADMISSION_STATUS_PARTIALLY_ADMITTED:
		return "partially_admitted"
	case orderv1.BatchReplaceAdmissionStatus_BATCH_REPLACE_ADMISSION_STATUS_REJECTED:
		return "rejected"
	default:
		return ""
	}
}

func batchReplaceItemAdmissionStatusName(v orderv1.BatchReplaceItemAdmissionStatus) string {
	switch v {
	case orderv1.BatchReplaceItemAdmissionStatus_BATCH_REPLACE_ITEM_ADMISSION_STATUS_ADMITTED:
		return "admitted"
	case orderv1.BatchReplaceItemAdmissionStatus_BATCH_REPLACE_ITEM_ADMISSION_STATUS_REJECTED:
		return "rejected"
	default:
		return ""
	}
}

func batchReplacePhaseName(v orderv1.BatchReplacePhase) string {
	switch v {
	case orderv1.BatchReplacePhase_BATCH_REPLACE_PHASE_ADMITTED:
		return "admitted"
	case orderv1.BatchReplacePhase_BATCH_REPLACE_PHASE_WORKING:
		return "working"
	case orderv1.BatchReplacePhase_BATCH_REPLACE_PHASE_REJECTED:
		return "rejected"
	case orderv1.BatchReplacePhase_BATCH_REPLACE_PHASE_TERMINAL:
		return "terminal"
	default:
		return ""
	}
}

// BatchCreateFromProto decodes batch create response.
//
// Per-item results now carry an Accepted/Rejected outcome oneof instead of flat
// status/order_id/code fields.
func BatchCreateFromProto(msg *orderv1.BatchCreateOrdersResponse) (models.BatchCreateOrdersResult, error) {
	results := make([]models.BatchCreateResultItem, 0, len(msg.GetResults()))
	decodedAccepted := 0
	decodedRejected := 0
	for _, item := range msg.GetResults() {
		out := models.BatchCreateResultItem{
			ClientOrderID: item.GetClientOrderId(),
		}
		if accepted := item.GetAccepted(); accepted != nil {
			decodedAccepted++
			out.Status = "accepted"
			out.OrderID = codecs.FormatUint64ID(accepted.GetOrderId())
		} else if rejected := item.GetRejected(); rejected != nil {
			decodedRejected++
			out.Status = "rejected"
			if err := rejected.GetError(); err != nil {
				raw := int32(err.GetCode())
				if raw == 0 {
					out.Code = "ERROR_CODE_UNSPECIFIED"
				} else if name, ok := orderv1.ErrorCode_name[raw]; ok {
					out.Code = name
				} else {
					out.Code = fmt.Sprintf("UNKNOWN_ERROR_CODE(%d)", raw)
				}
				out.RateLimit = RateLimitDetailFromProto(err.GetRateLimit())
			} else {
				out.Code = "ERROR_CODE_UNSPECIFIED"
			}
		} else {
			return models.BatchCreateOrdersResult{}, &sdkerrors.ResponseContractError{Operation: "BatchCreateOrders",
				Msg: "batch create response item has neither accepted nor rejected outcome",
			}
		}
		results = append(results, out)
	}
	accepted := int(msg.GetAcceptedCount())
	rejected := int(msg.GetRejectedCount())
	if accepted != decodedAccepted || rejected != decodedRejected ||
		accepted+rejected != len(results) {
		return models.BatchCreateOrdersResult{}, &sdkerrors.ResponseContractError{Operation: "BatchCreateOrders",
			Msg: fmt.Sprintf(
				"batch create response counts do not match decoded outcomes: accepted=%d/%d rejected=%d/%d results=%d",
				accepted, decodedAccepted, rejected, decodedRejected, len(results),
			),
		}
	}
	return models.BatchCreateOrdersResult{
		Results:       results,
		AcceptedCount: accepted,
		RejectedCount: rejected,
	}, nil
}

// BatchCancelFromProto decodes batch cancel response.
func BatchCancelFromProto(msg *orderv1.BatchCancelOrdersResponse) (models.BatchCancelOrdersResult, error) {
	results := make([]models.BatchCancelResultItem, 0, len(msg.GetResults()))
	decodedAccepted := 0
	decodedRejected := 0
	for _, item := range msg.GetResults() {
		status := batchCancelItemStatusName(item.GetStatus())
		switch status {
		case "accepted":
			decodedAccepted++
		case "rejected":
			decodedRejected++
		default:
			return models.BatchCancelOrdersResult{}, &sdkerrors.ResponseContractError{Operation: "BatchCancelOrders",
				Msg: fmt.Sprintf("batch cancel response has unknown status: %d", item.GetStatus()),
			}
		}
		var rateLimit *models.RateLimitDetail
		if errDetail := item.GetError(); errDetail != nil {
			rateLimit = RateLimitDetailFromProto(errDetail.GetRateLimit())
		}
		results = append(results, models.BatchCancelResultItem{
			Status:        status,
			OrderID:       codecs.FormatUint64ID(item.GetOrderId()),
			ClientOrderID: item.GetClientOrderId(),
			Code:          item.GetCode(),
			RateLimit:     rateLimit,
		})
	}
	accepted := int(msg.GetAcceptedCount())
	rejected := int(msg.GetRejectedCount())
	if accepted != decodedAccepted || rejected != decodedRejected ||
		accepted+rejected != len(results) {
		return models.BatchCancelOrdersResult{}, &sdkerrors.ResponseContractError{Operation: "BatchCancelOrders", Msg: fmt.Sprintf(
			"batch cancel response counts do not match decoded outcomes: accepted=%d/%d rejected=%d/%d results=%d",
			accepted, decodedAccepted, rejected, decodedRejected, len(results),
		)}
	}
	return models.BatchCancelOrdersResult{
		Results:       results,
		AcceptedCount: accepted,
		RejectedCount: rejected,
	}, nil
}

// CancelAllAfterFromProto decodes cancel-all-after response.
func CancelAllAfterFromProto(msg *orderv1.CancelAllAfterResponse) (models.CancelAllAfterResult, error) {
	status := cancelAllAfterStatusName(msg.GetStatus())
	if status == "" {
		return models.CancelAllAfterResult{}, &sdkerrors.ResponseContractError{Operation: "CancelAllAfter",
			Msg: fmt.Sprintf("invalid CancelAllAfter response: unknown status %d", msg.GetStatus()),
		}
	}
	return models.CancelAllAfterResult{
		Status:              status,
		EffectiveTimeoutSec: int(msg.GetEffectiveTimeoutSec()),
		ExpiresAtTsNs:       strconv.FormatUint(msg.GetExpiresAtTsNs(), 10),
	}, nil
}

func cancelAllAfterStatusName(status orderv1.CancelAllAfterResponse_Status) string {
	switch status {
	case orderv1.CancelAllAfterResponse_ARMED:
		return "armed"
	case orderv1.CancelAllAfterResponse_DISABLED:
		return "disabled"
	default:
		return ""
	}
}

func batchCancelItemStatusName(status orderv1.BatchCancelResultItem_Status) string {
	switch status {
	case orderv1.BatchCancelResultItem_ACCEPTED:
		return "accepted"
	case orderv1.BatchCancelResultItem_REJECTED:
		return "rejected"
	default:
		return ""
	}
}
