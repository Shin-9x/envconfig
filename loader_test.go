package envconfig

import (
	"testing"
)

// -------------------- BASIC TYPES --------------------

type basicCfg struct {
	Name string `env:"NAME"`
	Age  int    `env:"AGE"`
	Live bool   `env:"LIVE"`
}

// -------------------- DEFAULT / REQUIRED --------------------

type defaultCfg struct {
	Mode string `env:"MODE" default:"prod"`
}

type requiredCfg struct {
	Token string `env:"TOKEN" required:"true"`
}

// -------------------- PREFIX --------------------

type prefixInner struct {
	Value string `env:"VALUE"`
}

type prefixCfg struct {
	Inner prefixInner `envPrefix:"APP_"`
}

// -------------------- POINTER --------------------

type pointerCfg struct {
	Name *string `env:"NAME"`
}

// -------------------- SLICE --------------------

type sliceCfg struct {
	Tags []string `env:"TAGS" sep:";"`
}

// -------------------- MAP --------------------

type mapCfg struct {
	Headers map[string]string `env:"HEADERS" sep="," kvSep=":"`
}

// -------------------- NESTED --------------------

type nestedCfg struct {
	DB struct {
		Host string `env:"HOST"`
		Port int    `env:"PORT"`
	} `envPrefix:"DB_"`
}

// -------------------- TESTS --------------------

func TestLoad_BasicTypes(t *testing.T) {
	t.Setenv("NAME", "alice")
	t.Setenv("AGE", "30")
	t.Setenv("LIVE", "true")

	cfg, err := Load[basicCfg]()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if cfg.Name != "alice" {
		t.Errorf("Name = %s", cfg.Name)
	}
	if cfg.Age != 30 {
		t.Errorf("Age = %d", cfg.Age)
	}
	if cfg.Live != true {
		t.Errorf("Live = %v", cfg.Live)
	}
}

func TestLoad_DefaultValue(t *testing.T) {
	cfg, err := Load[defaultCfg]()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if cfg.Mode != "prod" {
		t.Errorf("Mode = %s", cfg.Mode)
	}
}

func TestLoad_RequiredMissing(t *testing.T) {
	_, err := Load[requiredCfg]()
	if err == nil {
		t.Fatal("expected error for missing required env")
	}
}

func TestLoad_PointerField(t *testing.T) {
	t.Setenv("NAME", "bob")

	cfg, err := Load[pointerCfg]()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if cfg.Name == nil || *cfg.Name != "bob" {
		t.Errorf("pointer not set correctly")
	}
}

func TestLoad_PointerField_Empty(t *testing.T) {
	cfg, err := Load[pointerCfg]()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if cfg.Name != nil {
		t.Errorf("expected nil pointer")
	}
}

func TestLoad_Slice(t *testing.T) {
	t.Setenv("TAGS", "a;b;c")

	cfg, err := Load[sliceCfg]()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(cfg.Tags) != 3 {
		t.Errorf("expected 3 tags, got %d", len(cfg.Tags))
	}
}

func TestLoad_Map(t *testing.T) {
	t.Setenv("HEADERS", "k1:v1,k2:v2")

	cfg, err := Load[mapCfg]()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if cfg.Headers["k1"] != "v1" {
		t.Errorf("k1 mismatch")
	}
	if cfg.Headers["k2"] != "v2" {
		t.Errorf("k2 mismatch")
	}
}

func TestLoad_NestedStruct(t *testing.T) {
	t.Setenv("DB_HOST", "localhost")
	t.Setenv("DB_PORT", "5432")

	cfg, err := Load[nestedCfg]()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if cfg.DB.Host != "localhost" {
		t.Errorf("host mismatch")
	}
	if cfg.DB.Port != 5432 {
		t.Errorf("port mismatch")
	}
}
