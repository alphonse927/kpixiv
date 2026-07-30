package pixiv

import "context"

// AuthProvider adapts *Client to satisfy auth.Provider.
type AuthProvider struct {
	Client *Client
}

func (p *AuthProvider) BeginLogin() (string, string, error) {
	flow, err := p.Client.BeginLogin()
	if err != nil {
		return "", "", err
	}
	return flow.URL, flow.CodeVerifier, nil
}

func (p *AuthProvider) FinishLogin(ctx context.Context, verifier, code string) (string, error) {
	state, err := p.Client.FinishLogin(ctx, verifier, code)
	if err != nil {
		return "", err
	}
	return state.UserName, nil
}
