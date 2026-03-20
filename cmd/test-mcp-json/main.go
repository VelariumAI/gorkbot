package main

import (
	"encoding/json"
	"fmt"
	"os"
)

type ServerConfig struct {
	Name      string            `json:"name"`
	Transport string            `json:"transport,omitempty"`
	Command   string            `json:"command"`
	Args      []string          `json:"args"`
	Env       map[string]string `json:"env,omitempty"`
	Disabled  bool              `json:"disabled,omitempty"`
}

type Config struct {
	Servers []ServerConfig `json:"servers"`
}

func main() {
	path := os.ExpandEnv("$HOME/.config/gorkbot/mcp.json")
	data, err := os.ReadFile(path)
	if err != nil {
		fmt.Printf("Read error: %v\n", err)
		return
	}

	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		fmt.Printf("Unmarshal error: %v\n", err)
		return
	}

	fmt.Printf("Successfully unmarshaled %d servers\n", len(cfg.Servers))
	for _, s := range cfg.Servers {
		fmt.Printf("- %s: %v\n", s.Name, s.Env)
	}
}
