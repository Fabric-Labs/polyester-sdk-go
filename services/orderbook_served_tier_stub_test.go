package services

import (
	"io"
	"net/http"

	ob "github.com/Fabric-Labs/polyester-sdk-go/gen/orderbook/v1"
	typev1 "github.com/Fabric-Labs/polyester-sdk-go/gen/polyester/type/v1"
	"google.golang.org/protobuf/proto"
)

// requestedTier reads the tier a GetOrderBook request asks for, as a stub
// answering like the contract names it served: DEPTH_50 when it is unset.
func requestedTier(r *http.Request) typev1.DepthTier {
	body, _ := io.ReadAll(r.Body)
	req := new(ob.GetOrderBookRequest)
	if proto.Unmarshal(body, req) != nil || req.GetDepth() == typev1.DepthTier_DEPTH_UNSPECIFIED {
		return typev1.DepthTier_DEPTH_50
	}
	return req.GetDepth()
}
