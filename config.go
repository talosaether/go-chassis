package chassis

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"
)

// ConfigData holds the parsed configuration as a nested map.
// Modules access their config sections by name.
type ConfigData map[string]any

// envVarPattern matches ${VAR} or ${VAR:-default} patterns
var envVarPattern = regexp.MustCompile(`\$\{([^}:]+)(?::-([^}]*))?\}`)

// LoadConfig reads a YAML config file and returns the parsed configuration.
// Environment variables in the format ${VAR} or ${VAR:-default} are expanded.
func LoadConfig(path string) (ConfigData, error) {
	data, err := os.ReadFile(filepath.Clean(path))
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	// Expand environment variables before parsing
	expanded := expandEnvVars(string(data))

	var config ConfigData
	if err := yaml.Unmarshal([]byte(expanded), &config); err != nil {
		return nil, fmt.Errorf("failed to parse config file: %w", err)
	}

	return config, nil
}

// expandEnvVars replaces ${VAR} and ${VAR:-default} patterns with environment values.
func expandEnvVars(content string) string {
	return envVarPattern.ReplaceAllStringFunc(content, func(match string) string {
		parts := envVarPattern.FindStringSubmatch(match)
		if len(parts) < 2 {
			return match
		}

		varName := parts[1]
		defaultVal := ""
		if len(parts) >= 3 {
			defaultVal = parts[2]
		}

		if val, exists := os.LookupEnv(varName); exists {
			return val
		}
		return defaultVal
	})
}

// Get retrieves a value from the config using dot notation (e.g., "storage.local.base_path").
// Returns nil if the path doesn't exist.
func (cfg ConfigData) Get(path string) any {
	parts := strings.Split(path, ".")
	var current any = map[string]any(cfg)

	for _, part := range parts {
		currentMap := toStringMap(current)
		if currentMap == nil {
			return nil
		}
		var ok bool
		current, ok = currentMap[part]
		if !ok {
			return nil
		}
	}

	return current
}

// toStringMap converts various map types to map[string]any.
// YAML unmarshals nested maps as map[string]interface{}, which requires this helper.
func toStringMap(val any) map[string]any {
	switch typed := val.(type) {
	case map[string]any:
		return typed
	case ConfigData:
		return map[string]any(typed)
	default:
		return nil
	}
}

// GetString retrieves a string value, returning empty string if not found or wrong type.
func (cfg ConfigData) GetString(path string) string {
	val := cfg.Get(path)
	if str, ok := val.(string); ok {
		return str
	}
	return ""
}

// GetInt retrieves an int value, returning 0 if not found or wrong type.
func (cfg ConfigData) GetInt(path string) int {
	val := cfg.Get(path)
	switch typed := val.(type) {
	case int:
		return typed
	case int64:
		return int(typed)
	case float64:
		return int(typed)
	}
	return 0
}

// GetBool retrieves a bool value, returning false if not found or wrong type.
func (cfg ConfigData) GetBool(path string) bool {
	val := cfg.Get(path)
	if boolean, ok := val.(bool); ok {
		return boolean
	}
	return false
}

// Section returns a subsection of the config as ConfigData.
// Returns nil if the section doesn't exist.
func (cfg ConfigData) Section(name string) ConfigData {
	val := cfg.Get(name)
	if asMap := toStringMap(val); asMap != nil {
		return ConfigData(asMap)
	}
	return nil
}
