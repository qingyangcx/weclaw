package config

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// Config holds the application configuration.
type Config struct {
	DefaultAgent string                 `json:"default_agent"`
	APIAddr      string                 `json:"api_addr,omitempty"`
	SaveDir      string                 `json:"save_dir,omitempty"`
	Agents       map[string]AgentConfig `json:"agents"`
	Alert        *AlertConfig           `json:"alert,omitempty"`
}

// AlertConfig configures operator alerts on errors.
type AlertConfig struct {
	Email *EmailAlertConfig `json:"email,omitempty"`
}

// EmailAlertConfig configures email alerts sent via SMTP.
type EmailAlertConfig struct {
	Enabled      bool     `json:"enabled"`
	SMTPHost     string   `json:"smtp_host"`
	SMTPPort     int      `json:"smtp_port,omitempty"` // default 587; 465 = implicit TLS
	Username     string   `json:"username,omitempty"`
	Password     string   `json:"password,omitempty"`      // prefer password_file or WECLAW_ALERT_SMTP_PASSWORD
	PasswordFile string   `json:"password_file,omitempty"` // read at startup; env password > password_file > password
	From         string   `json:"from,omitempty"`          // default: username
	To           []string `json:"to"`
}

// AgentConfig holds configuration for a single agent.
type AgentConfig struct {
	Type         string            `json:"type"`                    // "acp", "cli", or "http"
	Command      string            `json:"command,omitempty"`       // binary path (cli/acp type)
	Args         []string          `json:"args,omitempty"`          // extra args for command (e.g. ["acp"] for cursor)
	Aliases      []string          `json:"aliases,omitempty"`       // custom trigger commands (e.g. ["gpt", "4o"])
	Cwd          string            `json:"cwd,omitempty"`           // working directory (workspace)
	Env          map[string]string `json:"env,omitempty"`           // extra environment variables (cli/acp type)
	Model        string            `json:"model,omitempty"`         // model name
	SystemPrompt string            `json:"system_prompt,omitempty"` // system prompt
	Endpoint     string            `json:"endpoint,omitempty"`      // API endpoint (http type)
	APIKey       string            `json:"api_key,omitempty"`       // API key (http type)
	Headers      map[string]string `json:"headers,omitempty"`       // extra HTTP headers (http type)
	MaxHistory   int               `json:"max_history,omitempty"`   // max history (http type)
}

// BuildAliasMap builds a map from custom alias to agent name from all agent configs.
// It logs warnings for conflicts: duplicate aliases and aliases shadowing agent keys.
func BuildAliasMap(agents map[string]AgentConfig) map[string]string {
	// Built-in commands that cannot be overridden
	reserved := map[string]bool{
		"info": true, "help": true, "new": true, "clear": true, "cwd": true,
	}

	m := make(map[string]string)
	for name, cfg := range agents {
		for _, alias := range cfg.Aliases {
			if reserved[alias] {
				log.Printf("[config] WARNING: alias %q for agent %q conflicts with built-in command, ignored", alias, name)
				continue
			}
			if existing, ok := m[alias]; ok {
				log.Printf("[config] WARNING: alias %q is defined by both %q and %q, using %q", alias, existing, name, name)
			}
			m[alias] = name
		}
	}

	// Warn if a custom alias shadows an agent key
	for alias, target := range m {
		if _, isAgent := agents[alias]; isAgent && alias != target {
			log.Printf("[config] WARNING: alias %q (-> %q) shadows agent key %q", alias, target, alias)
		}
	}

	return m
}

// DefaultConfig returns an empty configuration.
func DefaultConfig() *Config {
	return &Config{
		Agents: make(map[string]AgentConfig),
	}
}

// ConfigPath returns the path to the config file.
func ConfigPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".weclaw", "config.json"), nil
}

// Load loads configuration from disk and environment variables.
func Load() (*Config, error) {
	cfg := DefaultConfig()

	path, err := ConfigPath()
	if err != nil {
		return cfg, nil
	}

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			loadEnv(cfg)
			return cfg, nil
		}
		return nil, fmt.Errorf("read config: %w", err)
	}

	if err := json.Unmarshal(data, cfg); err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}
	if cfg.Agents == nil {
		cfg.Agents = make(map[string]AgentConfig)
	}

	loadEnv(cfg)
	return cfg, nil
}

