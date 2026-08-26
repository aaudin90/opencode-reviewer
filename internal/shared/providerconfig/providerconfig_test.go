package providerconfig

import (
	"encoding/json"
	"maps"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseHeaders(t *testing.T) {
	headers, err := ParseHeaders([]string{
		"x-request-source=review-workflow",
		"X-Request-Context=repository:example,task:123=attempt:2",
		"X-REQUEST-SOURCE=other-workflow",
	})
	if err != nil {
		t.Fatalf("ParseHeaders: %v", err)
	}

	want := map[string]string{
		"X-Request-Source":  "other-workflow",
		"X-Request-Context": "repository:example,task:123=attempt:2",
	}
	if !maps.Equal(headers, want) {
		t.Fatalf("ParseHeaders = %v, want %v", headers, want)
	}
}

func TestParseHeadersRejectsInvalidHTTPFields(t *testing.T) {
	tests := []struct {
		name string
		spec string
	}{
		{name: "missing separator", spec: "X-Request-Source"},
		{name: "empty name", spec: "=value"},
		{name: "space in name", spec: "Bad Header=value"},
		{name: "colon in name", spec: "Bad:Header=value"},
		{name: "newline after name", spec: "X-Request-Source\n=value"},
		{name: "newline in value", spec: "X-Request-Source=review\nworkflow"},
		{name: "newline after value", spec: "X-Request-Source=review-workflow\n"},
		{name: "carriage return in value", spec: "X-Request-Source=review\rworkflow"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := ParseHeaders([]string{tt.spec}); err == nil {
				t.Fatalf("ParseHeaders(%q) succeeded, want error", tt.spec)
			}
		})
	}
}

func TestParseHeadersDoesNotExposeInvalidValue(t *testing.T) {
	const secret = "must-not-appear-in-errors"

	_, err := ParseHeaders([]string{"X-Private-Metadata=" + secret + "\ncontinuation"})
	if err == nil {
		t.Fatal("ParseHeaders succeeded, want error")
	}
	if strings.Contains(err.Error(), secret) {
		t.Fatalf("ParseHeaders error exposes header value: %v", err)
	}
}

func TestMergeHeadersAppliesToEveryProvider(t *testing.T) {
	data := json.RawMessage(`{
		"provider": {
			"primary": {
				"options": {
					"baseURL": "https://proxy.example.com/v1",
					"headers": {
						"Authorization": "Bearer token",
						"x-request-source": "old-source"
					}
				},
				"models": {"model": {}}
			},
			"fallback": {
				"models": {"model": {}}
			}
		},
		"model": "primary/model"
	}`)

	merged, err := MergeHeaders(data, map[string]string{
		"X-Request-Source":  "review-workflow",
		"X-Request-Context": "repository:example,task:123",
	})
	if err != nil {
		t.Fatalf("MergeHeaders: %v", err)
	}

	var generated map[string]any
	if err := json.Unmarshal(merged, &generated); err != nil {
		t.Fatalf("parse merged config: %v", err)
	}
	providers := generated["provider"].(map[string]any)
	for _, providerName := range []string{"primary", "fallback"} {
		provider := providers[providerName].(map[string]any)
		options := provider["options"].(map[string]any)
		headers := options["headers"].(map[string]any)

		if headers["X-Request-Source"] != "review-workflow" {
			t.Errorf("%s X-Request-Source = %v, want review-workflow", providerName, headers["X-Request-Source"])
		}
		if headers["X-Request-Context"] != "repository:example,task:123" {
			t.Errorf("%s X-Request-Context = %v, want repository:example,task:123", providerName, headers["X-Request-Context"])
		}
	}

	primary := providers["primary"].(map[string]any)
	primaryOptions := primary["options"].(map[string]any)
	primaryHeaders := primaryOptions["headers"].(map[string]any)
	if primaryOptions["baseURL"] != "https://proxy.example.com/v1" {
		t.Errorf("primary baseURL = %v, want preserved value", primaryOptions["baseURL"])
	}
	if primaryHeaders["Authorization"] != "Bearer token" {
		t.Errorf("primary Authorization = %v, want preserved value", primaryHeaders["Authorization"])
	}
	if _, exists := primaryHeaders["x-request-source"]; exists {
		t.Errorf("primary headers retain overridden casing: %v", primaryHeaders)
	}
}

