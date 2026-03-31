package providers

// Google OAuth2/OIDC endpoint defaults.
const (
	GoogleAuthEndpoint     = "https://accounts.google.com/o/oauth2/v2/auth"
	GoogleTokenEndpoint    = "https://oauth2.googleapis.com/token"
	GoogleUserInfoEndpoint = "https://www.googleapis.com/oauth2/v2/userinfo"
)

// GoogleDefaultScopes are the standard OIDC scopes for Google login.
var GoogleDefaultScopes = []string{"openid", "email", "profile"}

// NewGoogle creates a Google OAuth2/OIDC provider.
// All fields have sensible defaults; override any by setting them in cfg.
func NewGoogle(cfg ProviderConfig) *GenericProvider {
	return NewGeneric(GenericConfig{
		ProviderConfig:   withDefaultScopes(cfg, GoogleDefaultScopes...),
		ProviderName:     "google",
		Label:            "Google",
		Type:             "identity",
		AuthEndpoint:     GoogleAuthEndpoint,
		TokenEndpoint:    GoogleTokenEndpoint,
		UserInfoEndpoint: GoogleUserInfoEndpoint,
		AuthExtraParams: map[string]string{
			"access_type": "offline",
			"prompt":      "consent",
		},
		UserInfo: UserInfoMapper{
			SubjectKey:       "id",
			EmailKey:         "email",
			EmailVerifiedKey: "verified_email",
			NameKey:          "name",
			GivenNameKey:     "given_name",
			FamilyNameKey:    "family_name",
			PictureKey:       "picture",
			LocaleKey:        "locale",
		},
	})
}