func loadEnv(cfg *Config) {
	if v := os.Getenv("WECLAW_DEFAULT_AGENT"); v != "" {
		cfg.DefaultAgent = v
	}
	if v := os.Getenv("WECLAW_API_ADDR"); v != "" {
		cfg.APIAddr = v
	}
	if v := os.Getenv("WECLAW_SAVE_DIR"); v != "" {
		cfg.SaveDir = v
	}
}

// EmailAlert returns the effective email alert config (config file merged with
// WECLAW_ALERT_* env vars), or nil if email alerts are not enabled.
// Env values are applied to a copy so secrets are never written back by Save.
func (c *Config) EmailAlert() (*EmailAlertConfig, error) {
	var e EmailAlertConfig
	if c.Alert != nil && c.Alert.Email != nil {
		e = *c.Alert.Email
		e.To = append([]string(nil), e.To...)
	}
	if v := os.Getenv("WECLAW_ALERT_ENABLED"); v != "" {
		enabled, err := strconv.ParseBool(v)
		if err != nil {
			return nil, fmt.Errorf("invalid WECLAW_ALERT_ENABLED %q: %w", v, err)
		}
		e.Enabled = enabled
	}
	if v := os.Getenv("WECLAW_ALERT_SMTP_HOST"); v != "" {
		e.SMTPHost = v
	}
	if v := os.Getenv("WECLAW_ALERT_SMTP_PORT"); v != "" {
		port, err := strconv.Atoi(v)
		if err != nil {
			return nil, fmt.Errorf("invalid WECLAW_ALERT_SMTP_PORT %q: %w", v, err)
		}
		e.SMTPPort = port
	}
	if v := os.Getenv("WECLAW_ALERT_SMTP_USERNAME"); v != "" {
		e.Username = v
	}
	envPassword := os.Getenv("WECLAW_ALERT_SMTP_PASSWORD")
	if envPassword != "" {
		e.Password = envPassword
	}
	if v := os.Getenv("WECLAW_ALERT_SMTP_PASSWORD_FILE"); v != "" {
		e.PasswordFile = v
	}
	if v := os.Getenv("WECLAW_ALERT_FROM"); v != "" {
		e.From = v
	}
	if v := os.Getenv("WECLAW_ALERT_TO"); v != "" {
		e.To = nil
		for _, addr := range strings.Split(v, ",") {
			if addr = strings.TrimSpace(addr); addr != "" {
				e.To = append(e.To, addr)
			}
		}
	}

	if !e.Enabled {
		return nil, nil
	}
	if envPassword == "" && e.PasswordFile != "" {
		password, err := readPasswordFile(e.PasswordFile)
		if err != nil {
			return nil, err
		}
		e.Password = password
	}
	if e.SMTPPort == 0 {
		e.SMTPPort = 587
	}
	if e.From == "" {
		e.From = e.Username
	}
	if e.SMTPHost == "" || e.From == "" || len(e.To) == 0 {
		return nil, fmt.Errorf("alert.email requires smtp_host, from (or username) and to")
	}
	return &e, nil
}

// readPasswordFile reads a secret from path (leading "~/" expands to the home
// directory) and trims surrounding whitespace.
func readPasswordFile(path string) (string, error) {
	if strings.HasPrefix(path, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("expand password_file %q: %w", path, err)
		}
		path = filepath.Join(home, path[2:])
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("read alert.email password_file: %w", err)
	}
	password := strings.TrimSpace(string(data))
	if password == "" {
		return "", fmt.Errorf("alert.email password_file %s is empty", path)
	}
	return password, nil
}

// Save saves the configuration to disk.
func Save(cfg *Config) error {
	path, err := ConfigPath()
	if err != nil {
		return err
	}

	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("create config dir: %w", err)
	}

	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal config: %w", err)
	}

	return os.WriteFile(path, data, 0o600)
}
