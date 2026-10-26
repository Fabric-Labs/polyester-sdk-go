package realtime

import (
	"context"
	"net/url"
	"strings"

	"github.com/Fabric-Labs/polyester-sdk-go/codecs"
	sdkerrors "github.com/Fabric-Labs/polyester-sdk-go/errors"
	"github.com/Fabric-Labs/polyester-sdk-go/models"
)

// OrderStreamEvent is one delivery on an account's private order stream. It
// is one of OrderStreamConnected, OrderStreamDisconnected, OrderSnapshot,
// OrderUnit, OrderStale and OrderStreamError. Events are shared by every
// subscription of the same account: treat them as read-only.
type OrderStreamEvent interface {
	isOrderStreamEvent()
}

// OrderStreamConnected reports that the server confirmed the subscription.
// It comes again after each reconnect or resubscribe.
type OrderStreamConnected struct{}

// OrderStreamDisconnected reports that the subscription dropped. The stream
// reconnects by itself and resumes from its last position, unless the
// subscription ended: then the events channel closes and Err reports why.
type OrderStreamDisconnected struct{}

// OrderSnapshot is the account's full live view: open orders, accepted
// orders not yet on the book, and recently ended orders still retained.
// Replace local state with it. A snapshot also follows a resync, when the
// server could not resume from the last position; the events between the two
// states are not delivered.
type OrderSnapshot struct {
	Orders []models.Order
}

// OrderUnit is one change, applied whole.
type OrderUnit struct {
	// Orders are the changed orders, complete. Replace each by OrderID; a
	// higher Version is always newer and equal versions are identical.
	Orders []models.Order
	// RemovedOrderIDs left the live view. A removal is not a cancellation:
	// ended orders stay readable from order history.
	RemovedOrderIDs []string
	// Events are what caused the change, in order: order events and answers
	// to commands that changed no order.
	Events []models.OrderUnitEvent
}

// OrderStale reports that the account's live view is temporarily behind
// (Stale true, with the server's Reason such as "view_behind") or caught up
// again (Stale false). The subscription stays open and recovers by itself.
type OrderStale struct {
	Stale  bool
	Reason string
}

// OrderStreamError reports an error. The stream keeps running unless the
// events channel closes after it; Err then returns the terminal error.
type OrderStreamError struct {
	Err error
}

func (OrderStreamConnected) isOrderStreamEvent()    {}
func (OrderStreamDisconnected) isOrderStreamEvent() {}
func (OrderSnapshot) isOrderStreamEvent()           {}
func (OrderUnit) isOrderStreamEvent()               {}
func (OrderStale) isOrderStreamEvent()              {}
func (OrderStreamError) isOrderStreamEvent()        {}

// SubscribeOrders streams an account's orders over the private WebSocket
// (/v1/ws/private). accountID is the account's public ID.
//
// It returns at once; the first event is OrderStreamConnected, then an
// OrderSnapshot, then OrderUnit changes. One connection carries every account
// of this client, and subscriptions of the same account share one server
// subscription: a later one starts from the current state. Sequence gaps,
// expiring tokens and reconnects resume from the last applied position.
//
// The subscription ends, closing Messages with Err set, when the server
// refuses it (for example FORBIDDEN), when the API key is refused, on close
// codes 4400 and 4403, and when the consumer falls behind the bounded queue
// (QueueOverflowError). Errors arrive as OrderStreamError events, except a
// queue overflow, and every error also goes to the SetOnError callback.
// Close or cancel ctx to stop.
func (c *Client) SubscribeOrders(ctx context.Context, accountID string) (*Subscription[OrderStreamEvent], error) {
	if c == nil {
		return nil, &sdkerrors.RealtimeError{Msg: "Realtime client is not configured"}
	}
	if c.credentials == nil {
		return nil, &sdkerrors.AuthError{Msg: "Cannot subscribe to the private order stream without API-key credentials"}
	}
	account, err := codecs.IDToInt(accountID, "account_id")
	if err != nil {
		return nil, err
	}
	if account == 0 {
		return nil, &sdkerrors.ValidationError{Msg: "account_id must be a non-zero public ID"}
	}
	wsURL, err := privateWSURL(c.apiURL)
	if err != nil {
		return nil, err
	}
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return nil, &sdkerrors.RealtimeError{Msg: "Realtime client is closed"}
	}
	if c.orders == nil {
		c.orders = newPrivateOrderStream(privateStreamConfig{
			url: wsURL,
			fetchConnectToken: func(ctx context.Context) (string, error) {
				return fetchConnectionToken(ctx, c.http, c.credentials, c.apiURL)
			},
			fetchSubscribeToken: func(ctx context.Context, channel string) (string, error) {
				return fetchSubscriptionToken(ctx, c.http, c.credentials, c.apiURL, channel)
			},
			maxQueue: c.maxQueue,
			timing:   c.privateTiming,
		})
	}
	stream := c.orders
	c.mu.Unlock()

	sub := stream.subscribe(ctx, account)
	untrack := c.trackSubscription(func() { sub.Close() })
	sub.mu.Lock()
	sub.untrack = untrack
	sub.mu.Unlock()
	return sub, nil
}

// privateWSURL is the API URL with https → wss (http → ws) and the private
// WebSocket path.
func privateWSURL(apiURL string) (string, error) {
	if apiURL == "" {
		return "", &sdkerrors.RealtimeError{Msg: "The private order stream requires api_url"}
	}
	u, err := url.Parse(strings.TrimRight(apiURL, "/"))
	if err != nil {
		return "", &sdkerrors.ValidationError{Msg: "api_url is not a valid URL: " + err.Error()}
	}
	switch u.Scheme {
	case "https":
		u.Scheme = "wss"
	case "http":
		u.Scheme = "ws"
	default:
		return "", &sdkerrors.ValidationError{Msg: "api_url must be an http or https URL"}
	}
	u.Path = strings.TrimRight(u.Path, "/") + "/v1/ws/private"
	u.RawPath = ""
	u.RawQuery = ""
	u.ForceQuery = false
	u.Fragment = ""
	return u.String(), nil
}
