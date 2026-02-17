# connect

Connect-Go integration with gokit server — interceptors, error mapping, and service mounting.

## Install

```bash
go get github.com/kbukum/gokit/connect@latest
```

## Quick Start

```go
import (
    "github.com/kbukum/gokit/connect"
    "github.com/kbukum/gokit/server"
    "github.com/kbukum/gokit/logger"
    "connectrpc.com/connect"
)

log := logger.New()
srv := server.New(server.Config{Port: 8080}, log)

// Create Connect service handler
path, handler := userv1connect.NewUserServiceHandler(svc,
    connectrpc.WithInterceptors(goconnect.LoggingInterceptor(log), goconnect.ErrorInterceptor()),
)

// Mount on gokit server
connect.Mount(srv, path, handler)

// Or use the Service abstraction
services := []connect.Service{
    connect.NewService(path, handler),
}
connect.MountServices(srv, services...)
```

## JWT Authentication

The `connect` package provides type-safe JWT authentication interceptors that integrate with `gokit/auth`.

### Setup

```go
import (
    "github.com/kbukum/gokit/auth/jwt"
    "github.com/kbukum/gokit/auth/authctx"
    "github.com/kbukum/gokit/connect"
    gojwt "github.com/golang-jwt/jwt/v5"
)

// 1. Define your claims type
type MyClaims struct {
    gojwt.RegisteredClaims
    UserID   string `json:"user_id"`
    TenantID string `json:"tenant_id"`
    Email    string `json:"email"`
}

// 2. Create JWT service
cfg := &jwt.Config{
    Secret:           "your-secret-key",
    AccessTokenTTL:   15 * time.Minute,
    RefreshTokenTTL:  7 * 24 * time.Hour,
    Method:           jwt.HS256,
}
jwtSvc, err := jwt.NewService(cfg, func() *MyClaims { return &MyClaims{} })
if err != nil {
    log.Fatal(err)
}

// 3. Create service with JWT auth interceptor
path, handler := userv1connect.NewUserServiceHandler(
    svc,
    connect.WithInterceptors(
        connect.JWTAuthInterceptor(jwtSvc),  // ← JWT validation
        connect.LoggingInterceptor(log),
        connect.ErrorInterceptor(),
    ),
)
```

### Using Claims in Handlers

```go
func (s *UserService) GetProfile(
    ctx context.Context,
    req *connect.Request[userv1.GetProfileRequest],
) (*connect.Response[userv1.GetProfileResponse], error) {
    // Retrieve authenticated user's claims
    claims, err := connect.RequireAuth[*MyClaims](ctx)
    if err != nil {
        return nil, err  // Returns CodeUnauthenticated if missing
    }

    userID := claims.UserID
    email := claims.Email

    // ... use claims for authorization, db queries, etc.
    user, err := s.repo.GetUser(ctx, userID)
    if err != nil {
        return nil, err
    }

    return connect.NewResponse(&userv1.GetProfileResponse{
        User: user,
    }), nil
}
```

### Optional Authentication

For endpoints that work both with and without authentication:

```go
// Use OptionalJWTAuthInterceptor
path, handler := userv1connect.NewUserServiceHandler(
    svc,
    connect.WithInterceptors(
        connect.OptionalJWTAuthInterceptor(jwtSvc),  // ← Optional auth
    ),
)

// In handler
func (s *UserService) GetPublicContent(
    ctx context.Context,
    req *connect.Request[userv1.GetContentRequest],
) (*connect.Response[userv1.GetContentResponse], error) {
    // Check if user is authenticated
    claims, ok := connect.GetAuth[*MyClaims](ctx)
    if ok {
        // User authenticated — show personalized content
        content := s.getPersonalizedContent(ctx, claims.UserID)
        return connect.NewResponse(content), nil
    }

    // Anonymous user — show public content
    content := s.getPublicContent(ctx)
    return connect.NewResponse(content), nil
}
```

### Client Usage

```go
// Client-side: Add Bearer token to requests
client := userv1connect.NewUserServiceClient(
    http.DefaultClient,
    "http://localhost:8080",
)

// Add Authorization header
req := connect.NewRequest(&userv1.GetProfileRequest{})
req.Header().Set("Authorization", "Bearer "+accessToken)

resp, err := client.GetProfile(ctx, req)
```

### Complete Example

