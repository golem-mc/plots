package config

import (
	"os"
	"path/filepath"

	"go.yaml.in/yaml/v4"
)

// StorageConfig mirrors upstream PlotSquared's config/storage.yml.
type StorageConfig struct {
	Prefix string       `yaml:"prefix"`
	SQLite SQLiteConfig `yaml:"sqlite"`
	MySQL  MySQLConfig  `yaml:"mysql"`
}

type SQLiteConfig struct {
	Use bool   `yaml:"use"`
	DB  string `yaml:"db"`
}

type MySQLConfig struct {
	Use        bool     `yaml:"use"`
	Host       string   `yaml:"host"`
	Port       string   `yaml:"port"`
	User       string   `yaml:"user"`
	Password   string   `yaml:"password"`
	Database   string   `yaml:"database"`
	Properties []string `yaml:"properties"`
}

// DefaultStorageConfig returns upstream storage.yml defaults (SQLite enabled, "storage").
func DefaultStorageConfig() *StorageConfig {
	return &StorageConfig{
		Prefix: "",
		SQLite: SQLiteConfig{
			Use: true,
			DB:  "storage",
		},
		MySQL: MySQLConfig{
			Use:        false,
			Host:       "localhost",
			Port:       "3306",
			User:       "root",
			Password:   "password",
			Database:   "plot_db",
			Properties: []string{"useSSL=false"},
		},
	}
}

// LoadStorageConfig loads config/storage.yml, creating it with defaults if missing.
func LoadStorageConfig(dataDir string) (*StorageConfig, error) {
	cfgDir := filepath.Join(dataDir, "config")
	if err := os.MkdirAll(cfgDir, 0o755); err != nil {
		return nil, err
	}
	path := filepath.Join(cfgDir, "storage.yml")
	c := DefaultStorageConfig()
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return c, SaveStorageConfig(dataDir, c)
		}
		return nil, err
	}
	if len(data) > 0 {
		if err := yaml.Unmarshal(data, c); err != nil {
			return nil, err
		}
	}
	if c.SQLite.DB == "" {
		c.SQLite.DB = "storage"
	}
	return c, nil
}

// SaveStorageConfig writes config/storage.yml.
func SaveStorageConfig(dataDir string, c *StorageConfig) error {
	cfgDir := filepath.Join(dataDir, "config")
	if err := os.MkdirAll(cfgDir, 0o755); err != nil {
		return err
	}
	out, err := yaml.Marshal(c)
	if err != nil {
		return err
	}
	header := "# PlotSquared database storage configuration (storage.yml)\n"
	return os.WriteFile(filepath.Join(cfgDir, "storage.yml"), append([]byte(header), out...), 0o644)
}
