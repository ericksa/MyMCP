package config

import (
	"github.com/joho/godotenv"
	"github.com/spf13/viper"
	"log"
	"os"
	"path/filepath"
)

// Config represents the complete MCP configuration
// The structure matches the config.yaml file and can be overridden by environment variables

type Config struct {
	MCP MCPConfig `json:"mcp" mapstructure:"mcp"`
}

// MCPConfig contains the main MCP configuration

type MCPConfig struct {
	Server  ServerConfig  `json:"server" mapstructure:"server"`
	Auth    AuthConfig    `json:"auth" mapstructure:"auth"`
	LLM     LLMConfig     `json:"llm" mapstructure:"llm"`
	Workers WorkersConfig `json:"workers" mapstructure:"workers"`
}

// ServerConfig contains server-specific configuration

type ServerConfig struct {
	Addr           string `json:"addr" mapstructure:"addr"`
	MaxConnections int    `json:"max_connections" mapstructure:"max_connections"`
	Timeout        string `json:"timeout" mapstructure:"timeout"`
	RateLimit      int    `json:"rate_limit" mapstructure:"rate_limit"`
}

// AuthConfig contains authentication configuration

type AuthConfig struct {
	Token        string   `json:"token" mapstructure:"token"`
	AllowedTools []string `json:"allowed_tools" mapstructure:"allowed_tools"`
}

// LLMConfig contains LLM provider configuration

type LLMConfig struct {
	Provider string `json:"provider" mapstructure:"provider"`
	Endpoint string `json:"endpoint" mapstructure:"endpoint"`
	Model    string `json:"model" mapstructure:"model"`
	APIKey   string `json:"api_key" mapstructure:"api_key"`
}

// WorkersConfig contains all worker configurations

type WorkersConfig struct {
	BasePath    string            `json:"base_path" mapstructure:"base_path"`
	Shell       ShellConfig       `json:"shell" mapstructure:"shell"`
	TGI         TGIConfig         `json:"tgi" mapstructure:"tgi"`
	LMStudio    LMStudioConfig    `json:"lmstudio" mapstructure:"lmstudio"`
	HuggingFace HuggingFaceConfig `json:"huggingface" mapstructure:"huggingface"`
	Whisper     WhisperConfig     `json:"whisper" mapstructure:"whisper"`
	MinIO       MinIOConfig       `json:"minio" mapstructure:"minio"`
	Vector      VectorConfig      `json:"vector" mapstructure:"vector"`
	Git         GitConfig         `json:"git" mapstructure:"git"`
	Memory      MemoryConfig      `json:"memory" mapstructure:"memory"`
	Project     ProjectConfig     `json:"project" mapstructure:"project"`
}

// ShellConfig contains shell worker configuration

type ShellConfig struct {
	Enabled         bool     `json:"enabled" mapstructure:"enabled"`
	AllowedCommands []string `json:"allowed_commands" mapstructure:"allowed_commands"`
	MaxTimeout      int      `json:"max_timeout" mapstructure:"max_timeout"`
	WorkingDir      string   `json:"working_dir" mapstructure:"working_dir"`
}

// TGIConfig contains TGI (Text Generation Inference) worker configuration

type TGIConfig struct {
	Enabled   bool   `json:"enabled" mapstructure:"enabled"`
	Endpoint  string `json:"endpoint" mapstructure:"endpoint"`
	MaxTokens int    `json:"max_tokens" mapstructure:"max_tokens"`
}

// LMStudioConfig contains LM Studio worker configuration

type LMStudioConfig struct {
	Enabled  bool   `json:"enabled" mapstructure:"enabled"`
	Endpoint string `json:"endpoint" mapstructure:"endpoint"`
}

// HuggingFaceConfig contains HuggingFace worker configuration

type HuggingFaceConfig struct {
	Enabled  bool   `json:"enabled" mapstructure:"enabled"`
	APIToken string `json:"api_token" mapstructure:"api_token"`
}

type WhisperConfig struct {
	Enabled  bool   `json:"enabled" mapstructure:"enabled"`
	Endpoint string `json:"endpoint" mapstructure:"endpoint"`
	APIKey   string `json:"api_key" mapstructure:"api_key"`
}

