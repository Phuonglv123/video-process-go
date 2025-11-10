package main

import (
	"os"
	"testing"
)

func TestLoadConfig(t *testing.T) {
	// Set environment variables for testing
	os.Setenv("MINIO_ENDPOINT", "test.minio.io")
	os.Setenv("MINIO_ACCESS_KEY", "test_access")
	os.Setenv("MINIO_SECRET_KEY", "test_secret")
	os.Setenv("MINIO_BUCKET", "test-bucket")
	os.Setenv("WORKERS", "4")

	config := loadConfig()

	if config.MinIOEndpoint != "test.minio.io" {
		t.Errorf("Expected endpoint test.minio.io, got %s", config.MinIOEndpoint)
	}

	if config.MinIOAccessKey != "test_access" {
		t.Errorf("Expected access key test_access, got %s", config.MinIOAccessKey)
	}

	if config.Workers != 4 {
		t.Errorf("Expected 4 workers, got %d", config.Workers)
	}

	// Clean up
	os.Unsetenv("MINIO_ENDPOINT")
	os.Unsetenv("MINIO_ACCESS_KEY")
	os.Unsetenv("MINIO_SECRET_KEY")
	os.Unsetenv("MINIO_BUCKET")
	os.Unsetenv("WORKERS")
}

func TestGetEnv(t *testing.T) {
	// Test with existing environment variable
	os.Setenv("TEST_VAR", "test_value")
	value := getEnv("TEST_VAR", "default")
	if value != "test_value" {
		t.Errorf("Expected test_value, got %s", value)
	}
	os.Unsetenv("TEST_VAR")

	// Test with non-existing environment variable
	value = getEnv("NON_EXISTENT_VAR", "default_value")
	if value != "default_value" {
		t.Errorf("Expected default_value, got %s", value)
	}
}

func TestCreateDirectories(t *testing.T) {
	// Create a temporary config
	config := &Config{
		BackupDir:    "/tmp/test_backup",
		ProcessedDir: "/tmp/test_processed",
		LogDir:       "/tmp/test_logs",
	}

	// Create directories
	err := createDirectories(config)
	if err != nil {
		t.Errorf("Failed to create directories: %v", err)
	}

	// Verify directories exist
	if _, err := os.Stat(config.BackupDir); os.IsNotExist(err) {
		t.Errorf("Backup directory was not created")
	}

	if _, err := os.Stat(config.ProcessedDir); os.IsNotExist(err) {
		t.Errorf("Processed directory was not created")
	}

	if _, err := os.Stat(config.LogDir); os.IsNotExist(err) {
		t.Errorf("Log directory was not created")
	}

	// Clean up
	os.RemoveAll(config.BackupDir)
	os.RemoveAll(config.ProcessedDir)
	os.RemoveAll(config.LogDir)
}
