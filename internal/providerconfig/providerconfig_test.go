package providerconfig

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestValidate_Valid(t *testing.T) {
	data := json.RawMessage(`{
		"provider": {
			"my-proxy": {
				"npm": "@ai-sdk/openai-compatible",
				"options": {"baseURL": "https://llm-proxy.example.com/v1"},
				"models": {
					"claude-sonnet": {
						"name": "Claude Sonnet",
						"cost": {"input": 0.003, "output": 0.015},
						"limit": {"context": 200000, "output": 8192}
					}
				}
			}
		},
		"model": "my-proxy/claude-sonnet"
	}`)

	if err := Validate(data); err != nil {
		t.Fatalf("expected valid, got: %v", err)
	}
}

func TestValidate_NoCostLimit(t *testing.T) {
	data := json.RawMessage(`{
		"provider": {
			"p": {
				"models": {"m": {"name": "M"}}
			}
		},
		"model": "p/m"
	}`)

	if err := Validate(data); err != nil {
		t.Fatalf("cost and limit are optional, got: %v", err)
	}
}

func TestValidate_EmptyProvider(t *testing.T) {
	data := json.RawMessage(`{"provider": {}, "model": "p/m"}`)
	if err := Validate(data); err == nil {
		t.Fatal("expected error for empty provider")
	}
}

func TestValidate_NoModels(t *testing.T) {
	data := json.RawMessage(`{
		"provider": {"p": {"models": {}}},
		"model": "p/m"
	}`)
	if err := Validate(data); err == nil {
		t.Fatal("expected error for empty models")
	}
}

func TestValidate_NoModel(t *testing.T) {
	data := json.RawMessage(`{
		"provider": {"p": {"models": {"m": {}}}}
	}`)
	if err := Validate(data); err == nil {
		t.Fatal("expected error for missing model")
	}
}

func TestValidate_NegativeCost(t *testing.T) {
	data := json.RawMessage(`{
		"provider": {"p": {"models": {"m": {"cost": {"input": -1, "output": 0}}}}},
		"model": "p/m"
	}`)
	if err := Validate(data); err == nil {
		t.Fatal("expected error for negative cost")
	}
}

func TestValidate_InvalidLimit(t *testing.T) {
	data := json.RawMessage(`{
		"provider": {"p": {"models": {"m": {"limit": {"context": 0}}}}},
		"model": "p/m"
	}`)
	if err := Validate(data); err == nil {
		t.Fatal("expected error for zero context limit")
	}
}

func TestValidate_LimitWithoutOutput(t *testing.T) {
	data := json.RawMessage(`{
		"provider": {"p": {"models": {"m": {"limit": {"context": 130000}}}}},
		"model": "p/m"
	}`)
	if err := Validate(data); err != nil {
		t.Fatalf("limit.output is optional, got: %v", err)
	}
}

func TestValidate_CacheReadCost(t *testing.T) {
	data := json.RawMessage(`{
		"provider": {"p": {"models": {"m": {"cost": {"input": 0.60, "output": 3.00, "cacheRead": 0.10}}}}},
		"model": "p/m"
	}`)
	if err := Validate(data); err != nil {
		t.Fatalf("cacheRead cost should be accepted, got: %v", err)
	}
}

func TestValidate_InvalidJSON(t *testing.T) {
	data := json.RawMessage(`not json`)
	if err := Validate(data); err == nil {
		t.Fatal("expected error for invalid JSON")
	}
}

func TestLoad_FromEnv(t *testing.T) {
	val := `{
		"provider": {"p": {"models": {"m": {}}}},
		"model": "p/m"
	}`
	t.Setenv("OR_PROVIDER_CONFIG", val)
	t.Setenv("OR_PROVIDER_CONFIG_PATH", "")

	data, err := Load("")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if data == nil {
		t.Fatal("expected non-nil data")
	}
}

func TestLoad_FromFile(t *testing.T) {
	val := `{
		"provider": {"p": {"models": {"m": {}}}},
		"model": "p/m"
	}`
	dir := t.TempDir()
	path := filepath.Join(dir, "provider.json")
	if err := os.WriteFile(path, []byte(val), 0o600); err != nil {
		t.Fatal(err)
	}

	t.Setenv("OR_PROVIDER_CONFIG_PATH", path)
	t.Setenv("OR_PROVIDER_CONFIG", "")

	data, err := Load("")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if data == nil {
		t.Fatal("expected non-nil data")
	}
}

func TestLoad_FileOverridesEnv(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "provider.json")
	fileContent := `{
		"provider": {"file-provider": {"models": {"m": {}}}},
		"model": "file-provider/m"
	}`
	if err := os.WriteFile(path, []byte(fileContent), 0o600); err != nil {
		t.Fatal(err)
	}

	t.Setenv("OR_PROVIDER_CONFIG_PATH", path)
	t.Setenv("OR_PROVIDER_CONFIG", `{"provider": {"env-provider": {"models": {"m": {}}}}, "model": "env-provider/m"}`)

	data, err := Load("")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	var cfg configShape
	if err := json.Unmarshal(data, &cfg); err != nil {
		t.Fatal(err)
	}
	if _, ok := cfg.Provider["file-provider"]; !ok {
		t.Error("expected file-provider, file should take priority over env")
	}
}

func TestLoad_NeitherSet(t *testing.T) {
	t.Setenv("OR_PROVIDER_CONFIG", "")
	t.Setenv("OR_PROVIDER_CONFIG_PATH", "")

	data, err := Load("")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if data != nil {
		t.Fatal("expected nil when neither env is set")
	}
}

func TestLoad_FromConfigPath(t *testing.T) {
	val := `{
		"provider": {"toml-provider": {"models": {"m": {}}}},
		"model": "toml-provider/m"
	}`
	dir := t.TempDir()
	path := filepath.Join(dir, "provider.json")
	if err := os.WriteFile(path, []byte(val), 0o600); err != nil {
		t.Fatal(err)
	}

	t.Setenv("OR_PROVIDER_CONFIG", "")
	t.Setenv("OR_PROVIDER_CONFIG_PATH", "")

	data, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if data == nil {
		t.Fatal("expected non-nil data from configPath")
	}

	var cfg configShape
	if err := json.Unmarshal(data, &cfg); err != nil {
		t.Fatal(err)
	}
	if _, ok := cfg.Provider["toml-provider"]; !ok {
		t.Error("expected toml-provider from configPath fallback")
	}
}

func TestLoad_EnvOverridesConfigPath(t *testing.T) {
	envVal := `{
		"provider": {"env-provider": {"models": {"m": {}}}},
		"model": "env-provider/m"
	}`
	t.Setenv("OR_PROVIDER_CONFIG", envVal)
	t.Setenv("OR_PROVIDER_CONFIG_PATH", "")

	data, err := Load("/nonexistent/path.json")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	var cfg configShape
	if err := json.Unmarshal(data, &cfg); err != nil {
		t.Fatal(err)
	}
	if _, ok := cfg.Provider["env-provider"]; !ok {
		t.Error("expected env-provider, env should take priority over configPath")
	}
}
