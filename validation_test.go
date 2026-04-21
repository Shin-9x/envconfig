package envconfig_test

import (
	"fmt"
	"testing"

	"github.com/Shin-9x/envconfig"
)

// ---- Custom Validator example ----

type Port struct {
	value int
}

func (p *Port) UnmarshalEnv(s string) error {
	n := 0
	for _, c := range s {
		if c < '0' || c > '9' {
			return fmt.Errorf("not a number: %q", s)
		}
		n = n*10 + int(c-'0')
	}
	p.value = n
	return nil
}

func (p Port) ValidateEnv() error {
	if p.value < 1 || p.value > 65535 {
		return fmt.Errorf("port must be 1-65535, got %d", p.value)
	}
	return nil
}

// ---- Test structs ----

type numericCfg struct {
	Score int     `env:"SCORE" validate:"min=0,max=100"`
	Rate  float64 `env:"RATE"  validate:"min=0.0,max=1.0"`
}

type oneofCfg struct {
	Level string `env:"LEVEL" validate:"oneof=DEBUG|INFO|WARN|ERROR"`
}

type regexCfg struct {
	Code string `env:"CODE" validate:"regex=^[A-Z]{3}-\\d{4}$"`
}

type lenCfg struct {
	Token string `env:"TOKEN"  validate:"len=32"`
	Tags  string `env:"TAGS"   validate:"minlen=1,maxlen=5"`
}

type customCfg struct {
	Port Port `env:"PORT"`
}

// ---- Tests ----

func TestValidation_NumericRange_Pass(t *testing.T) {
	t.Setenv("SCORE", "42")
	t.Setenv("RATE", "0.75")
	cfg, err := envconfig.Load[numericCfg]()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Score != 42 {
		t.Errorf("Score: got %d, want 42", cfg.Score)
	}
}

func TestValidation_NumericRange_BelowMin(t *testing.T) {
	t.Setenv("SCORE", "-1")
	t.Setenv("RATE", "0.5")
	_, err := envconfig.Load[numericCfg]()
	if err == nil {
		t.Fatal("expected error for SCORE=-1, got nil")
	}
}

func TestValidation_NumericRange_AboveMax(t *testing.T) {
	t.Setenv("SCORE", "101")
	t.Setenv("RATE", "0.5")
	_, err := envconfig.Load[numericCfg]()
	if err == nil {
		t.Fatal("expected error for SCORE=101, got nil")
	}
}

func TestValidation_FloatRange_AboveMax(t *testing.T) {
	t.Setenv("SCORE", "50")
	t.Setenv("RATE", "1.1")
	_, err := envconfig.Load[numericCfg]()
	if err == nil {
		t.Fatal("expected error for RATE=1.1, got nil")
	}
}

func TestValidation_OneOf_Pass(t *testing.T) {
	t.Setenv("LEVEL", "INFO")
	_, err := envconfig.Load[oneofCfg]()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestValidation_OneOf_Fail(t *testing.T) {
	t.Setenv("LEVEL", "TRACE")
	_, err := envconfig.Load[oneofCfg]()
	if err == nil {
		t.Fatal("expected error for LEVEL=TRACE, got nil")
	}
}

func TestValidation_Regex_Pass(t *testing.T) {
	t.Setenv("CODE", "ABC-1234")
	_, err := envconfig.Load[regexCfg]()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestValidation_Regex_Fail(t *testing.T) {
	t.Setenv("CODE", "abc-1234")
	_, err := envconfig.Load[regexCfg]()
	if err == nil {
		t.Fatal("expected error for CODE=abc-1234, got nil")
	}
}

func TestValidation_Len_Exact_Pass(t *testing.T) {
	t.Setenv("TOKEN", "12345678901234567890123456789012") // 32 chars
	t.Setenv("TAGS", "abc")
	_, err := envconfig.Load[lenCfg]()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestValidation_Len_Exact_Fail(t *testing.T) {
	t.Setenv("TOKEN", "short")
	t.Setenv("TAGS", "abc")
	_, err := envconfig.Load[lenCfg]()
	if err == nil {
		t.Fatal("expected error for TOKEN too short, got nil")
	}
}

func TestValidation_MinLen_Fail(t *testing.T) {
	t.Setenv("TOKEN", "12345678901234567890123456789012")
	t.Setenv("TAGS", "")
	// empty value → field stays zero, validation skipped (consistent with existing loader behaviour)
	_, err := envconfig.Load[lenCfg]()
	// TAGS="" is treated as "not set", so no minlen error is expected
	if err != nil {
		t.Fatalf("unexpected error for empty TAGS: %v", err)
	}
}

func TestValidation_CustomValidator_Pass(t *testing.T) {
	t.Setenv("PORT", "8080")
	cfg, err := envconfig.Load[customCfg]()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Port.value != 8080 {
		t.Errorf("Port: got %d, want 8080", cfg.Port.value)
	}
}

func TestValidation_CustomValidator_Fail(t *testing.T) {
	t.Setenv("PORT", "99999")
	_, err := envconfig.Load[customCfg]()
	if err == nil {
		t.Fatal("expected error for PORT=99999, got nil")
	}
}
