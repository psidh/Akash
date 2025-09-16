package config

import (
	core "Akash/core"
	"encoding/json"
	"os"
)

func LoadConfig(path string) (*core.UserConfig, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	var Config core.UserConfig
	if err := json.NewDecoder(file).Decode(&Config); err != nil {
		return nil, err
	}
	return &Config, nil
}
