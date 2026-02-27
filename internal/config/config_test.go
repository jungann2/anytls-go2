package config

import (
	"os"
	"reflect"
	"testing"

	"github.com/leanovate/gopter"
	"github.com/leanovate/gopter/gen"
	"github.com/leanovate/gopter/prop"
	"gopkg.in/yaml.v3"
)

// Feature: anytls-xboard-integration, Property 7: 配置 round-trip
// **Validates: Requirements 6.1**

// genNonEmptyString generates a non-empty alphanumeric string.
func genNonEmptyString() gopter.Gen {
	return gen.AlphaString().SuchThat(func(s string) bool {
		return len(s) > 0
	})
}

// genConfig generates a random valid Config struct.
func genConfig() gopter.Gen {
	return gopter.CombineGens(
		gen.AlphaString(),       // Listen
		genNonEmptyString(),     // APIHost (non-empty)
		genNonEmptyString(),     // APIToken (non-empty)
		gen.IntRange(1, 100000), // NodeID (> 0)
		gen.AlphaString(),       // NodeType
		gen.AlphaString(),       // TLS.CertFile
		gen.AlphaString(),       // TLS.KeyFile
		gen.AnyString(),         // Log.Level
		gen.AnyString(),         // Log.FilePath
		gen.AlphaString(),       // Fallback
	).Map(func(values []interface{}) Config {
		return Config{
			Listen:   values[0].(string),
			APIHost:  values[1].(string),
			APIToken: values[2].(string),
			NodeID:   values[3].(int),
			NodeType: values[4].(string),
			TLS: TLSConfig{
				CertFile: values[5].(string),
				KeyFile:  values[6].(string),
			},
			Log: LogConfig{
				Level:    values[7].(string),
				FilePath: values[8].(string),
			},
			Fallback: values[9].(string),
		}
	})
}

func TestProperty7_ConfigRoundTrip(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 100

	properties := gopter.NewProperties(parameters)

	properties.Property("Config round-trip through YAML serialization", prop.ForAll(
		func(cfg Config) bool {
			// Marshal to YAML
			data, err := yaml.Marshal(&cfg)
			if err != nil {
				t.Logf("marshal error: %v", err)
				return false
			}

			// Unmarshal back
			var restored Config
			if err := yaml.Unmarshal(data, &restored); err != nil {
				t.Logf("unmarshal error: %v", err)
				return false
			}

			// Compare
			return reflect.DeepEqual(cfg, restored)
		},
		genConfig(),
	))

	properties.TestingRun(t)
}

// --- Unit Tests for LoadConfig ---
// Validates: Requirements 6.2

func TestLoadConfig_Valid(t *testing.T) {
	content := `
listen: "0.0.0.0:9443"
api_host: "https://panel.example.com"
api_token: "test-token-123"
node_id: 42
node_type: "anytls"
tls:
  cert_file: "/etc/anytls/cert.pem"
  key_file: "/etc/anytls/key.pem"
log:
  level: "debug"
  file_path: "/var/log/anytls.log"
fallback: "127.0.0.1:80"
`
	f, err := os.CreateTemp("", "config-valid-*.yaml")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(f.Name())

	if _, err := f.WriteString(content); err != nil {
		t.Fatal(err)
	}
	f.Close()

	cfg, err := LoadConfig(f.Name())
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}

	if cfg.Listen != "0.0.0.0:9443" {
		t.Errorf("Listen = %q, want %q", cfg.Listen, "0.0.0.0:9443")
	}
	if cfg.APIHost != "https://panel.example.com" {
		t.Errorf("APIHost = %q, want %q", cfg.APIHost, "https://panel.example.com")
	}
	if cfg.APIToken != "test-token-123" {
		t.Errorf("APIToken = %q, want %q", cfg.APIToken, "test-token-123")
	}
	if cfg.NodeID != 42 {
		t.Errorf("NodeID = %d, want %d", cfg.NodeID, 42)
	}
	if cfg.NodeType != "anytls" {
		t.Errorf("NodeType = %q, want %q", cfg.NodeType, "anytls")
	}
	if cfg.TLS.CertFile != "/etc/anytls/cert.pem" {
		t.Errorf("TLS.CertFile = %q, want %q", cfg.TLS.CertFile, "/etc/anytls/cert.pem")
	}
	if cfg.TLS.KeyFile != "/etc/anytls/key.pem" {
		t.Errorf("TLS.KeyFile = %q, want %q", cfg.TLS.KeyFile, "/etc/anytls/key.pem")
	}
	if cfg.Log.Level != "debug" {
		t.Errorf("Log.Level = %q, want %q", cfg.Log.Level, "debug")
	}
	if cfg.Log.FilePath != "/var/log/anytls.log" {
		t.Errorf("Log.FilePath = %q, want %q", cfg.Log.FilePath, "/var/log/anytls.log")
	}
	if cfg.Fallback != "127.0.0.1:80" {
		t.Errorf("Fallback = %q, want %q", cfg.Fallback, "127.0.0.1:80")
	}
}

