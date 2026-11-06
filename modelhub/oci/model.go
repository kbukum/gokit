package oci

import (
	"encoding/json"
	"path"
	"strings"
	"time"

	apperr "github.com/kbukum/gokit/errors"
)

// Kind names the config format a model artifact uses.
type Kind string

// Config kinds.
const (
	// KindModelPack is a CNCF ModelPack config (application/vnd.cncf.model.config.v1+json).
	KindModelPack Kind = "modelpack"
	// KindDocker is a Docker model config (application/vnd.docker.ai.model.config.v0.x+json).
	KindDocker Kind = "docker"
	// KindUnknown is any other config; no model facts are read from it.
	KindUnknown Kind = "unknown"
)

// Role names what a layer holds.
type Role string

// Layer roles.
const (
	RoleWeights   Role = "weights"
	RoleProjector Role = "projector"
	RoleLicense   Role = "license"
	RoleDoc       Role = "doc"
	RoleTemplate  Role = "template"
	RoleConfig    Role = "config"
	RoleOther     Role = "other"
)

// Media types the client understands.
const (
	MediaTypeModelPackConfig = "application/vnd.cncf.model.config.v1+json"
	MediaTypeDockerConfigV01 = "application/vnd.docker.ai.model.config.v0.1+json"
	MediaTypeDockerConfigV02 = "application/vnd.docker.ai.model.config.v0.2+json"
	annotationFilePath       = "org.cncf.model.filepath"
	maxPathBytes             = 1024
)

// Layer is one blob of a model artifact.
type Layer struct {
	// MediaType is the layer media type.
	MediaType string
	// Digest is the layer's sha256 digest.
	Digest string
	// Size is the declared size in bytes.
	Size int64
	// Path is the file path a ModelPack layer declares, if any.
	Path string
	// Role names what the layer holds.
	Role Role
}

// Capabilities are the capabilities a ModelPack config declares. Nil fields are not stated.
type Capabilities struct {
	InputTypes  []string
	OutputTypes []string
	ToolUsage   *bool
	Reasoning   *bool
}

// Model holds the facts read from a model artifact. Empty fields are not stated by the artifact.
type Model struct {
	// Reference is the reference that was resolved.
	Reference Reference
	// Digest is the manifest's verified sha256 digest; resolve it to read the same artifact again.
	Digest string
	// ArtifactType is the manifest artifact type, if any.
	ArtifactType string
	// ConfigMediaType is the config blob media type.
	ConfigMediaType string
	// Kind is the config format.
	Kind Kind
	// Name, Family and Architecture identify the model.
	Name, Family, Architecture string
	// Format is the weight format, such as "gguf" or "safetensors".
	Format string
	// Parameters is the stated parameter size, such as "8b" or "361.82 M".
	Parameters string
	// Quantization and Precision describe the weight encoding.
	Quantization, Precision string
	// Licenses are license identifiers the config states. Docker models carry license files instead; see Files.
	Licenses []string
	// ContextLength is the stated context length in tokens, or 0.
	ContextLength int64
	// Capabilities are stated capabilities, or nil.
	Capabilities *Capabilities
	// Weights are the weight layers; WeightBytes is their declared total.
	Weights     []Layer
	WeightBytes int64
	// Annotations are the manifest's annotations, such as "ai.model.repo" naming the repository a Docker model was
	// packaged from. They are stated by the registry, not verified.
	Annotations map[string]string
	// Files are the other layers, such as licenses, docs, templates and vision projectors.
	Files []Layer
	// ReadAt is when the artifact was read.
	ReadAt time.Time
}

type modelPackConfig struct {
	Descriptor struct {
		Name     string   `json:"name"`
		Family   string   `json:"family"`
		Licenses []string `json:"licenses"`
	} `json:"descriptor"`
	Config struct {
		Architecture string `json:"architecture"`
		Format       string `json:"format"`
		ParamSize    string `json:"paramSize"`
		Precision    string `json:"precision"`
		Quantization string `json:"quantization"`
		Capabilities *struct {
			InputTypes  []string `json:"inputTypes"`
			OutputTypes []string `json:"outputTypes"`
			ToolUsage   *bool    `json:"toolUsage"`
			Reasoning   *bool    `json:"reasoning"`
		} `json:"capabilities"`
	} `json:"config"`
}

type dockerConfig struct {
	Config struct {
		Format       string `json:"format"`
		Quantization string `json:"quantization"`
		Parameters   string `json:"parameters"`
		Architecture string `json:"architecture"`
		ContextSize  *int64 `json:"context_size"`
	} `json:"config"`
}

// describe reads the facts a config blob states.
func (m *Model) describe(body []byte) error {
	switch m.ConfigMediaType {
	case MediaTypeModelPackConfig:
		var cfg modelPackConfig
		if err := json.Unmarshal(body, &cfg); err != nil {
			return malformed("config")
		}
		m.Kind = KindModelPack
		m.Name, m.Family, m.Licenses = cfg.Descriptor.Name, cfg.Descriptor.Family, cfg.Descriptor.Licenses
		c := cfg.Config
		m.Architecture, m.Format, m.Parameters, m.Precision, m.Quantization = c.Architecture, c.Format, c.ParamSize, c.Precision, c.Quantization
		if caps := c.Capabilities; caps != nil {
			m.Capabilities = &Capabilities{InputTypes: caps.InputTypes, OutputTypes: caps.OutputTypes, ToolUsage: caps.ToolUsage, Reasoning: caps.Reasoning}
		}
	case MediaTypeDockerConfigV01, MediaTypeDockerConfigV02:
		var cfg dockerConfig
		if err := json.Unmarshal(body, &cfg); err != nil {
			return malformed("config")
		}
		m.Kind = KindDocker
		c := cfg.Config
		m.Format, m.Quantization, m.Parameters, m.Architecture = c.Format, c.Quantization, c.Parameters, c.Architecture
		if c.ContextSize != nil && *c.ContextSize > 0 {
			m.ContextLength = *c.ContextSize
		}
	default:
		m.Kind = KindUnknown
	}
	return nil
}

// roleOf names what a layer holds. ModelPack packs a vision projector as a weight layer, so a weight file named
// "mmproj-*" by llama.cpp convention is a projector, as Docker's own projector media type is.
func roleOf(mediaType, filePath string) Role {
	switch {
	case strings.HasPrefix(mediaType, "application/vnd.cncf.model.weight.v1.") && strings.HasPrefix(strings.ToLower(path.Base(filePath)), "mmproj"):
		return RoleProjector
	case strings.HasPrefix(mediaType, "application/vnd.cncf.model.weight.config.v1."):
		return RoleConfig
	case strings.HasPrefix(mediaType, "application/vnd.cncf.model.weight.v1."),
		mediaType == "application/vnd.docker.ai.gguf.v3",
		mediaType == "application/vnd.docker.ai.safetensors",
		mediaType == "application/vnd.docker.ai.dduf":
		return RoleWeights
	case mediaType == "application/vnd.docker.ai.mmproj":
		return RoleProjector
	case mediaType == "application/vnd.docker.ai.license":
		return RoleLicense
	case strings.HasPrefix(mediaType, "application/vnd.cncf.model.doc.v1."):
		return RoleDoc
	case mediaType == "application/vnd.docker.ai.chat.template.jinja":
		return RoleTemplate
	default:
		return RoleOther
	}
}

func malformed(field string) error {
	return apperr.InvalidFormat(field, "a valid model artifact").WithReason(ReasonMalformed)
}
