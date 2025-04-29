package paymaster

import (
	"errors"
	"os"
	"time"
)

// Config represents the configuration for the Paymaster service
type Config struct {
	// Paymaster settings
	PrivateKey string // Private key (for signing transactions)

	// Processing settings
	ProcessorInterval time.Duration // Interval for processing pending transactions
}

// DefaultConfig returns the default configuration
func DefaultConfig() Config {
	return Config{
		// Paymaster settings
		PrivateKey: "",

		// Processing settings
		ProcessorInterval: 10 * time.Second,
	}
}

// Copy creates a deep copy of the Config
func (c *Config) Copy() *Config {
	return &Config{
		PrivateKey:        c.PrivateKey,
		ProcessorInterval: c.ProcessorInterval,
	}
}

// Validate validates the configuration
func (c *Config) Validate() error {
	if c.PrivateKey == "" {
		return errors.New("private key must be set")
	}
	if c.ProcessorInterval <= 0 {
		return errors.New("processor interval must be greater than zero")
	}
	return nil
}

// LoadFromEnv loads configuration from environment variables
func LoadFromEnv() Config {
	config := DefaultConfig()

	if key := os.Getenv("PAYMASTER_PRIVATE_KEY"); key != "" {
		config.PrivateKey = key
	}
	if interval := os.Getenv("PAYMASTER_PROCESSOR_INTERVAL"); interval != "" {
		if d, err := time.ParseDuration(interval); err == nil {
			config.ProcessorInterval = d
		}
	}
	return config
}
