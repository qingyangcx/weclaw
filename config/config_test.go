package config

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestAgentConfigUnmarshalEnv(t *testing.T) {
	var cfg Config
	data := []byte(`{
		"agents": {
			"claude": {
				"type": "cli",
				"command": "claude",
				"env": {
					"ANTHROPIC_API_KEY": "test-key",
					"EMPTY": ""
				}
			}
		}
	}`)

	if err := json.Unmarshal(data, &cfg); err != nil {
		t.Fatalf("unmarshal config: %v", err)
	}

	ag, ok := cfg.Agents["claude"]
	if !ok {
		t.Fatalf("expected claude agent config")
	}
	if got := ag.Env["ANTHROPIC_API_KEY"]; got != "test-key" {
		t.Fatalf("ANTHROPIC_API_KEY = %q, want %q", got, "test-key")
	}
	if got, ok := ag.Env["EMPTY"]; !ok || got != "" {
		t.Fatalf("EMPTY = %q, present=%v; want empty string present", got, ok)
	}
}

func TestAgentConfigMarshalEnvRoundTrip(t *testing.T) {
	cfg := Config{
		Agents: map[string]AgentConfig{
			"claude": {
				Type:    "cli",
				Command: "claude",
				Env: map[string]string{
					"ANTHROPIC_API_KEY": "test-key",
					"EMPTY":             "",
				},
			},
		},
	}

	data, err := json.Marshal(cfg)
	if err != nil {
		t.Fatalf("marshal config: %v", err)
	}

	var decoded Config
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("round-trip unmarshal: %v", err)
	}

	got := decoded.Agents["claude"].Env
	if got["ANTHROPIC_API_KEY"] != "test-key" || got["EMPTY"] != "" {
		t.Fatalf("round-trip env = %#v", got)
	}
}

func TestAgentConfigWithoutEnvStillLoads(t *testing.T) {
	var cfg Config
	data := []byte(`{
		"agents": {
			"claude": {
				"type": "cli",
				"command": "claude"
			}
		}
	}`)

	if err := json.Unmarshal(data, &cfg); err != nil {
		t.Fatalf("unmarshal config without env: %v", err)
	}

	if cfg.Agents["claude"].Env != nil {
		t.Fatalf("Env = %#v, want nil", cfg.Agents["claude"].Env)
	}
}

func TestDefaultConfigInitializesAgentsMap(t *testing.T) {
	cfg := DefaultConfig()
	if cfg.Agents == nil {
		t.Fatal("DefaultConfig() Agents = nil, want initialized map")
	}
}

func TestLoadEnvOverridesTopLevelOnly(t *testing.T) {
	t.Setenv("WECLAW_DEFAULT_AGENT", "codex")
	t.Setenv("WECLAW_API_ADDR", "127.0.0.1:18011")

	cfg := DefaultConfig()
	cfg.Agents["claude"] = AgentConfig{
		Type: "cli",
		Env: map[string]string{
			"KEEP": "value",
		},
	}

	loadEnv(cfg)

	if cfg.DefaultAgent != "codex" {
		t.Fatalf("DefaultAgent = %q, want %q", cfg.DefaultAgent, "codex")
	}
	if cfg.APIAddr != "127.0.0.1:18011" {
		t.Fatalf("APIAddr = %q, want %q", cfg.APIAddr, "127.0.0.1:18011")
	}
	if got := cfg.Agents["claude"].Env["KEEP"]; got != "value" {
		t.Fatalf("agent env = %q, want preserved value", got)
	}
}

func TestEmailAlertAbsent(t *testing.T) {
	cfg := DefaultConfig()
	got, err := cfg.EmailAlert()
	if err != nil || got != nil {
		t.Fatalf("EmailAlert() = %+v, %v; want nil, nil", got, err)
	}
}

func TestEmailAlertFromJSONWithEnvOverrides(t *testing.T) {
	var cfg Config
	data := []byte(`{
		"alert": {
			"email": {
				"enabled": true,
				"smtp_host": "smtp.example.com",
				"username": "bot@example.com",
				"password": "from-file",
				"to": ["ops@example.com"]
			}
		}
	}`)
	if err := json.Unmarshal(data, &cfg); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	t.Setenv("WECLAW_ALERT_SMTP_PASSWORD", "from-env")
	t.Setenv("WECLAW_ALERT_SMTP_PORT", "465")
	t.Setenv("WECLAW_ALERT_TO", "a@example.com, b@example.com")

	got, err := cfg.EmailAlert()
	if err != nil {
		t.Fatalf("EmailAlert: %v", err)
	}
	if got.SMTPHost != "smtp.example.com" || got.SMTPPort != 465 || got.Password != "from-env" ||
		got.From != "bot@example.com" || strings.Join(got.To, ",") != "a@example.com,b@example.com" {
		t.Fatalf("unexpected config: %+v", got)
	}
	// Env overrides must not leak into the saved config.
	if cfg.Alert.Email.Password != "from-file" || len(cfg.Alert.Email.To) != 1 {
		t.Fatalf("env overrides mutated config: %+v", cfg.Alert.Email)
	}
}

func TestEmailAlertDefaultsAndValidation(t *testing.T) {
	cfg := &Config{Alert: &AlertConfig{Email: &EmailAlertConfig{
		Enabled: true, SMTPHost: "smtp.example.com", From: "a@example.com", To: []string{"b@example.com"},
	}}}
	got, err := cfg.EmailAlert()
	if err != nil || got.SMTPPort != 587 {
		t.Fatalf("EmailAlert() = %+v, %v; want port 587", got, err)
	}

	cfg.Alert.Email.To = nil
	if _, err := cfg.EmailAlert(); err == nil {
		t.Fatal("expected error when 'to' is empty")
	}

	cfg.Alert.Email.Enabled = false
	if got, err := cfg.EmailAlert(); got != nil || err != nil {
		t.Fatalf("disabled config: got %+v, %v", got, err)
	}
}

func TestEmailAlertEnvOnly(t *testing.T) {
	t.Setenv("WECLAW_ALERT_ENABLED", "true")
	t.Setenv("WECLAW_ALERT_SMTP_HOST", "smtp.example.com")
	t.Setenv("WECLAW_ALERT_SMTP_USERNAME", "bot@example.com")
	t.Setenv("WECLAW_ALERT_TO", "ops@example.com")

	got, err := DefaultConfig().EmailAlert()
	if err != nil || got == nil || got.From != "bot@example.com" {
		t.Fatalf("EmailAlert() = %+v, %v", got, err)
	}
}
