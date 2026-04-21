# envconfig

A lightweight, type-safe Go library for loading configuration from environment variables into structs — with support for defaults, required fields, nested structs, prefix scoping, pointer types, maps, slices, validation, sensitive masking, custom types, `time.Duration`, and `time.Time`.

[![Go Reference](https://pkg.go.dev/badge/github.com/Shin-9x/envconfig.svg)](https://pkg.go.dev/github.com/Shin-9x/envconfig)
[![License](https://img.shields.io/badge/license-Apache%202.0-blue.svg)](LICENSE)

---

## Features

- **Generic & type-safe** — uses Go generics (`Load[T]()`) for zero-boilerplate usage
- **Struct tags** — declarative configuration via `env`, `default`, `required`, `sensitive`, `sep`, `kvSep`, `envPrefix`, and `validate` tags
- **Rich type support** — all integer and float variants, `bool`, `string`, `time.Duration`, `time.Time`, pointer types, slices, and maps with string keys
- **Nested structs** — recursive resolution with optional prefix scoping via `envPrefix`
- **Pointer fields** — allocated on demand; left `nil` when the value is absent
- **Validation** — built-in tag-based rules (`min`, `max`, `oneof`, `regex`, `len`, `minlen`, `maxlen`) and a custom `Validator` interface for post-parse logic
- **Standard interfaces** — supports `encoding.TextUnmarshaler` for loading and `encoding.TextMarshaler` for display in `Mask()`
- **Custom types** — implement `Unmarshaler` for full control over raw value parsing
- **Sensitive field masking** — redact secrets from logs with `Mask()`
- **Aggregated errors** — all missing/invalid fields are reported at once, with fully-qualified field paths

---

## Installation

```bash
go get github.com/Shin-9x/envconfig
```

Requires **Go 1.25.6** or later.

---

## Quick Start

```go
package main

import (
    "fmt"
    "log"
    "time"

    "github.com/Shin-9x/envconfig"
)

type DatabaseConfig struct {
    Host string `env:"HOST" default:"localhost"`
    Port int    `env:"PORT" default:"5432" validate:"min=1,max=65535"`
}

type Config struct {
    Database DatabaseConfig `envPrefix:"DB_"`
    Debug    bool           `env:"APP_DEBUG"   default:"false"`
    Timeout  time.Duration  `env:"APP_TIMEOUT" default:"30s"`
    APIKey   string         `env:"API_KEY"     required:"true" sensitive:"true"`
    Tags     []string       `env:"APP_TAGS"    default:"web,api" comma:","`
    Labels   map[string]string `env:"APP_LABELS" sep:"," kvSep:":"`
}

func main() {
    cfg, err := envconfig.Load[Config]()
    if err != nil {
        log.Fatal(err)
    }

    fmt.Println(envconfig.Mask(cfg))
    // Config: {DB_HOST=localhost, DB_PORT=5432, APP_DEBUG=false, APP_TIMEOUT=30s, API_KEY=[*****], APP_TAGS=[web,api], APP_LABELS=map[]}
}
```

---

## Struct Tags Reference

| Tag         | Description                                                                                        |
|-------------|----------------------------------------------------------------------------------------------------|
| `env`       | Environment variable name to read. Fields without this tag are skipped.                           |
| `envPrefix` | Prefix prepended to all `env` keys within the tagged struct field. Prefixes are cumulative.       |
| `default`   | Fallback value when the variable is unset or empty.                                               |
| `required`  | Mark the field as required. Any non-empty tag value (e.g. `"true"`) triggers the check.          |
| `sensitive` | Set to `"true"` to redact the value in `Mask()` output.                                          |
| `sep`       | Separator for slice and map fields. Defaults to `","`.                                            |
| `kvSep`     | Key-value separator for map fields. Defaults to `":"`.                                            |
| `validate`  | Comma-separated validation rules applied after the field is set (see [Validation](#validation)).  |

---

## Supported Types

| Go Type          | Example env value          | Notes                              |
|------------------|----------------------------|------------------------------------|
| `string`         | `hello`                    |                                    |
| `int`            | `42`                       |                                    |
| `int32` / `rune` | `65`                       |                                    |
| `int64`          | `9223372036854775807`      |                                    |
| `uint`           | `10`                       |                                    |
| `uint8` / `byte` | `255`                      |                                    |
| `uint64`         | `18446744073709551615`     |                                    |
| `float32`        | `3.14`                     |                                    |
| `float64`        | `2.718281828`              |                                    |
| `bool`           | `true`, `false`, `1`, `0`  |                                    |
| `time.Duration`  | `30s`, `1h`, `500ms`       |                                    |
| `time.Time`      | `2024-01-15T10:30:00Z`     | Parsed as RFC3339                  |
| `*T`             | any value for `T`          | `nil` if the variable is absent    |
| `[]T`            | `a,b,c`                    | Any of the above as element type   |
| `map[string]T`   | `key1:val1,key2:val2`      | String keys only                   |

---

## Nested Structs & Prefix Scoping

Nested structs are resolved recursively. Use `envPrefix` on a struct field to scope its children under a common prefix. Prefixes from outer and inner levels are concatenated.

```go
type DatabaseConfig struct {
    Host string `env:"HOST" default:"localhost"`
    Port int    `env:"PORT" default:"5432"`
}

type Config struct {
    Database DatabaseConfig `envPrefix:"DB_"`
}
// DB_HOST → Config.Database.Host
// DB_PORT → Config.Database.Port
```

Prefixes nest naturally:

```go
type Config struct {
    Prod struct {
        DB DatabaseConfig `envPrefix:"DB_"`
    } `envPrefix:"PROD_"`
}
// PROD_DB_HOST → Config.Prod.DB.Host
```

Struct types that implement `Unmarshaler` or `encoding.TextUnmarshaler` are treated as scalars and are not recursed into.

---

## Pointer Fields

Fields with pointer types are allocated only when the environment variable is present and non-empty. If the variable is absent and the field is not required, the pointer remains `nil`.

```go
type Config struct {
    MaxRetries *int `env:"MAX_RETRIES"` // nil if MAX_RETRIES is unset
}
```

---

## Map Fields

Maps with string keys are supported. Items are split by `sep` (default `","`) and each item is split into key and value by `kvSep` (default `":"`).

```go
type Config struct {
    Labels map[string]string `env:"LABELS" sep:"," kvSep:":"`
}
// LABELS=env:prod,region:eu → map[env:prod region:eu]
```

The map value type can be any scalar type supported by the library.

---

## Validation

Validation runs after a field is successfully set and only when the field has a non-empty value. Two mechanisms are available and can be combined freely.

### Built-in Tag Rules

Declare rules in the `validate` struct tag as a comma-separated list of `key=value` pairs:

```go
type Config struct {
    Port    int    `env:"PORT"     validate:"min=1,max=65535"`
    Level   string `env:"LOG_LVL"  validate:"oneof=DEBUG|INFO|WARN|ERROR"`
    Token   string `env:"TOKEN"    validate:"minlen=32,maxlen=128"`
    Code    string `env:"CODE"     validate:"regex=^[A-Z]{3}-[0-9]{4}$"`
    Pin     string `env:"PIN"      validate:"len=4"`
}
```

| Rule     | Applies to              | Description                                      |
|----------|-------------------------|--------------------------------------------------|
| `min`    | numeric types           | Value must be ≥ the given bound (inclusive).     |
| `max`    | numeric types           | Value must be ≤ the given bound (inclusive).     |
| `oneof`  | string                  | Value must match one of the pipe-separated options. |
| `regex`  | string                  | Value must match the regular expression.         |
| `len`    | string, slice, map      | Exact length required.                           |
| `minlen` | string, slice, map      | Length must be ≥ the given bound.                |
| `maxlen` | string, slice, map      | Length must be ≤ the given bound.                |

> **Note on regex:** Commas inside regex patterns are handled correctly — the parser splits only on commas that are immediately followed by another known rule name.

### Custom `Validator` Interface

For logic that cannot be expressed with tags, implement `Validator` on your type:

```go
type Validator interface {
    ValidateEnv() error
}
```

`ValidateEnv` is called after the field is set. Both value and pointer receivers are supported.

```go
type Port struct{ value int }

func (p Port) ValidateEnv() error {
    if p.value < 1 || p.value > 65535 {
        return fmt.Errorf("port must be between 1 and 65535, got %d", p.value)
    }
    return nil
}
```

---

## Custom Parsing via `Unmarshaler`

Implement `Unmarshaler` to control how a custom type is parsed from its raw string value:

```go
type Unmarshaler interface {
    UnmarshalEnv(value string) error
}
```

`Unmarshaler` takes precedence over `encoding.TextUnmarshaler` when both are implemented.

```go
type LogLevel struct{ level string }

func (l *LogLevel) UnmarshalEnv(value string) error {
    switch value {
    case "DEBUG", "INFO", "WARN", "ERROR":
        l.level = value
        return nil
    default:
        return fmt.Errorf("invalid log level %q, must be one of DEBUG, INFO, WARN, ERROR", value)
    }
}

type Config struct {
    Level LogLevel `env:"LOG_LEVEL" default:"INFO"`
}
```

The library also supports the standard `encoding.TextUnmarshaler` interface out of the box, without any additional setup.

---

## Masking Sensitive Fields

Use `Mask()` to produce a log-safe string representation of your config. Fields tagged with `sensitive:"true"` are replaced with `[*****]`. The function handles nested structs, pointer fields, nil slices, and map fields transparently. Types that implement `encoding.TextMarshaler` are rendered via their `MarshalText` output.

```go
cfg, _ := envconfig.Load[Config]()
fmt.Println(envconfig.Mask(cfg))
// Config: {DB_HOST=localhost, DB_PORT=5432, API_KEY=[*****], APP_TAGS=[web,api], APP_LABELS=map[env:prod, region:eu]}

// Also works with a pointer
fmt.Println(envconfig.Mask(&cfg))
```

---

## Error Handling

`Load` collects **all** errors before returning, giving you a complete picture of every misconfigured field in a single run. Each error includes the fully-qualified field path:

```
Database.Host: missing required env var DB_HOST
Database.Port: value 99999 exceeds max=65535 for DB_PORT
```

```go
cfg, err := envconfig.Load[Config]()
if err != nil {
    log.Fatal(err)
}
```

---

## License

This project is licensed under the [Apache License 2.0](LICENSE).