```go
package main

import (
    "context"
    "log"
    "time"

    "connectrpc.com/connect"
    gojwt "github.com/golang-jwt/jwt/v5"
    
    "github.com/kbukum/gokit/auth/jwt"
    "github.com/kbukum/gokit/auth/authctx"
    goconnect "github.com/kbukum/gokit/connect"
    "github.com/kbukum/gokit/logger"
    "github.com/kbukum/gokit/server"
    
    "yourproject/gen/user/v1/userv1connect"
)

type UserClaims struct {
    gojwt.RegisteredClaims
    UserID string `json:"user_id"`
    Email  string `json:"email"`
}

type UserService struct {
    jwtSvc *jwt.Service[*UserClaims]
}

func (s *UserService) Login(
    ctx context.Context,
    req *connect.Request[userv1.LoginRequest],
) (*connect.Response[userv1.LoginResponse], error) {
    // Validate credentials...
    userID := "user-123"
    email := "user@example.com"

    // Generate access token
    claims := &UserClaims{
        RegisteredClaims: gojwt.RegisteredClaims{
            Subject: userID,
        },
        UserID: userID,
        Email:  email,
    }
    
    accessToken, err := s.jwtSvc.GenerateAccess(claims)
    if err != nil {
        return nil, err
    }

    refreshToken, err := s.jwtSvc.GenerateRefresh(claims)
    if err != nil {
        return nil, err
    }

    return connect.NewResponse(&userv1.LoginResponse{
        AccessToken:  accessToken,
        RefreshToken: refreshToken,
    }), nil
}

func (s *UserService) GetProfile(
    ctx context.Context,
    req *connect.Request[userv1.GetProfileRequest],
) (*connect.Response[userv1.GetProfileResponse], error) {
    // Claims injected by JWTAuthInterceptor
    claims, err := goconnect.RequireAuth[*UserClaims](ctx)
    if err != nil {
        return nil, err
    }

    return connect.NewResponse(&userv1.GetProfileResponse{
        UserId: claims.UserID,
        Email:  claims.Email,
    }), nil
}

func main() {
    log := logger.New()

    // Setup JWT service
    jwtCfg := &jwt.Config{
        Secret:          "your-secret-key",
        AccessTokenTTL:  15 * time.Minute,
        RefreshTokenTTL: 7 * 24 * time.Hour,
    }
    jwtSvc, err := jwt.NewService(jwtCfg, func() *UserClaims { return &UserClaims{} })
    if err != nil {
        log.Fatal("failed to create jwt service", map[string]interface{}{"error": err})
    }

    svc := &UserService{jwtSvc: jwtSvc}

    // Public endpoints (no auth required)
    publicPath, publicHandler := userv1connect.NewUserServiceHandler(
        svc,
        connect.WithInterceptors(
            goconnect.LoggingInterceptor(log),
            goconnect.ErrorInterceptor(),
        ),
    )

    // Protected endpoints (auth required)
    protectedPath, protectedHandler := userv1connect.NewUserServiceHandler(
        svc,
        connect.WithInterceptors(
            goconnect.JWTAuthInterceptor(jwtSvc),  // ← Require auth
            goconnect.LoggingInterceptor(log),
            goconnect.ErrorInterceptor(),
        ),
    )

    // Start server
    srv := server.New(server.Config{Port: 8080}, log)
    goconnect.Mount(srv, publicPath, publicHandler)
    goconnect.Mount(srv, protectedPath, protectedHandler)
    
    if err := srv.Start(); err != nil {
        log.Fatal("server failed", map[string]interface{}{"error": err})
    }
}
```



## Key Types & Functions

| Symbol | Description |
|---|---|
| `Config` | SendMaxBytes, ReadMaxBytes, Enabled |
| `Service` | Interface — `Path() string`, `Handler() http.Handler` |
| `NewService(path, handler)` | Create a Service from path and handler |
| `Mount(srv, path, handler)` | Mount a single Connect handler on gokit server |
| `MountServices(srv, ...Service)` | Mount multiple services at once |
| `LoggingInterceptor(log)` | Log RPC calls with duration and status |
| `ErrorInterceptor()` | Convert `*AppError` to Connect errors |
| `AuthInterceptor(validateToken)` | Bearer token validation interceptor |
| `ToConnectError(appErr)` | `*AppError` → `*connect.Error` |
| `FromConnectError(err)` | `*connect.Error` → `*AppError` |

---

[← Back to main gokit README](../README.md)
