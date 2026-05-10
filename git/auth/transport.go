package auth

// Transport describes a transport authentication mechanism.
type Transport interface{ isTransport() }

// SSHKey authenticates with a private key file.
type SSHKey struct {
	User           string
	PrivateKeyPath string
	Passphrase     string
	KnownHostsFile string
}

func (SSHKey) isTransport() {}

// Token authenticates with an HTTP token.
type Token struct {
	Username string
	Value    string
	Provider TokenProvider
}

func (Token) isTransport() {}

// BasicAuth authenticates with a username and password.
type BasicAuth struct {
	Username string
	Password string
}

func (BasicAuth) isTransport() {}

// CredentialHelper requests credentials from a helper program.
type CredentialHelper struct {
	Program string
	Args    []string
}

func (CredentialHelper) isTransport() {}
