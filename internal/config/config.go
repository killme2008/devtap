package config

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/BurntSushi/toml"
)

// Config represents the devtap configuration file.
type Config struct {
	Store StoreConfig `toml:"store"`
}

// StoreConfig configures the storage backend.
type StoreConfig struct {
	Backend    string           `toml:"backend"` // "file" (default) | "greptimedb"
	GreptimeDB GreptimeDBConfig `toml:"greptimedb"`
}

// GreptimeDBConfig holds GreptimeDB connection settings.
type GreptimeDBConfig struct {
	Endpoint      string `toml:"endpoint"`       // gRPC endpoint for ingestion (default "127.0.0.1:4001")
	MySQLEndpoint string `toml:"mysql_endpoint"` // MySQL protocol endpoint for queries (default "127.0.0.1:4002")
	Database      string `toml:"database"`       // database name (default "devtap")
}

// Default returns the default configuration.
func Default() *Config {
	return &Config{
		Store: StoreConfig{
			Backend: "file",
			GreptimeDB: GreptimeDBConfig{
				Endpoint:      "127.0.0.1:4001",
				MySQLEndpoint: "127.0.0.1:4002",
				Database:      "devtap",
			},
		},
	}
}

// Load reads the config from the default path (~/.devtap/config.toml),
// falling back to defaults if the file doesn't exist.
func Load() (*Config, error) {
	return LoadFrom(defaultConfigPath())
}

// LoadFrom reads the config from the given path.
func LoadFrom(path string) (*Config, error) {
	cfg := Default()

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return cfg, nil // use defaults
		}
		return nil, fmt.Errorf("read config: %w", err)
	}

	if err := toml.Unmarshal(data, cfg); err != nil {
		return nil, fmt.Errorf("parse config %s: %w", path, err)
	}

	return cfg, nil
}

// AuthToken returns the GreptimeDB auth token from the environment.
func (c *GreptimeDBConfig) AuthToken() string {
	return os.Getenv("DEVTAP_GREPTIMEDB_AUTH_TOKEN")
}

// Username returns the GreptimeDB username from the environment.
func (c *GreptimeDBConfig) Username() string {
	return os.Getenv("DEVTAP_GREPTIMEDB_USERNAME")
}

// Password returns the GreptimeDB password from the environment.
func (c *GreptimeDBConfig) Password() string {
	return os.Getenv("DEVTAP_GREPTIMEDB_PASSWORD")
}

func defaultConfigPath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".devtap", "config.toml")
}
