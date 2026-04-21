package envconfig

import (
	"testing"
)

// -------------------- TEST STRUCTS --------------------

type maskCfg struct {
	Name     string `env:"NAME"`
	Password string `env:"PASS" sensitive:"true"`
}

type nestedMaskCfg struct {
	User struct {
		Email string `env:"EMAIL"`
	} `envPrefix:"U_"`
}

type sliceMaskCfg struct {
	Tags []string `env:"TAGS"`
}

type mapMaskCfg struct {
	Data map[string]string `env:"DATA"`
}

// -------------------- TEXT MARSHALER --------------------

type customText struct {
	Value string
}

func (c customText) MarshalText() ([]byte, error) {
	return []byte("TEXT:" + c.Value), nil
}

type textCfg struct {
	Field customText `env:"FIELD"`
}

// -------------------- TESTS --------------------

func TestMask_SensitiveField(t *testing.T) {
	cfg := maskCfg{
		Name:     "alice",
		Password: "secret123",
	}

	out := Mask(cfg)

	if out == "" {
		t.Fatal("empty output")
	}

	if !contains(out, "[*****]") {
		t.Errorf("expected masked password, got: %s", out)
	}
}

func TestMask_NestedStruct(t *testing.T) {
	cfg := nestedMaskCfg{}
	cfg.User.Email = "a@b.com"

	out := Mask(cfg)

	if !contains(out, "a@b.com") {
		t.Errorf("nested value missing: %s", out)
	}
}

func TestMask_Slice(t *testing.T) {
	cfg := sliceMaskCfg{
		Tags: []string{"a", "b"},
	}

	out := Mask(cfg)

	if !contains(out, "[a,b]") {
		t.Errorf("slice formatting incorrect: %s", out)
	}
}

func TestMask_Map(t *testing.T) {
	cfg := mapMaskCfg{
		Data: map[string]string{
			"b": "2",
			"a": "1",
		},
	}

	out := Mask(cfg)

	if !contains(out, "a:1") || !contains(out, "b:2") {
		t.Errorf("map formatting incorrect: %s", out)
	}
}

func TestMask_TextMarshaler(t *testing.T) {
	cfg := textCfg{
		Field: customText{Value: "hello"},
	}

	out := Mask(cfg)

	if !contains(out, "TEXT:hello") {
		t.Errorf("TextMarshaler not used: %s", out)
	}
}

// -------------------- HELPER --------------------

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (stringIndex(s, substr) >= 0)
}

// lightweight index (no strings package to keep test minimal dependency footprint)
func stringIndex(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}
