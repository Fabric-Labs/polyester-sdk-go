package services

import (
	"context"

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