type MinIOConfig struct {
	Enabled        bool     `json:"enabled" mapstructure:"enabled"`
	Endpoint       string   `json:"endpoint" mapstructure:"endpoint"`
	AccessKey      string   `json:"access_key" mapstructure:"access_key"`
	SecretKey      string   `json:"secret_key" mapstructure:"secret_key"`
	UseSSL         bool     `json:"use_ssl" mapstructure:"use_ssl"`
	AllowedBuckets []string `json:"allowed_buckets" mapstructure:"allowed_buckets"`
	MaxFileSize    string   `json:"max_file_size" mapstructure:"max_file_size"`
	DefaultBucket  string   `json:"default_bucket" mapstructure:"default_bucket"`
}

// VectorConfig contains vector worker configuration

type VectorConfig struct {
	Enabled           bool   `json:"enabled" mapstructure:"enabled"`
	Backend           string `json:"backend" mapstructure:"backend"`
	Endpoint          string `json:"endpoint" mapstructure:"endpoint"`
	DefaultCollection string `json:"default_collection" mapstructure:"default_collection"`
	DefaultDimension  int    `json:"default_dimension" mapstructure:"default_dimension"`
	MaxTopK           int    `json:"max_top_k" mapstructure:"max_top_k"`
	DistanceMetric    string `json:"distance_metric" mapstructure:"distance_metric"`
}

// GitConfig contains git worker configuration

type GitConfig struct {
	Enabled      bool     `json:"enabled" mapstructure:"enabled"`
	AllowedRepos []string `json:"allowed_repos" mapstructure:"allowed_repos"`
}

// MemoryConfig contains memory worker configuration

type MemoryConfig struct {
	StoragePath string `json:"storage_path" mapstructure:"storage_path"`
	MaxSize     string `json:"max_size" mapstructure:"max_size"`
}

// ProjectConfig contains project worker configuration

type ProjectConfig struct {
	Enabled    bool                   `json:"enabled" mapstructure:"enabled"`
	AutoDetect bool                   `json:"auto_detect" mapstructure:"auto_detect"`
	Frameworks map[string]interface{} `json:"frameworks" mapstructure:"frameworks"`
}

// Load loads the configuration from file and environment variables
func Load() (*Config, error) {
	// Load .env first (ignore error if not present)
	_ = godotenv.Load()

	viper.SetConfigName("config")
	viper.SetConfigType("yaml")
	viper.AddConfigPath(".")
	viper.AddConfigPath("$HOME/.mymcp")
	viper.AutomaticEnv()
	viper.SetEnvPrefix("MCP")

	setDefaults()

	if err := viper.ReadInConfig(); err != nil {
		if _, ok := err.(viper.ConfigFileNotFoundError); ok {
			log.Println("No config file found, using defaults")
		} else {
			return nil, err
		}
	}

	var cfg Config
	if err := viper.Unmarshal(&cfg); err != nil {
		return nil, err
	}

	// Resolve paths (expand ~)
	cfg.MCP.Workers.BasePath = resolvePath(cfg.MCP.Workers.BasePath)
	if cfg.MCP.Workers.Memory.StoragePath != "" {
		cfg.MCP.Workers.Memory.StoragePath = resolvePath(cfg.MCP.Workers.Memory.StoragePath)
	}
	return &cfg, nil
}

