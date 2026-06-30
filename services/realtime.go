package services

import (
	"github.com/Fabric-Labs/polyester-sdk-go/errors"
	"github.com/Fabric-Labs/polyester-sdk-go/realtime"
)

// RealtimeClient is the Centrifugo client used by service subscribe helpers.
type RealtimeClient = *realtime.Client

func requireRealtime(realtime RealtimeClient) error {
	if realtime == nil {
		return &errors.RealtimeError{Msg: "Realtime client is not configured on this Polyester instance"}
	}
	return nil
}
