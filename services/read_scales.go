package services

import (
	"context"

	"github.com/Fabric-Labs/polyester-sdk-go/catalogs"
	"github.com/Fabric-Labs/polyester-sdk-go/codecs/decode"
)

// readQuantityScales returns the base quantity scales an order, trade or
// trigger read attaches to its quantities. Like any call that scales its
// results from the catalogs, it first waits for a load the client began at
// construction; a failed load leaves the quantities unscaled instead of
// failing the read. Without catalogs it returns nil.
func readQuantityScales(ctx context.Context, cats *catalogs.Manager) (decode.BaseQuantityScales, error) {
	if cats == nil {
		return nil, nil
	}
	if err := awaitCatalogs(ctx, cats, false); err != nil {
		return nil, err
	}
	return cats.BaseQuantityScaleForSymbolID, nil
}

// scaledRead wraps a read decoder so its result carries the catalogs' base
// quantity scales.
func scaledRead[Resp any, Out any](decodeFn func(Resp) Out, scale func(Out, decode.BaseQuantityScales) Out, scales decode.BaseQuantityScales) func(Resp) Out {
	return func(msg Resp) Out {
		return scale(decodeFn(msg), scales)
	}
}

// streamQuantityScales returns the base quantity scales a stream attaches to
// what it delivers, as loaded when each message arrives; nil without
// catalogs. A stream does not wait for the catalogs.
func streamQuantityScales(cats *catalogs.Manager) decode.BaseQuantityScales {
	if cats == nil {
		return nil
	}
	return cats.BaseQuantityScaleForSymbolID
}
