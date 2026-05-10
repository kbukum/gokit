package embedded

import (
	"fmt"

	gittransport "github.com/go-git/go-git/v5/plumbing/transport"
	httpauth "github.com/go-git/go-git/v5/plumbing/transport/http"
	sshauth "github.com/go-git/go-git/v5/plumbing/transport/ssh"

	"github.com/kbukum/gokit/git/auth"
	giterr "github.com/kbukum/gokit/git/internal/giterr"
)

func transportAuthMethod(cfg auth.Transport) (gittransport.AuthMethod, error) {
	switch v := cfg.(type) {
	case nil:
		return nil, nil //nolint:nilnil // nil is the go-git convention for "no authentication"
	case auth.Token:
		return tokenAuth(v)
	case *auth.Token:
		if v == nil {
			return nil, nil //nolint:nilnil // nil pointer treated as absent credential
		}
		return tokenAuth(*v)
	case auth.BasicAuth:
		return &httpauth.BasicAuth{Username: v.Username, Password: v.Password}, nil
	case *auth.BasicAuth:
		if v == nil {
			return nil, nil //nolint:nilnil // nil pointer treated as absent credential
		}
		return &httpauth.BasicAuth{Username: v.Username, Password: v.Password}, nil
	case auth.SSHKey:
		return sshAuth(v)
	case *auth.SSHKey:
		if v == nil {
			return nil, nil //nolint:nilnil // nil pointer treated as absent credential
		}
		return sshAuth(*v)
	case auth.CredentialHelper, *auth.CredentialHelper:
		return nil, giterr.CLINotImplemented()
	default:
		return nil, giterr.InvalidTransport(fmt.Sprintf("%T", cfg))
	}
}

func tokenAuth(cfg auth.Token) (gittransport.AuthMethod, error) {
	password := cfg.Value
	if password == "" && cfg.Provider != nil {
		resolved, err := cfg.Provider.Token()
		if err != nil {
			return nil, giterr.Network(err)
		}
		password = resolved
	}
	username := cfg.Username
	if username == "" {
		username = "git"
	}
	return &httpauth.BasicAuth{Username: username, Password: password}, nil
}

func sshAuth(cfg auth.SSHKey) (gittransport.AuthMethod, error) {
	if cfg.PrivateKeyPath == "" {
		return nil, giterr.InvalidPath(cfg.PrivateKeyPath)
	}
	user := cfg.User
	if user == "" {
		user = "git"
	}
	authMethod, err := sshauth.NewPublicKeysFromFile(user, cfg.PrivateKeyPath, cfg.Passphrase)
	if err != nil {
		return nil, giterr.Internal(err)
	}
	return authMethod, nil
}
