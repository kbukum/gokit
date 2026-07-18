> **Note:** This `skill` primitive borrows the `SKILL.md` filename
> and progressive-disclosure model from Anthropic Agent Skills. It is a distinct primitive
> and makes no interop claim with the Anthropic runtime.

# gokit/skill

`skill` owns `kit.skill.yaml` manifests, progressive-disclosure loading, in-process skill providers,
signature verification seams, and activation envelope helpers.

## Architecture

```mermaid
flowchart TD
    Skill[skill]
    Manifest[manifest + prompt refs]
    Loader[loader + verifier]
    Registry[registry]
    Activation[activation + effective policy]
    Tool[tool references]
    MCP[mcp skill adapter]
    Agent[agent capability bundles]
    Authz[authz]
    Security[security]
    Obs[observability]

    Skill --> Manifest
    Skill --> Loader
    Skill --> Registry
    Skill --> Activation
    Manifest --> Tool
    Loader --> Security
    Activation --> Authz
    Registry --> Obs
    MCP --> Registry
    Agent --> Registry
```
