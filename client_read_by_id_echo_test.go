package polyester

import (
	"context"
	stderrors "errors"
	"io"
	"net/http"
	"sync"
	"testing"

	"github.com/Fabric-Labs/polyester-sdk-go/auth"
	"github.com/Fabric-Labs/polyester-sdk-go/codecs"
	sdkerrors "github.com/Fabric-Labs/polyester-sdk-go/errors"
	authv1 "github.com/Fabric-Labs/polyester-sdk-go/gen/auth/v1"
	lifecyclev1 "github.com/Fabric-Labs/polyester-sdk-go/gen/chain/lifecycle/v1"
	changelogv1 "github.com/Fabric-Labs/polyester-sdk-go/gen/changelog/v1"
	orderv1 "github.com/Fabric-Labs/polyester-sdk-go/gen/orders/v1"
	transferv1 "github.com/Fabric-Labs/polyester-sdk-go/gen/transfer/v1"
	triggersv1 "github.com/Fabric-Labs/polyester-sdk-go/gen/triggers/v1"
	"github.com/Fabric-Labs/polyester-sdk-go/internal/hardening"
	"github.com/Fabric-Labs/polyester-sdk-go/models"
	"google.golang.org/protobuf/proto"
)

// echoClient answers every Connect call with the next of replies, encoded as
// the proto reply of a unary call, and records each request body.
func echoClient(t *testing.T, replies ...proto.Message) (*Client, func() [][]byte) {
	t.Helper()
	var mu sync.Mutex
	var bodies [][]byte
	srv := hardening.NewTestServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		mu.Lock()
		bodies = append(bodies, body)
		if len(replies) == 0 {
			mu.Unlock()
			http.NotFound(w, r)
			return
		}
		reply := replies[0]
		replies = replies[1:]
		mu.Unlock()
		out, err := proto.Marshal(reply)
		if err != nil {
			t.Errorf("marshal reply: %v", err)
		}
		w.Header().Set("Content-Type", "application/proto")
		_, _ = w.Write(out)
	}))
	t.Cleanup(srv.Close)
	keypair := auth.GenerateEd25519Keypair()
	client, err := New(Config{APIURL: srv.URL, APIKeyID: "ak_echo", APIPrivateKey: keypair.SecretKeyHex, SkipCatalogHydration: true})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = client.Close() })
	return client, func() [][]byte {
		mu.Lock()
		defer mu.Unlock()
		return bodies
	}
}

