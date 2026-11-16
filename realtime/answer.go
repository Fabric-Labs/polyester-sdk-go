package realtime

import (
	"net/http"

	sdkerrors "github.com/Fabric-Labs/polyester-sdk-go/errors"
	"github.com/Fabric-Labs/polyester-sdk-go/wire"
)

// upgradeAnswerError keeps a refused WebSocket upgrade's answer, its HTTP
// status and the delay it asked for, on err. A dial that got no answer, such
// as a refused connection, has no resp and carries none.
func upgradeAnswerError(err error, resp *http.Response) error {
	if resp != nil {
		sdkerrors.AttachAnswer(err, resp.StatusCode, wire.RetryAfterHeaderSeconds(resp.Header))
	}
	return err
}
