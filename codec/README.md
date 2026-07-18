# codec

Pluggable structured-text codecs (JSON, TOML, …) over a single canonical in-memory model.
Any package that reads or writes a config file, manifest,
or document reuses these codecs instead of re-implementing "bounded read → parse → typed error" per format.

## Install

```bash
go get github.com/kbukum/gokit
```

## Quick Start

```go
package main

import (
    "fmt"

    "github.com/kbukum/gokit/codec"
)

type Config struct {
    Name    string `json:"name"`
    Retries int    `json:"retries"`
}

func main() {
    c := codec.PrettyJSON()

    text, _ := codec.Encode(c, Config{Name: "svc", Retries: 3})
    fmt.Println(text)

    cfg, _ := codec.Decode[Config](c, text)
    fmt.Println(cfg.Retries) // 3

    // Select a codec at runtime by name or file path.
    if tc, ok := codec.CodecForPath("app.toml"); ok {
        _, _ = codec.Encode(tc, cfg)
    }
}
```

## Key Types & Functions

| Name | Description |
|------|-------------|
| `Codec` | Interface encoding/decoding one text format through the `Value` model |
| `Value` (`= any`) | Canonical format-neutral tree (documented opaque-value exception) |
| `Encode[T](codec, value)` / `Decode[T](codec, contents)` | Generic typed encode/decode |
| `PrettyJSON()` / `CompactJSON()` | JSON codecs (multiline / single-line) |
| `NewTOMLCodec()` | TOML codec |
| `CodecForName(name)` / `CodecForPath(path)` | Runtime codec selection by format name or file path |

---

[⬅ Back to main README](../README.md)