// setDefaults sets default configuration values
func setDefaults() {
	viper.SetDefault("MCP.SERVER.ADDR", ":8080")
	viper.SetDefault("MCP.SERVER.MAX_CONNECTIONS", 1000)
	viper.SetDefault("MCP.SERVER.TIMEOUT", "30s")
	viper.SetDefault("MCP.SERVER.RATELIMIT", 100)

	viper.SetDefault("MCP.AUTH.TOKEN", "default-secret-token")
	viper.SetDefault("MCP.AUTH.ALLOWED_TOOLS", []string{"*"})

	// LLM defaults
	viper.SetDefault("MCP.LLM.PROVIDER", "ollama")
	viper.SetDefault("MCP.LLM.ENDPOINT", "http://localhost:11434")
	viper.SetDefault("MCP.LLM.MODEL", "qwen3:8b")

	viper.SetDefault("MCP.WORKERS.BASE_PATH", "/Users/adamerickson/Projects")

	// Shell defaults
	viper.SetDefault("MCP.WORKERS.SHELL.ENABLED", true)
	viper.SetDefault("MCP.WORKERS.SHELL.ALLOWED_COMMANDS", []string{"ls", "cat", "git", "go", "npm", "python", "swift", "make", "docker", "kubectl"})
	viper.SetDefault("MCP.WORKERS.SHELL.MAX_TIMEOUT", 60)
	viper.SetDefault("MCP.WORKERS.SHELL.WORKING_DIR", "./")

	// TGI defaults
	viper.SetDefault("MCP.WORKERS.TGI.ENABLED", true)
	viper.SetDefault("MCP.WORKERS.TGI.ENDPOINT", "http://localhost:3000")
	viper.SetDefault("MCP.WORKERS.TGI.MAX_TOKENS", 512)
	viper.SetDefault("MCP.WORKERS.TGI.TEMPERATURE", 0.7)

	// LMStudio defaults
	viper.SetDefault("MCP.WORKERS.LMSTUDIO.ENABLED", true)
	viper.SetDefault("MCP.WORKERS.LMSTUDIO.ENDPOINT", "http://localhost:1234")

	// HuggingFace defaults
	viper.SetDefault("MCP.WORKERS.HUGGINGFACE.ENABLED", true)

	// Whisper defaults
	viper.SetDefault("MCP.WORKERS.WHISPER.ENABLED", true)
	viper.SetDefault("MCP.WORKERS.WHISPER.ENDPOINT", "https://api-inference.huggingface.co/models/openai/whisper-large-v3")

	// MinIO defaults
	viper.SetDefault("MCP.WORKERS.MINIO.ENABLED", true)
	viper.SetDefault("MCP.WORKERS.MINIO.ENDPOINT", "127.0.0.1:9000")
	viper.SetDefault("MCP.WORKERS.MINIO.ACCESS_KEY", "minioadmin")
	viper.SetDefault("MCP.WORKERS.MINIO.SECRET_KEY", "minioadmin")
	viper.SetDefault("MCP.WORKERS.MINIO.USE_SSL", false)
	viper.SetDefault("MCP.WORKERS.MINIO.ALLOWED_BUCKETS", []string{"*"})
	viper.SetDefault("MCP.WORKERS.MINIO.MAX_FILE_SIZE", "100MB")
	viper.SetDefault("MCP.WORKERS.MINIO.DEFAULT_BUCKET", "mcp-default")

	// Vector defaults
	viper.SetDefault("MCP.WORKERS.VECTOR.ENABLED", true)
	viper.SetDefault("MCP.WORKERS.VECTOR.BACKEND", "chroma")
	viper.SetDefault("MCP.WORKERS.VECTOR.ENDPOINT", "http://localhost:8000")
	viper.SetDefault("MCP.WORKERS.VECTOR.DEFAULT_COLLECTION", "default")
	viper.SetDefault("MCP.WORKERS.VECTOR.DEFAULT_DIMENSION", 1024)
	viper.SetDefault("MCP.WORKERS.VECTOR.MAX_TOP_K", 100)
	viper.SetDefault("MCP.WORKERS.VECTOR.DISTANCE_METRIC", "cosine")

	// Git defaults
	viper.SetDefault("MCP.WORKERS.GIT.ENABLED", true)
	viper.SetDefault("MCP.WORKERS.GIT.ALLOWED_REPOS", []string{"*"})

	// Memory defaults
	viper.SetDefault("MCP.WORKERS.MEMORY.STORAGE_PATH", "~/.mymcp/memory.json")
	viper.SetDefault("MCP.WORKERS.MEMORY.MAX_SIZE", "10MB")

	// Project defaults
	viper.SetDefault("MCP.WORKERS.PROJECT.ENABLED", true)
	viper.SetDefault("MCP.WORKERS.PROJECT.AUTO_DETECT", true)
	viper.SetDefault("MCP.WORKERS.PROJECT.FRAMEWORKS", map[string]interface{}{
		"go":     "go.mod",
		"python": []string{"requirements.txt", "pyproject.toml"},
		"node":   "package.json",
		"rust":   "Cargo.toml",
		"swift":  "Package.swift",
	})
}

// resolvePath resolves ~ to home directory and cleans the path
func resolvePath(p string) string {
	if p == "" {
		return p
	}
	if p[0] == '~' {
		home, err := os.UserHomeDir()
		if err == nil {
			p = filepath.Join(home, p[1:])
		}
	}
	return filepath.Clean(p)
}
