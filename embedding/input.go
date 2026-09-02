package embedding

import (
	"encoding/json"
	"fmt"

	"github.com/kbukum/gokit/ai"
)

// Canonical wire tags for the embedding input discriminator, shared across kits.
// An input serializes as {"type": <input type>, "value": <payload>}; asset payloads
// nest a second {"type": <asset type>, "value": <bytes|url>} discriminator.
const (
	inputTypeText  = "text"
	inputTypeImage = "image"
	inputTypeAudio = "audio"
	inputTypeVideo = "video"

	assetTypeBytes = "bytes"
	assetTypeURL   = "url"
)

// EmbedInput is the sealed interface for embedding inputs.
// Each variant serializes as a tagged {type, value} discriminator.
type EmbedInput interface {
	embedInput()
}

// Text is a text embedding input.
// Serialization is governed entirely by [Text.MarshalJSON] (the {type,value} wire form),
// so the field carries no JSON tag.
type Text struct {
	Text string
}

func (Text) embedInput() {}

// MarshalJSON serializes text as {"type":"text","value":"<text>"}.
func (t Text) MarshalJSON() ([]byte, error) { return marshalTagged(inputTypeText, t.Text) }

// Image is an image embedding input, either inline bytes or a URL.
// Serialization is governed entirely by [Image.MarshalJSON] (the {type,value} wire form),
// so the fields carry no JSON tags.
type Image struct {
	Data []byte
	URL  string
}

func (Image) embedInput() {}

// MarshalJSON serializes an image as {"type":"image","value":<asset>}.
func (im Image) MarshalJSON() ([]byte, error) {
	return marshalAssetInput(inputTypeImage, im.Data, im.URL)
}

// Audio is an audio embedding input, either inline bytes or a URL.
// Serialization is governed entirely by [Audio.MarshalJSON] (the {type,value} wire form),
// so the fields carry no JSON tags.
type Audio struct {
	Data []byte
	URL  string
}

func (Audio) embedInput() {}

// MarshalJSON serializes audio as {"type":"audio","value":<asset>}.
func (a Audio) MarshalJSON() ([]byte, error) { return marshalAssetInput(inputTypeAudio, a.Data, a.URL) }

// Video is a video embedding input, either inline bytes or a URL.
// Serialization is governed entirely by [Video.MarshalJSON] (the {type,value} wire form),
// so the fields carry no JSON tags.
type Video struct {
	Data []byte
	URL  string
}

func (Video) embedInput() {}

// MarshalJSON serializes video as {"type":"video","value":<asset>}.
func (v Video) MarshalJSON() ([]byte, error) { return marshalAssetInput(inputTypeVideo, v.Data, v.URL) }

// taggedEnvelope is the wire shape of every {type, value} discriminator.
type taggedEnvelope struct {
	Type  string          `json:"type"`
	Value json.RawMessage `json:"value"`
}

func marshalTagged(typ string, value any) ([]byte, error) {
	raw, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}
	return json.Marshal(taggedEnvelope{Type: typ, Value: raw})
}

// marshalAssetInput wraps an asset ({type: bytes|url}) in an input discriminator.
// A URL, when present, takes precedence over inline bytes.
func marshalAssetInput(inputType string, data []byte, url string) ([]byte, error) {
	var asset []byte
	var err error
	if url != "" {
		asset, err = marshalTagged(assetTypeURL, url)
	} else {
		asset, err = marshalTagged(assetTypeBytes, data)
	}
	if err != nil {
		return nil, err
	}
	return json.Marshal(taggedEnvelope{Type: inputType, Value: asset})
}

// UnmarshalJSON decodes the tagged inputs slice into concrete EmbedInput variants.
func (r *EmbedRequest) UnmarshalJSON(data []byte) error {
	var aux struct {
		Model   ai.Model          `json:"model"`
		Inputs  []json.RawMessage `json:"inputs"`
		Options map[string]any    `json:"options,omitempty"`
	}
	if err := json.Unmarshal(data, &aux); err != nil {
		return err
	}
	r.Model = aux.Model
	r.Options = aux.Options
	r.Inputs = make([]EmbedInput, 0, len(aux.Inputs))
	for _, raw := range aux.Inputs {
		in, err := decodeInput(raw)
		if err != nil {
			return err
		}
		r.Inputs = append(r.Inputs, in)
	}
	return nil
}

func decodeInput(raw json.RawMessage) (EmbedInput, error) {
	var env taggedEnvelope
	if err := json.Unmarshal(raw, &env); err != nil {
		return nil, err
	}
	switch env.Type {
	case inputTypeText:
		var s string
		if err := json.Unmarshal(env.Value, &s); err != nil {
			return nil, err
		}
		return Text{Text: s}, nil
	case inputTypeImage:
		data, url, err := decodeAsset(env.Value)
		if err != nil {
			return nil, err
		}
		return Image{Data: data, URL: url}, nil
	case inputTypeAudio:
		data, url, err := decodeAsset(env.Value)
		if err != nil {
			return nil, err
		}
		return Audio{Data: data, URL: url}, nil
	case inputTypeVideo:
		data, url, err := decodeAsset(env.Value)
		if err != nil {
			return nil, err
		}
		return Video{Data: data, URL: url}, nil
	default:
		return nil, fmt.Errorf("embedding: unknown input type %q", env.Type)
	}
}

func decodeAsset(raw json.RawMessage) (data []byte, url string, err error) {
	var env taggedEnvelope
	if err := json.Unmarshal(raw, &env); err != nil {
		return nil, "", err
	}
	switch env.Type {
	case assetTypeURL:
		if err := json.Unmarshal(env.Value, &url); err != nil {
			return nil, "", err
		}
		return nil, url, nil
	case assetTypeBytes:
		if err := json.Unmarshal(env.Value, &data); err != nil {
			return nil, "", err
		}
		return data, "", nil
	default:
		return nil, "", fmt.Errorf("embedding: unknown asset type %q", env.Type)
	}
}
