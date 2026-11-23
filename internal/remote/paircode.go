package remote

import (
	"context"
	"net/http"
	"pairshell/internal/paircode"
)

func (m *Merchant) PairCode(ctx context.Context) (string, <-chan error, error) {
	return paircode.Publish(ctx, m.c.base, m.Invite.SessionID, m.c.token, m.Invite.URL(), m.c.client)
}

// ConnectCode establishes an invitation exchange only. Connect still verifies
// peer identity and the customer must consent before any SSH operation.
func ConnectCode(ctx context.Context, base, code, deploy string, client *http.Client) (*Merchant, error) {
	var m *Merchant
	err := paircode.SendWith(ctx, base, code, client, func(permit string) (string, error) {
		credential := deploy
		if credential == "" {
			credential = permit
		}
		var err error
		m, err = Create(ctx, base, credential, client)
		if err != nil {
			return "", err
		}
		return m.Invite.URL(), nil
	})
	if err != nil {
		if m != nil {
			m.c.end()
		}
		return nil, err
	}
	return m, nil
}
