# envconfig

A lightweight, type-safe Go library for loading configuration from environment variables into structs — with support for defaults, required fields, nested structs, prefix scoping, pointer types, sensitive masking, slices, custom types, `time.Duration`, and `time.Time`.

[![Go Reference](https://pkg.go.dev/badge/github.com/Shin-9x/envconfig.svg)](https://pkg.go.dev/github.com/Shin-9x/envconfig)
[![License](https://img.shields.io/badge/license-Apache%202.0-blue.svg)](LICENSE)

---

## Features

- **Generic & type-safe** — uses Go generics (`Load[T]()`) for zero-boilerplate usage
- **Struct tags** — declarative configuration via `env`, `default`, `required`, `sensitive`, `sep`, and `envPrefix` tags
- **Built-in type support** — `string`, `int`, `int64`, `float32`, `float64`, `bool`, `time.Duration`, `time.Time`, pointers to any supported type, and slices of any supported type
- **Nested structs** — recursive resolution with optional prefix scoping via `envPrefix`
- **Pointer fields** — pointers are allocated on demand; left `nil` when the value is absent
- **Custom types** — implement the `Unmarshaler` interface for full control over parsing
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
    Port int    `env:"PORT" default:"5432"`
}

type Config struct {
    Database DatabaseConfig `envPrefix:"DB_"`
    Debug    bool           `env:"APP_DEBUG"   default:"false"`
    Timeout  time.Duration  `env:"APP_TIMEOUT" default:"30s"`
    APIKey   string         `env:"API_KEY"     required:"true" sensitive:"true"`
    Tags     []string       `env:"APP_TAGS"    default:"web,api" sep:","`
}

func main() {
    cfg, err := envconfig.Load[Config]()
    if err != nil {
        log.Fatal(err)
    }

    fmt.Println(envconfig.Mask(cfg))
    // Config: {DB_HOST=localhost, DB_PORT=5432, APP_DEBUG=false, APP_TIMEOUT=30s, API_KEY=[*****], APP_TAGS=[web,api]}
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
| `sep`       | Separator for slice fields. Defaults to `","`.                                                    |

---

## Supported Types

| Go Type         | Example env value          | Notes                              |
|-----------------|----------------------------|------------------------------------|
| `string`        | `hello`                    |                                    |
| `int`           | `42`                       |                                    |
| `int64`         | `9223372036854775807`      |                                    |
| `float32`       | `3.14`                     |                                    |
| `float64`       | `2.718281828`              |                                    |
| `bool`          | `true`, `false`, `1`, `0`  |                                    |
| `time.Duration` | `30s`, `1h`, `500ms`       |                                    |
| `time.Time`     | `2024-01-15T10:30:00Z`     | Parsed as RFC3339                  |
| `*T`            | any value for `T`          | `nil` if the variable is absent    |
| `[]T`           | `a,b,c`                    | Any of the above as element type   |

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
```

This maps `DB_HOST` → `Database.Host` and `DB_PORT` → `Database.Port`.

Prefixes nest naturally:

```go
type Config struct {
    Prod struct {
        DB DatabaseConfig `envPrefix:"DB_"`
    } `envPrefix:"PROD_"`
}
// resolves PROD_DB_HOST → Config.Prod.DB.Host
```

---

## Pointer Fields

Fields with pointer types are allocated only when the environment variable is present and non-empty. If the variable is absent and the field is not required, the pointer remains `nil`.

```go
type Config struct {
    MaxRetries *int `env:"MAX_RETRIES"` // nil if MAX_RETRIES is unset
}
```

---

## Custom Types via `Unmarshaler`

Implement the `Unmarshaler` interface to control how a custom type is parsed:

```go
type Unmarshaler interface {
    UnmarshalEnv(value string) error
}
```

### Example

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

---

## Masking Sensitive Fields

Use `Mask()` to produce a log-safe string representation of your config. Fields tagged with `sensitive:"true"` are replaced with `[*****]`. The function handles nested structs, pointer fields, and nil slices transparently.

```go
cfg, _ := envconfig.Load[Config]()
fmt.Println(envconfig.Mask(cfg))
// Config: {DB_HOST=localhost, DB_PORT=5432, API_KEY=[*****], APP_TAGS=[web,api]}

// Also works with a pointer
fmt.Println(envconfig.Mask(&cfg))
```

---

## Error Handling

`Load` collects **all** errors before returning, so you get a complete picture of what is misconfigured rather than fixing problems one at a time. Each error includes the fully-qualified field path to pinpoint exactly where the issue is:

```
Database.Host: missing required env var DB_HOST
Database.Port: invalid int for DB_PORT: strconv.Atoi: parsing "abc": invalid syntax
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
