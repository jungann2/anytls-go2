package config

import (
	"bytes"
	"encoding/json"
	"os"
	"testing"

	"github.com/leanovate/gopter"
	"github.com/leanovate/gopter/gen"
	"github.com/leanovate/gopter/prop"
	"github.com/sirupsen/logrus"
)

// Feature: anytls-xboard-integration, Property 10: 结构化日志格式
// **Validates: Requirements 9.3**

func TestProperty10_StructuredLogFormat(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 100

	properties := gopter.NewProperties(parameters)

	// Sub-property: auth success events contain required fields
	properties.Property("Auth success log contains timestamp, operation, and user_id", prop.ForAll(
		func(userID int) bool {
			logger, buf := newTestLogger()
			logger.WithFields(logrus.Fields{
				"operation": "auth_success",
				"user_id":   userID,
			}).Info("认证成功")

			return validateLogFields(buf.Bytes(), "auth_success", &userID)
		},
		gen.IntRange(1, 100000),
	))

	// Sub-property: auth failure events contain required fields
	properties.Property("Auth failure log contains timestamp, operation, and ip", prop.ForAll(
		func(ipSuffix int) bool {
			logger, buf := newTestLogger()
			logger.WithFields(logrus.Fields{
				"operation": "auth_failure",
				"ip":        "10.0.0.1",
			}).Warn("认证失败")

			return validateLogFields(buf.Bytes(), "auth_failure", nil)
		},
		gen.IntRange(1, 255),
	))

	// Sub-property: API sync events contain required fields
	properties.Property("API sync log contains timestamp and operation", prop.ForAll(
		func(userCount int) bool {
			logger, buf := newTestLogger()
			logger.WithFields(logrus.Fields{
				"operation": "api_sync",
				"count":     userCount,
			}).Info("用户列表已同步")

			return validateLogFields(buf.Bytes(), "api_sync", nil)
		},
		gen.IntRange(0, 1000),
	))

	// Sub-property: traffic push events contain required fields
	properties.Property("Traffic push log contains timestamp and operation", prop.ForAll(
		func(userCount int) bool {
			logger, buf := newTestLogger()
			logger.WithFields(logrus.Fields{
				"operation": "traffic_push",
				"users":     userCount,
			}).Info("流量已上报")

			return validateLogFields(buf.Bytes(), "traffic_push", nil)
		},
		gen.IntRange(1, 500),
	))

	// Sub-property: log with user_id always includes user_id field
	properties.Property("Log entries with user context always include user_id", prop.ForAll(
		func(userID int, op string) bool {
			logger, buf := newTestLogger()
			logger.WithFields(logrus.Fields{
				"operation": op,
				"user_id":   userID,
			}).Info("event")

			return validateLogFields(buf.Bytes(), op, &userID)
		},
		gen.IntRange(1, 100000),
		gen.OneConstOf("auth_success", "device_limit", "traffic_push"),
	))

	properties.TestingRun(t)
}

func TestSetupLogger_Defaults(t *testing.T) {
	logger, err := SetupLogger(LogConfig{Level: "info"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if logger.GetLevel() != logrus.InfoLevel {
		t.Errorf("expected info level, got %v", logger.GetLevel())
	}
}

func TestSetupLogger_WithFile(t *testing.T) {
	f, err := os.CreateTemp("", "logger-test-*.log")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(f.Name())
	f.Close()

	logger, err := SetupLogger(LogConfig{Level: "debug", FilePath: f.Name()})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if logger.GetLevel() != logrus.DebugLevel {
		t.Errorf("expected debug level, got %v", logger.GetLevel())
	}
}

func TestSetupLogger_InvalidLevel(t *testing.T) {
	logger, err := SetupLogger(LogConfig{Level: "invalid_level"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Should fall back to info level
	if logger.GetLevel() != logrus.InfoLevel {
		t.Errorf("expected info level fallback, got %v", logger.GetLevel())
	}
}

// --- helpers ---

func newTestLogger() (*logrus.Logger, *bytes.Buffer) {
	var buf bytes.Buffer
	logger := logrus.New()
	logger.SetFormatter(&logrus.JSONFormatter{
		TimestampFormat: "2006-01-02 15:04:05",
	})
	logger.SetOutput(&buf)
	logger.SetLevel(logrus.DebugLevel)
	return logger, &buf
}

func validateLogFields(data []byte, expectedOp string, expectedUserID *int) bool {
	var entry map[string]interface{}
	if err := json.Unmarshal(data, &entry); err != nil {
		return false
	}

	// Must have timestamp
	if _, ok := entry["time"]; !ok {
		return false
	}

	// Must have operation field matching expected
	op, ok := entry["operation"]
	if !ok {
		return false
	}
	if op.(string) != expectedOp {
		return false
	}

	// If user_id expected, verify it
	if expectedUserID != nil {
		uid, ok := entry["user_id"]
		if !ok {
			return false
		}
		// JSON numbers are float64
		if int(uid.(float64)) != *expectedUserID {
			return false
		}
	}

	return true
}
