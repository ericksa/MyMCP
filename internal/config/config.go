package config

import (
	"fmt"
	"os"

	"github.com/spf13/viper"
)

type Config struct {
	ServerAddr     string
	JWTSecret      string
	AllowedOrigins []string
	Workers        WorkerConfig
	LLM            LLMConfig
}

type WorkerConfig struct {
	EnableFileIO   bool
	EnableSQLite   bool
	EnableVector   bool
	EnableExternal bool
	BasePath       string
}

type LLMConfig struct {
	Provider string
	Endpoint string
	Model    string
	APIKey   string
}

func Load() (*Config, error) {
	viper.SetConfigName("config")
	viper.SetConfigType("yaml")
	viper.AddConfigPath(".")
	viper.AddConfigPath("$HOME/.mymcp")
	viper.AutomaticEnv()

	defaults()
	if err := viper.ReadInConfig(); err != nil {
		if _, ok := err.(viper.ConfigFileNotFoundError); !ok {
			return nil, fmt.Errorf("failed to read config: %w", err)
		}
	}

	cfg := &Config{
		ServerAddr:     viper.GetString("server.addr"),
		JWTSecret:      viper.GetString("auth.jwt_secret"),
		AllowedOrigins: viper.GetStringSlice("server.allowed_origins"),
		Workers: WorkerConfig{
			EnableFileIO:   viper.GetBool("workers.enable_file_io"),
			EnableSQLite:   viper.GetBool("workers.enable_sqlite"),
			EnableVector:   viper.GetBool("workers.enable_vector"),
			EnableExternal: viper.GetBool("workers.enable_external"),
			BasePath:       viper.GetString("workers.base_path"),
		},
		LLM: LLMConfig{
			Provider: viper.GetString("llm.provider"),
			Endpoint: viper.GetString("llm.endpoint"),
			Model:    viper.GetString("llm.model"),
			APIKey:   viper.GetString("llm.api_key"),
		},
	}
	return cfg, nil
}

func defaults() {
	viper.SetDefault("server.addr", "localhost:8080")
	viper.SetDefault("server.allowed_origins", []string{"*"})
	viper.SetDefault("auth.jwt_secret", "dev-secret-change-in-prod")
	viper.SetDefault("workers.enable_file_io", true)
	viper.SetDefault("workers.enable_sqlite", true)
	viper.SetDefault("workers.enable_vector", false)
	viper.SetDefault("workers.enable_external", false)
	viper.SetDefault("workers.base_path", os.Getenv("HOME"))
	viper.SetDefault("llm.provider", "ollama")
	viper.SetDefault("llm.endpoint", "http://localhost:11434")
	viper.SetDefault("llm.model", "llama3")
}
