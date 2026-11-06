package oci

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"mime"

	apperr "github.com/kbukum/gokit/errors"
	"github.com/kbukum/gokit/modelhub/internal/fetch"
)

// Reasons on [github.com/kbukum/gokit/errors.AppError] values the client returns.
const (
	// ReasonDigestMismatch marks content whose sha256 digest differs from the digest the registry or caller stated.
	ReasonDigestMismatch = "OCI_DIGEST_MISMATCH"
	// ReasonIndexUnsupported marks an image index or manifest list; resolve a platform manifest by digest instead.
	ReasonIndexUnsupported = "OCI_INDEX_UNSUPPORTED"
	// ReasonMalformed marks a manifest or config that does not follow the specification.
	ReasonMalformed = "OCI_MALFORMED"
)

const (
	mediaTypeOCIManifest    = "application/vnd.oci.image.manifest.v1+json"
	mediaTypeOCIIndex       = "application/vnd.oci.image.index.v1+json"
	mediaTypeDockerManifest = "application/vnd.docker.distribution.manifest.v2+json"
	mediaTypeDockerList     = "application/vnd.docker.distribution.manifest.list.v2+json"
	manifestAccept          = mediaTypeOCIManifest + ", " + mediaTypeDockerManifest + ", " + mediaTypeOCIIndex + ", " + mediaTypeDockerList
)

type descriptor struct {
	MediaType   string            `json:"mediaType"`
	Digest      string            `json:"digest"`
	Size        int64             `json:"size"`
	Annotations map[string]string `json:"annotations"`
}

type manifest struct {
	SchemaVersion int               `json:"schemaVersion"`
	MediaType     string            `json:"mediaType"`
	ArtifactType  string            `json:"artifactType"`
	Config        *descriptor       `json:"config"`
	Layers        []descriptor      `json:"layers"`
	Manifests     []descriptor      `json:"manifests"`
	Annotations   map[string]string `json:"annotations"`
}

// Bounds on manifest annotations, so a hostile registry cannot hand callers unbounded text.
const (
	maxAnnotations          = 64
	maxAnnotationKeyBytes   = 255
	maxAnnotationValueBytes = 4096
)

func validAnnotations(a map[string]string) error {
	if len(a) > maxAnnotations {
		return malformed("manifest.annotations")
	}
	for k, v := range a {
		if k == "" || len(k) > maxAnnotationKeyBytes || len(v) > maxAnnotationValueBytes {
			return malformed("manifest.annotations")
		}
	}
	return nil
}

func (d descriptor) validate() error {
	if err := validDigest(d.Digest); err != nil {
		return malformed("descriptor.digest")
	}
	if d.Size < 0 || len(d.MediaType) > 255 {
		return malformed("descriptor")
	}
	return nil
}

// Resolve reads the manifest and config of ref and returns the model facts they state. It never downloads weights.
func (c *Client) Resolve(ctx context.Context, ref Reference) (*Model, error) {
	if err := ref.Validate(); err != nil {
		return nil, err
	}
	resp, err := c.get(ctx, ref.Repository, fetch.Request{
		URL:      c.registry + "/v2/" + ref.Repository + "/manifests/" + ref.ref(),
		Headers:  map[string]string{"Accept": manifestAccept},
		MaxBytes: c.cfg.MaxManifestBytes,
	})
	if err != nil {
		return nil, err
	}
	digest := digestOf(resp.Body)
	if stated := resp.Header("Docker-Content-Digest"); stated != "" && stated != digest {
		return nil, mismatch("manifest")
	}
	if ref.Digest != "" && ref.Digest != digest {
		return nil, mismatch("manifest")
	}
	var m manifest
	if err = json.Unmarshal(resp.Body, &m); err != nil {
		return nil, malformed("manifest")
	}
	contentType, _, _ := mime.ParseMediaType(resp.Header("Content-Type"))
	if isIndex(m.MediaType) || isIndex(contentType) || m.Manifests != nil {
		return nil, apperr.InvalidFormat("manifest", "a single manifest").WithReason(ReasonIndexUnsupported)
	}
	if m.SchemaVersion != 2 || m.Config == nil || len(m.Layers) > maxLayers {
		return nil, malformed("manifest")
	}
	if err := m.Config.validate(); err != nil {
		return nil, err
	}
	if err := validAnnotations(m.Annotations); err != nil {
		return nil, err
	}
	var annotations map[string]string
	if len(m.Annotations) > 0 {
		annotations = m.Annotations
	}
	model := &Model{Reference: ref, Digest: digest, ArtifactType: m.ArtifactType, ConfigMediaType: m.Config.MediaType, Annotations: annotations, ReadAt: c.fetch.Now()}
	for _, d := range m.Layers {
		if err := d.validate(); err != nil {
			return nil, err
		}
		path := d.Annotations[annotationFilePath]
		layer := Layer{MediaType: d.MediaType, Digest: d.Digest, Size: d.Size, Path: path, Role: roleOf(d.MediaType, path)}
		if len(layer.Path) > maxPathBytes {
			return nil, malformed("layer.path")
		}
		if layer.Role == RoleWeights {
			model.Weights = append(model.Weights, layer)
			if model.WeightBytes > (1<<63-1)-layer.Size {
				return nil, malformed("layer.size")
			}
			model.WeightBytes += layer.Size
		} else {
			model.Files = append(model.Files, layer)
		}
	}
	if model.ConfigMediaType == MediaTypeModelPackConfig || model.ConfigMediaType == MediaTypeDockerConfigV01 || model.ConfigMediaType == MediaTypeDockerConfigV02 {
		body, err := c.config(ctx, ref.Repository, *m.Config)
		if err != nil {
			return nil, err
		}
		if err := model.describe(body); err != nil {
			return nil, err
		}
	} else {
		model.Kind = KindUnknown
	}
	return model, nil
}