func TestLoadConfig_MissingAPIHost(t *testing.T) {
	content := `
api_host: ""
api_token: "token"
node_id: 1
`
	f, err := os.CreateTemp("", "config-nohost-*.yaml")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(f.Name())
	f.WriteString(content)
	f.Close()

	_, err = LoadConfig(f.Name())
	if err == nil {
		t.Fatal("expected error for missing api_host, got nil")
	}
}

func TestLoadConfig_MissingAPIToken(t *testing.T) {
	content := `
api_host: "https://panel.example.com"
api_token: ""
node_id: 1
`
	f, err := os.CreateTemp("", "config-notoken-*.yaml")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(f.Name())
	f.WriteString(content)
	f.Close()

	_, err = LoadConfig(f.Name())
	if err == nil {
		t.Fatal("expected error for missing api_token, got nil")
	}
}

func TestLoadConfig_MissingNodeID(t *testing.T) {
	content := `
api_host: "https://panel.example.com"
api_token: "token"
node_id: 0
`
	f, err := os.CreateTemp("", "config-nonodeid-*.yaml")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(f.Name())
	f.WriteString(content)
	f.Close()

	_, err = LoadConfig(f.Name())
	if err == nil {
		t.Fatal("expected error for node_id=0, got nil")
	}
}

func TestLoadConfig_InvalidYAML(t *testing.T) {
	content := `{{{not valid yaml:::}`
	f, err := os.CreateTemp("", "config-invalid-*.yaml")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(f.Name())
	f.WriteString(content)
	f.Close()

	_, err = LoadConfig(f.Name())
	if err == nil {
		t.Fatal("expected error for invalid YAML, got nil")
	}
}

func TestLoadConfig_FileNotFound(t *testing.T) {
	_, err := LoadConfig("/nonexistent/path/config.yaml")
	if err == nil {
		t.Fatal("expected error for non-existent file, got nil")
	}
}

func TestLoadConfig_Defaults(t *testing.T) {
	content := `
api_host: "https://panel.example.com"
api_token: "token"
node_id: 1
`
	f, err := os.CreateTemp("", "config-defaults-*.yaml")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(f.Name())
	f.WriteString(content)
	f.Close()

	cfg, err := LoadConfig(f.Name())
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}

	if cfg.Listen != "0.0.0.0:8443" {
		t.Errorf("default Listen = %q, want %q", cfg.Listen, "0.0.0.0:8443")
	}
	if cfg.NodeType != "anytls" {
		t.Errorf("default NodeType = %q, want %q", cfg.NodeType, "anytls")
	}
	if cfg.Log.Level != "info" {
		t.Errorf("default Log.Level = %q, want %q", cfg.Log.Level, "info")
	}
}
