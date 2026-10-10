package jwt

import gojwt "github.com/golang-jwt/jwt/v5"

// Registered carries the registered claims every token needs. Application claims embed it by value:
//
//	type MyClaims struct {
//	    jwt.Registered
//	    Scope string `json:"scope"`
//	}
type Registered struct{ gojwt.RegisteredClaims }

func (r *Registered) registered() *gojwt.RegisteredClaims { return &r.RegisteredClaims }

// Claims is implemented by pointers to structs that embed [Registered]. The service fills and checks the registered
// claims through it without reflection.
type Claims interface {
	comparable
	gojwt.Claims
	registered() *gojwt.RegisteredClaims
}
