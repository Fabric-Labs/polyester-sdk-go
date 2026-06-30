package services

import (
	"context"
	"strings"

	"github.com/Fabric-Labs/polyester-sdk-go/codecs/decode"
	"github.com/Fabric-Labs/polyester-sdk-go/errors"
	authv1 "github.com/Fabric-Labs/polyester-sdk-go/gen/auth/v1"
	"github.com/Fabric-Labs/polyester-sdk-go/gen/auth/v1/authv1connect"
	"github.com/Fabric-Labs/polyester-sdk-go/models"
	"github.com/Fabric-Labs/polyester-sdk-go/transport"
)

type ResolveService struct {
	transport *transport.Factory
}

func NewResolveService(factory *transport.Factory) *ResolveService {
	return &ResolveService{transport: factory}
}

func (s *ResolveService) client() authv1connect.ResolveServiceClient {
	return authv1connect.NewResolveServiceClient(s.transport.HTTP, s.transport.Config.APIURL, s.transport.ConnectOptions(true)...)
}

func (s *ResolveService) ResolveAccount(ctx context.Context, query, hint string, includeSubaccounts bool) (models.ResolvedAccountsList, error) {
	aliases := map[string]authv1.ResolveHint{
		"auto": authv1.ResolveHint_RESOLVE_HINT_UNSPECIFIED, "unspecified": authv1.ResolveHint_RESOLVE_HINT_UNSPECIFIED,
		"username": authv1.ResolveHint_USERNAME, "id": authv1.ResolveHint_ID, "smart_account": authv1.ResolveHint_SMART_ACCOUNT,
	}
	key := strings.ToLower(hint)
	h, ok := aliases[key]
	if !ok {
		name := strings.ToUpper(strings.ReplaceAll(key, "-", "_"))
		if v, ok := authv1.ResolveHint_value[name]; ok {
			h = authv1.ResolveHint(v)
		} else if v, ok := authv1.ResolveHint_value["RESOLVE_HINT_"+name]; ok {
			h = authv1.ResolveHint(v)
		} else {
			return models.ResolvedAccountsList{}, &errors.ValidationError{Msg: "hint must be one of 'auto', 'username', 'id', or 'smart_account'"}
		}
	}
	req := &authv1.ResolveAccountRequest{Query: query, Hint: h, IncludeSubaccounts: includeSubaccounts}
	return UnaryAuth(ctx, s.transport, s.client().ResolveAccount, req, decode.ResolvedAccountsFromProto)
}
