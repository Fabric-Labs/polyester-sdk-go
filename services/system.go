package services

import (
	"context"
	"iter"
	"time"

	"github.com/Fabric-Labs/polyester-sdk-go/codecs"
	"github.com/Fabric-Labs/polyester-sdk-go/codecs/decode"
	"github.com/Fabric-Labs/polyester-sdk-go/errors"
	systemv1 "github.com/Fabric-Labs/polyester-sdk-go/gen/system/v1"
	"github.com/Fabric-Labs/polyester-sdk-go/gen/system/v1/systemv1connect"
	"github.com/Fabric-Labs/polyester-sdk-go/models"
	"github.com/Fabric-Labs/polyester-sdk-go/transport"
)

// SystemService reads public facts about the environment the client points
// at. It needs no credentials; Client.GetVersion is its entry point.
type SystemService struct {
	transport *transport.Factory
}

func NewSystemService(factory *transport.Factory) *SystemService {
	return &SystemService{transport: factory}
}

func (s *SystemService) client() systemv1connect.SystemServiceClient {
	return systemv1connect.NewSystemServiceClient(s.transport.HTTP, s.transport.Config.APIURL, s.transport.ConnectOptions(false)...)
}

// GetVersion reads the build the environment serves. It is public: the
// request is never signed.
func (s *SystemService) GetVersion(ctx context.Context) (models.ServerVersion, error) {
	return UnaryPublic(ctx, s.transport, s.client().GetVersion, &systemv1.GetVersionRequest{}, func(msg *systemv1.GetVersionResponse) models.ServerVersion {
		return models.ServerVersion{
			Version:     msg.GetVersion(),
			Commit:      msg.GetCommit(),
			Environment: msg.GetEnvironment(),
		}
	})
}

// GetServerTime reads the server's clock, the one that checks the timestamp
// of requests signed with an API key, and estimates this host's offset from
// it. It is public: the request is never signed, and the answer is never
// cached. A signed request is accepted only while its timestamp is at most
// 1000 ms ahead of the server time and at most its receive window behind
// it. A reply without a positive server time returns a
// *errors.ResponseContractError.
func (s *SystemService) GetServerTime(ctx context.Context) (models.ServerTime, error) {
	sent := time.Now()
	return UnaryPublicDecoded(ctx, s.transport, s.client().GetTime, &systemv1.GetTimeRequest{}, func(msg *systemv1.GetTimeResponse) (models.ServerTime, error) {
		received := time.Now()
		if msg.GetServerTimeMs() <= 0 {
			return models.ServerTime{}, &errors.ResponseContractError{Operation: "GetTime", Msg: "missing server_time_ms"}
		}
		server := time.UnixMilli(msg.GetServerTimeMs()).UTC()
		roundTrip := received.Sub(sent)
		return models.ServerTime{Time: server, Offset: server.Sub(sent.Add(roundTrip / 2)), RoundTrip: roundTrip}, nil
	})
}

// ListStatusEvents returns one page of the environment's scheduled
// maintenance and incidents, the latest start first. It is public: the
// request is never signed, and answers can be up to 10 seconds old. Empty
// opts.States lists current and upcoming events; the server applies that
// default. An unknown state or kind label is a *errors.ValidationError and
// nothing is sent.
func (s *SystemService) ListStatusEvents(ctx context.Context, opts models.ListStatusEventsOptions) (models.StatusEventsPage, error) {
	req, err := listStatusEventsRequest(opts)
	if err != nil {
		return models.StatusEventsPage{}, err
	}
	return UnaryPublicDecoded(ctx, s.transport, s.client().ListStatusEvents, req, decode.StatusEventsPageFromProto)
}

// StatusEvents iterates over every status event matching opts from
// opts.PageToken (or the latest start), fetching pages as needed with the
// same States and Kinds. Iteration stops at the first error. An event that
// changes state between two pages can be missing or come twice: match events
// by ID.
func (s *SystemService) StatusEvents(ctx context.Context, opts models.ListStatusEventsOptions) iter.Seq2[models.StatusEvent, error] {
	return paginate(opts.PageToken, func(token string) ([]models.StatusEvent, string, error) {
		opts.PageToken = token
		page, err := s.ListStatusEvents(ctx, opts)
		return page.Events, page.NextPageToken, err
	})
}

func listStatusEventsRequest(opts models.ListStatusEventsOptions) (*systemv1.ListStatusEventsRequest, error) {
	limit, err := PaginationLimit(opts.Limit, "limit")
	if err != nil {
		return nil, err
	}
	states, err := changelogEnums[systemv1.StatusEventState](opts.States, systemv1.StatusEventState_value, "states")
	if err != nil {
		return nil, err
	}
	kinds, err := changelogEnums[systemv1.StatusEventKind](opts.Kinds, systemv1.StatusEventKind_value, "kinds")
	if err != nil {
		return nil, err
	}
	req := &systemv1.ListStatusEventsRequest{States: states, Kinds: kinds, Limit: limit, PageToken: opts.PageToken}
	if err := codecs.CheckPageLimit(req); err != nil {
		return nil, err
	}
	return req, nil
}
