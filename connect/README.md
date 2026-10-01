# connect

Connect-Go integration for gokit — server-side interceptors, error mapping, service mounting, and client utilities.

## Install

```bash
go get github.com/kbukum/gokit/connect@latest
```

## Quick Start — Server

```go
import (
    goconnect "github.com/kbukum/gokit/connect"
    "github.com/kbukum/gokit/logging"
    "connectrpc.com/connect"
)

log := logging.NewDefault("my-service")

// Create Connect service handler with interceptors
path, handler := userv1connect.NewUserServiceHandler(svc,
    connect.WithInterceptors(goconnect.LoggingInterceptor(log), goconnect.NormalizingInterceptor(log)),
)

// Mount on any server implementing HandlerMounter (e.g. gokit/server.Server)
goconnect.Mount(srv, path, handler)
```

## Quick Start — Client

```go
import (
    "github.com/kbukum/gokit/connect/client"
)

cfg := client.Config{BaseURL: "http://localhost:8080"}
httpClient, err := client.NewHTTPClient(cfg)
if err != nil {
    return err
}
svcClient := userv1connect.NewUserServiceClient(httpClient, cfg.BaseURL)

// For bidi streaming, use gRPC protocol:
cfg := client.Config{BaseURL: "http://localhost:8080", Protocol: client.ProtocolGRPC}
httpClient, err := client.NewHTTPClient(cfg)
opts := client.ClientOptions(cfg)
svcClient := userv1connect.NewUserServiceClient(httpClient, cfg.BaseURL, opts...)
```

The client uses native HTTP/2 transport controls. Enabled TLS requires `https://`; the default h2c mode requires `http://`. Each request, including redirects, is checked before dialing, so neither mode silently falls back to HTTP/1. Call `httpClient.CloseIdleConnections()` when its owner shuts down.

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
        connect.NormalizingInterceptor(log),
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
    "github.com/kbukum/gokit/logging"
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
    log := logging.NewDefault("my-service")

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
            goconnect.NormalizingInterceptor(log),
        ),
    )

    // Protected endpoints (auth required)
    protectedPath, protectedHandler := userv1connect.NewUserServiceHandler(
        svc,
        connect.WithInterceptors(
            goconnect.JWTAuthInterceptor(jwtSvc),  // ← Require auth
            goconnect.LoggingInterceptor(log),
            goconnect.NormalizingInterceptor(log),
        ),
    )

    // Start server
    srv := server.New(&server.Config{Port: 8080}, log)
    goconnect.Mount(srv, publicPath, publicHandler)
    goconnect.Mount(srv, protectedPath, protectedHandler)
    
    if err := srv.Start(context.Background()); err != nil {
        log.Fatal("server failed", map[string]interface{}{"error": err})
    }
}
```



## Key Types & Functions

| Symbol | Description |
|---|---|
| `Config` | Server-side config: SendMaxBytes, ReadMaxBytes, Enabled |
| `Service` | Interface — `Path() string`, `Handler() http.Handler` |
| `NewService(path, handler)` | Create a Service from path and handler |
| `HandlerMounter` | Interface for servers that mount HTTP handlers |
| `Mount(srv, path, handler)` | Mount a single Connect handler on any HandlerMounter |
| `MountServices(srv, ...Service)` | Mount multiple services at once |
| `LoggingInterceptor(log)` | Log RPC calls with duration and status |
| `NormalizingInterceptor(log)` | Convert any handler error to a coded Connect error (unary + streaming); already-coded errors pass through |
| `ValidationInterceptor(v)` | Validate requests with an injected `protovalidate.Validator`, emitting shared violations |
| `DeadlineInterceptor(max)` | Bound unary RPCs to a server maximum deadline |
| `TokenAuthInterceptor(validator)` | Bearer token validation interceptor |
| `JWTAuthInterceptor(jwtSvc)` | JWT-specific auth interceptor |
| `ToConnectError(appErr, service)` | Encode shared google.rpc details; returns `(*connect.Error, error)` |
| `DecodeError(err)` | Return a remote `*rpc.Error` or an explicit decode error; never trust remote text as an AppError |
| `RetryDelay(err)` | Decode the server's minimum delay for a shared resilience policy |
| **client subpackage** | |
| `client.Config` | Client config: BaseURL, Timeout, DialTimeout, Protocol, TLS |
| `client.NewHTTPClient(cfg)` | Create a native `net/http.Transport` HTTP/2 client (h2c or TLS) for ConnectRPC |
| `client.ClientOptions(cfg)` | Build connect.ClientOption slice from config |
| `client.ProtocolOption(cfg)` | Get wire protocol option (gRPC, gRPC-Web, or nil) |

---

Install the normalizer outside validation: logging → normalization → server deadline → auth → validation → handler. Validation returns application errors so the outer boundary can log evaluation causes before serialization. Deadlines only cancel work that honors its context.

The [shared error contract](../errors/README.md#shared-rpc-contract) defines namespaced identities, explicit retry verdicts, semantic violations, and HTTP-only extensions. Connect's HTTP status is protocol-owned. Client retry execution still requires idempotency and a bounded policy; `IsRetryable` alone does not authorize another attempt.

[← Back to main gokit README](../README.md)