// Digest returns the manifest digest ref points to with a HEAD request, which registries such as Docker Hub do not
// count against their pull quota. A tag that names an image index returns the index digest. Resolve the digest to
// read the facts it pins.
func (c *Client) Digest(ctx context.Context, ref Reference) (string, error) {
	if err := ref.Validate(); err != nil {
		return "", err
	}
	_, resp, err := authorized(ctx, c, ref.Repository, func(headers map[string]string) (struct{}, *fetch.Response, error) {
		headers["Accept"] = manifestAccept
		resp, err := c.fetch.Head(ctx, fetch.Request{URL: c.registry + "/v2/" + ref.Repository + "/manifests/" + ref.ref(), Headers: headers})
		return struct{}{}, resp, err
	})
	if err != nil {
		return "", err
	}
	digest := resp.Header("Docker-Content-Digest")
	if validDigest(digest) != nil {
		return "", malformed("Docker-Content-Digest")
	}
	if ref.Digest != "" && ref.Digest != digest {
		return "", mismatch("manifest")
	}
	return digest, nil
}

// config reads and verifies a config blob within the configured bound.
func (c *Client) config(ctx context.Context, repo string, d descriptor) ([]byte, error) {
	if d.Size > c.cfg.MaxConfigBytes {
		return nil, apperr.InvalidInput("config", "The config blob exceeds the size limit").
			WithReason(fetch.ReasonResponseTooLarge).WithDetail("limit_bytes", c.cfg.MaxConfigBytes)
	}
	resp, err := c.get(ctx, repo, fetch.Request{URL: c.blobURL(repo, d.Digest), MaxBytes: max(d.Size, 1)})
	if err != nil {
		return nil, err
	}
	if int64(len(resp.Body)) != d.Size || digestOf(resp.Body) != d.Digest {
		return nil, mismatch("config")
	}
	return resp.Body, nil
}

// get performs an authenticated read of repo.
func (c *Client) get(ctx context.Context, repo string, req fetch.Request) (*fetch.Response, error) {
	_, resp, err := authorized(ctx, c, repo, func(headers map[string]string) (struct{}, *fetch.Response, error) {
		for k, v := range req.Headers {
			headers[k] = v
		}
		resp, err := c.fetch.Get(ctx, fetch.Request{URL: req.URL, Query: req.Query, Headers: headers, MaxBytes: req.MaxBytes})
		return struct{}{}, resp, err
	})
	return resp, err
}

func (c *Client) blobURL(repo, digest string) string {
	return c.registry + "/v2/" + repo + "/blobs/" + digest
}

func isIndex(mediaType string) bool {
	return mediaType == mediaTypeOCIIndex || mediaType == mediaTypeDockerList
}

func digestOf(b []byte) string {
	sum := sha256.Sum256(b)
	return "sha256:" + hex.EncodeToString(sum[:])
}

func mismatch(field string) error {
	return apperr.InvalidFormat(field, "content matching its digest").WithReason(ReasonDigestMismatch)
}
