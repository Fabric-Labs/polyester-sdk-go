package hardening_test

import (
	orderbookv1 "github.com/Fabric-Labs/polyester-sdk-go/gen/orderbook/v1"
	typev1 "github.com/Fabric-Labs/polyester-sdk-go/gen/polyester/type/v1"
)

// servedTier is the tier a contract server names as served for req:
// the requested tier, DEPTH_50 when it is unset.
func servedTier(req *orderbookv1.GetOrderBookRequest) typev1.DepthTier {
	if req.GetDepth() == typev1.DepthTier_DEPTH_UNSPECIFIED {
		return typev1.DepthTier_DEPTH_50
	}
	return req.GetDepth()
}
