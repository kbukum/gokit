# authz

Authorization building blocks with a canonical **RBAC + ABAC** engine
and explicit default-deny semantics.

This module has **zero external dependencies**.

## Quick Start

```go
import "github.com/kbukum/gokit/authz"

engine, _ := authz.NewEngine(
    []authz.Role{
        {
            Name: "editor",
            Inherits: []string{"viewer"},
            Permissions: []authz.Permission{
                {Resource: "article", Action: "write"},
            },
        },
        {
            Name: "viewer",
            Permissions: []authz.Permission{
                {Resource: "article", Action: "read"},
            },
        },
    },
    []authz.Policy{
        {
            Name: "same-tenant-only",
            Effect: authz.EffectAllow,
            Resources: []string{"article"},
            Actions: []string{"write"},
            Conditions: []authz.Condition{{
                Source: authz.AttributeSourceSubject,
                Key: "tenant_id",
                Operator: authz.OperatorEquals,
                CompareSource: authz.AttributeSourceResource,
                CompareKey: "tenant_id",
            }},
        },
    },
)

decision := engine.Authorize(authz.Request{
    Subject: authz.Subject{
        ID: "user-1",
        Roles: []string{"editor"},
        Attributes: authz.Attributes{"tenant_id": "tenant-a"},
    },
    Resource: authz.Resource{
        Type: "article",
        Attributes: authz.Attributes{"tenant_id": "tenant-a"},
    },
    Action: "write",
})
```

## Legacy helper

`MapChecker` remains available for simple wildcard permission maps:

```go
checker := authz.NewMapChecker(map[string][]string{
    "admin": {"*:*"},
})
allowed := checker.HasPermission("admin", "article:delete")
```

## Key Types

| Symbol | Description |
|---|---|
| `Engine` | RBAC + ABAC authorization engine |
| `Role` | Hierarchical RBAC role with wildcard permissions |
| `Policy` | ABAC rule with allow/deny effect |
| `Condition` | Subject/resource/context attribute comparison |
| `Request` | Canonical authorization request |
| `Decision` | Authorization result with reason |
| `Checker` | Lightweight boolean permission interface |
| `MapChecker` | Legacy wildcard map-backed checker |