func TestMergeHeadersRequiresProviderConfig(t *testing.T) {
	_, err := MergeHeaders(nil, map[string]string{"X-Request-Source": "review-workflow"})
	if err == nil || !strings.Contains(err.Error(), "provider config") {
		t.Fatalf("MergeHeaders error = %v, want missing provider config error", err)
	}
}

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

func TestLoadWithOptions_DisablesLegacyEnv(t *testing.T) {
	envVal := `{
		"provider": {"env-provider": {"models": {"m": {}}}},
		"model": "env-provider/m"
	}`
	fileVal := `{
		"provider": {"file-provider": {"models": {"m": {}}}},
		"model": "file-provider/m"
	}`
	dir := t.TempDir()
	path := filepath.Join(dir, "provider.json")
	if err := os.WriteFile(path, []byte(fileVal), 0o600); err != nil {
		t.Fatal(err)
	}

	t.Setenv("OR_PROVIDER_CONFIG", envVal)
	t.Setenv("OR_PROVIDER_CONFIG_PATH", "")

	data, err := LoadWithOptions(path, Options{UseLegacyEnv: false})
	if err != nil {
		t.Fatalf("LoadWithOptions: %v", err)
	}

	var cfg configShape
	if err := json.Unmarshal(data, &cfg); err != nil {
		t.Fatal(err)
	}
	if _, ok := cfg.Provider["file-provider"]; !ok {
		t.Error("expected file-provider, legacy env should be ignored")
	}
}

func TestLoadWithOptions_LegacyEnvFallback(t *testing.T) {
	envVal := `{
		"provider": {"env-provider": {"models": {"m": {}}}},
		"model": "env-provider/m"
	}`
	fileVal := `{
		"provider": {"file-provider": {"models": {"m": {}}}},
		"model": "file-provider/m"
	}`
	dir := t.TempDir()
	path := filepath.Join(dir, "provider.json")
	if err := os.WriteFile(path, []byte(fileVal), 0o600); err != nil {
		t.Fatal(err)
	}

	t.Setenv("OR_PROVIDER_CONFIG", envVal)
	t.Setenv("OR_PROVIDER_CONFIG_PATH", "")

	data, err := LoadWithOptions(path, Options{UseLegacyEnv: true, LegacyEnvFallback: true})
	if err != nil {
		t.Fatalf("LoadWithOptions: %v", err)
	}

	var cfg configShape
	if err := json.Unmarshal(data, &cfg); err != nil {
		t.Fatal(err)
	}
	if _, ok := cfg.Provider["file-provider"]; !ok {
		t.Error("expected file-provider, fallback env should not override config path")
	}
}

func TestDefaultModel(t *testing.T) {
	data := json.RawMessage(`{
		"provider": {"p": {"models": {"m": {}}}},
		"model": "p/m"
	}`)

	model, err := DefaultModel(data)
	if err != nil {
		t.Fatalf("DefaultModel() error = %v", err)
	}
	if model != "p/m" {
		t.Fatalf("DefaultModel() = %q, want p/m", model)
	}
}

func TestDefaultModel_Empty(t *testing.T) {
	model, err := DefaultModel(nil)
	if err != nil {
		t.Fatalf("DefaultModel() error = %v", err)
	}
	if model != "" {
		t.Fatalf("DefaultModel() = %q, want empty", model)
	}
}

func TestDefaultModel_InvalidJSON(t *testing.T) {
	if _, err := DefaultModel(json.RawMessage(`not json`)); err == nil {
		t.Fatal("DefaultModel() error = nil, want error")
	}
}
