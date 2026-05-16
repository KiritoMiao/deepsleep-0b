package appconfig

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
)

type Config struct {
	Domain string `json:"domain"`
}

func LoadFile(path string) (Config, error) {
	file, err := os.Open(path)
	if err != nil {
		return Config{}, fmt.Errorf("config: open %s: %w", path, err)
	}
	defer file.Close()

	config, err := Load(file)
	if err != nil {
		return Config{}, fmt.Errorf("config: %s: %w", path, err)
	}
	return config, nil
}

func Load(r io.Reader) (Config, error) {
	var config Config
	decoder := json.NewDecoder(r)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&config); err != nil {
		return Config{}, fmt.Errorf("decode: %w", err)
	}
	if err := Validate(config); err != nil {
		return Config{}, err
	}
	return config, nil
}

func Validate(config Config) error {
	domain := strings.TrimSpace(config.Domain)
	if domain == "" {
		return fmt.Errorf("domain is required")
	}
	if strings.Contains(domain, "://") || strings.Contains(domain, "/") {
		return fmt.Errorf("domain must be a host name without scheme or path")
	}
	return nil
}