// A read by ID must answer the object it asked for: a reply naming another
// one breaks the contract, however well formed it is, and the reply naming
// the requested one decodes.
func TestReadByIDRefusesAReplyForAnotherObject(t *testing.T) {
	ctx := context.Background()
	orderReply := func(orderID uint64, clientOrderID string) proto.Message {
		return &orderv1.GetOrderResponse{Order: &orderv1.Order{OrderId: orderID, ClientOrderId: clientOrderID}}
	}
	flow := func(flowID string) *lifecyclev1.FlowDetailView {
		return &lifecyclev1.FlowDetailView{Summary: &lifecyclev1.FlowSummaryView{
			FlowId: flowID, FlowKind: lifecyclev1.FlowKind_KIND_TRANSFER, Participation: lifecyclev1.FlowParticipation_PARTICIPATION_OWNER,
		}}
	}
	subaccountID := codecs.FormatID(9)
	const keyID = "ak_0123456789abcdef0123456789abcdef"
	cases := []struct {
		name         string
		wrong, right proto.Message
		read         func(*Client) error
	}{
		{"GetBatchReplaceStatus",
			&orderv1.GetBatchReplaceStatusResponse{BatchRequestId: 8, AdmissionStatus: orderv1.BatchReplaceAdmissionStatus_BATCH_REPLACE_ADMISSION_STATUS_ADMITTED},
			&orderv1.GetBatchReplaceStatusResponse{BatchRequestId: 7, AdmissionStatus: orderv1.BatchReplaceAdmissionStatus_BATCH_REPLACE_ADMISSION_STATUS_ADMITTED},
			func(c *Client) error {
				_, err := c.Orders.GetBatchReplaceStatus(ctx, nil, codecs.FormatID(7), nil)
				return err
			}},
		{"GetOrder by order ID", orderReply(43, "cid-1"), orderReply(42, "cid-1"),
			func(c *Client) error {
				_, err := c.Orders.Get(ctx, nil, models.OrderKeyByID(codecs.FormatID(42)), nil, false, false)
				return err
			}},
		{"GetOrder by client order ID", orderReply(42, "cid-2"), orderReply(42, "cid-1"),
			func(c *Client) error {
				_, err := c.Orders.Get(ctx, nil, models.OrderKeyByClientID("cid-1"), nil, false, false)
				return err
			}},
		{"GetTrigger",
			&triggersv1.GetTriggerResponse{Trigger: &triggersv1.Trigger{TriggerId: 6}},
			&triggersv1.GetTriggerResponse{Trigger: &triggersv1.Trigger{TriggerId: 5}},
			func(c *Client) error {
				_, err := c.Triggers.Get(ctx, nil, codecs.FormatID(5), nil)
				return err
			}},
		{"GetDustConversion",
			&transferv1.GetDustConversionResponse{Conversion: &transferv1.DustConversion{ConversionId: "124"}},
			&transferv1.GetDustConversionResponse{Conversion: &transferv1.DustConversion{ConversionId: "123"}},
			func(c *Client) error {
				_, err := c.DustConversions.Get(ctx, "123", nil, nil)
				return err
			}},
		{"GetFlowById",
			&lifecyclev1.GetFlowResponse{Flow: flow("flow_b")},
			&lifecyclev1.GetFlowResponse{Flow: flow("flow_a")},
			func(c *Client) error {
				id := "flow_a"
				_, err := c.Lifecycle.GetFlow(ctx, nil, &id)
				return err
			}},
		{"GetAccountFlow",
			&lifecyclev1.GetAccountFlowResponse{Flow: flow("flow_b")},
			&lifecyclev1.GetAccountFlowResponse{Flow: flow("flow_a")},
			func(c *Client) error {
				_, err := c.Lifecycle.GetAccountFlow(ctx, "flow_a", nil, nil)
				return err
			}},
		{"GetSubaccount",
			&authv1.GetSubaccountResponse{Subaccount: &authv1.Subaccount{Id: 10}},
			&authv1.GetSubaccountResponse{Subaccount: &authv1.Subaccount{Id: 9}},
			func(c *Client) error {
				_, err := c.SubAccounts.Get(ctx, nil, &subaccountID, false, false, false, false, false, "")
				return err
			}},
		{"GetApiKey",
			&authv1.GetApiKeyResponse{ApiKey: &authv1.ApiKey{KeyId: "ak_fedcba9876543210fedcba9876543210"}},
			&authv1.GetApiKeyResponse{ApiKey: &authv1.ApiKey{KeyId: keyID}},
			func(c *Client) error {
				_, err := c.APIKeys.Get(ctx, keyID)
				return err
			}},
		{"GetPost",
			&changelogv1.GetPostResponse{Post: &changelogv1.Post{Summary: &changelogv1.PostSummary{Slug: "2026-09-25"}}},
			&changelogv1.GetPostResponse{Post: &changelogv1.Post{Summary: &changelogv1.PostSummary{Slug: "2026-09-24"}}},
			func(c *Client) error {
				_, err := c.Changelog.GetPost(ctx, "2026-09-24")
				return err
			}},
		{"GetDeploymentGroup",
			&changelogv1.GetDeploymentGroupResponse{Group: &changelogv1.DeploymentGroup{Summary: &changelogv1.DeploymentGroupSummary{Slug: "testnet-v0.92.8"}}},
			&changelogv1.GetDeploymentGroupResponse{Group: &changelogv1.DeploymentGroup{Summary: &changelogv1.DeploymentGroupSummary{Slug: "testnet-v0.92.9"}}},
			func(c *Client) error {
				_, err := c.Changelog.GetDeploymentGroup(ctx, "testnet-v0.92.9")
				return err
			}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			client, _ := echoClient(t, tc.wrong, tc.right)
			var contractErr *sdkerrors.ResponseContractError
			if err := tc.read(client); !stderrors.As(err, &contractErr) {
				t.Fatalf("reply for another object: err = %v, want a ResponseContractError", err)
			}
			if err := tc.read(client); err != nil {
				t.Fatalf("reply for the requested object: err = %v", err)
			}
		})
	}
}

// The server reads an API key ID with surrounding whitespace trimmed, so the
// reply naming the trimmed key answers a padded request.
func TestGetApiKeyAcceptsTheTrimmedKey(t *testing.T) {
	const keyID = "ak_0123456789abcdef0123456789abcdef"
	client, _ := echoClient(t, &authv1.GetApiKeyResponse{ApiKey: &authv1.ApiKey{KeyId: keyID}})
	got, err := client.APIKeys.Get(context.Background(), " "+keyID+"\n")
	if err != nil || got == nil || got.KeyID != keyID {
		t.Fatalf("Get = %+v, %v", got, err)
	}
}
