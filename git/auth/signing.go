package auth

// Signing describes commit or tag signing configuration.
type Signing interface{ isSigning() }

// GPGSign configures GPG signing.
type GPGSign struct {
	KeyID string
}

func (GPGSign) isSigning() {}

// SSHSign configures SSH signing.
type SSHSign struct {
	KeyPath string
}

func (SSHSign) isSigning() {}
