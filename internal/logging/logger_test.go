package logging

import (
	"bytes"
	"context"
	"log"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

// captureLog captures log output for testing
func captureLog(f func()) string {
	var buf bytes.Buffer
	log.SetOutput(&buf)
	defer log.SetOutput(log.Writer())

	f()

	return buf.String()
}

func TestNew(t *testing.T) {
	logger := New("test-prefix")

	assert.NotNil(t, logger)
	assert.Equal(t, "test-prefix", logger.prefix)
}

func TestLogger_Info(t *testing.T) {
	logger := New("test")
	ctx := context.Background()

	output := captureLog(func() {
		logger.Info(ctx, "test message")
	})

	assert.Contains(t, output, "[INFO]")
	assert.Contains(t, output, "test:")
	assert.Contains(t, output, "test message")
}

func TestLogger_Error(t *testing.T) {
	logger := New("test")
	ctx := context.Background()

	output := captureLog(func() {
		logger.Error(ctx, "error occurred")
	})

	assert.Contains(t, output, "[ERROR]")
	assert.Contains(t, output, "test:")
	assert.Contains(t, output, "error occurred")
}

func TestLogger_Warn(t *testing.T) {
	logger := New("test")
	ctx := context.Background()

	output := captureLog(func() {
		logger.Warn(ctx, "warning message")
	})

	assert.Contains(t, output, "[WARN]")
	assert.Contains(t, output, "test:")
	assert.Contains(t, output, "warning message")
}

func TestLogger_Debug(t *testing.T) {
	logger := New("test")
	ctx := context.Background()

	output := captureLog(func() {
		logger.Debug(ctx, "debug info")
	})

	assert.Contains(t, output, "[DEBUG]")
	assert.Contains(t, output, "test:")
	assert.Contains(t, output, "debug info")
}

func TestLogger_WithFormatting(t *testing.T) {
	logger := New("test")
	ctx := context.Background()

	output := captureLog(func() {
		logger.Info(ctx, "user %s logged in with id %d", "john", 123)
	})

	assert.Contains(t, output, "user john logged in with id 123")
}

func TestLogger_WithRequestID(t *testing.T) {
	logger := New("test")
	ctx := context.WithValue(context.Background(), "request_id", "req-12345")

	output := captureLog(func() {
		logger.Info(ctx, "test message")
	})

	assert.Contains(t, output, "[req_id=req-12345]")
	assert.Contains(t, output, "test message")
}

func TestLogger_WithUserID(t *testing.T) {
	logger := New("test")
	ctx := context.WithValue(context.Background(), "user_id", int64(42))

	output := captureLog(func() {
		logger.Info(ctx, "test message")
	})

	assert.Contains(t, output, "user_id=42")
	assert.Contains(t, output, "test message")
}

func TestLogger_WithRequestIDAndUserID(t *testing.T) {
	logger := New("test")
	ctx := context.WithValue(context.Background(), "request_id", "req-999")
	ctx = context.WithValue(ctx, "user_id", int64(100))

	output := captureLog(func() {
		logger.Info(ctx, "authenticated request")
	})

	assert.Contains(t, output, "[req_id=req-999 user_id=100]")
	assert.Contains(t, output, "authenticated request")
}

func TestLogger_WithoutContext(t *testing.T) {
	logger := New("test")

	output := captureLog(func() {
		logger.Info(nil, "no context message")
	})

	// Should not have context info
	assert.NotContains(t, output, "req_id")
	assert.NotContains(t, output, "user_id")
	assert.Contains(t, output, "no context message")
}

func TestLogger_EmptyContext(t *testing.T) {
	logger := New("test")
	ctx := context.Background()

	output := captureLog(func() {
		logger.Info(ctx, "empty context message")
	})

	// Should not have context info
	assert.NotContains(t, output, "req_id")
	assert.NotContains(t, output, "user_id")
	assert.Contains(t, output, "empty context message")
}

func TestLogger_TimestampFormat(t *testing.T) {
	logger := New("test")
	ctx := context.Background()

	output := captureLog(func() {
		logger.Info(ctx, "test")
	})

	// Timestamp format: 2006-01-02 15:04:05
	// Should contain date and time
	assert.Regexp(t, `\d{4}-\d{2}-\d{2} \d{2}:\d{2}:\d{2}`, output)
}

func TestLogger_LogFormat(t *testing.T) {
	logger := New("myapp")
	ctx := context.Background()

	output := captureLog(func() {
		logger.Info(ctx, "processing request")
	})

	// Format: timestamp [LEVEL] prefix: message
	parts := strings.Fields(output)

	// Should have date, time, level, prefix, and message
	assert.GreaterOrEqual(t, len(parts), 5)
	assert.Contains(t, output, "[INFO]")
	assert.Contains(t, output, "myapp:")
	assert.Contains(t, output, "processing request")
}

func TestExtractRequestID_ValidContext(t *testing.T) {
	ctx := context.WithValue(context.Background(), "request_id", "test-req-id")

	requestID := extractRequestID(ctx)

	assert.Equal(t, "test-req-id", requestID)
}

func TestExtractRequestID_MissingKey(t *testing.T) {
	ctx := context.Background()

	requestID := extractRequestID(ctx)

	assert.Equal(t, "", requestID)
}

func TestExtractRequestID_WrongType(t *testing.T) {
	ctx := context.WithValue(context.Background(), "request_id", 12345)

	requestID := extractRequestID(ctx)

	assert.Equal(t, "", requestID)
}

func TestExtractRequestID_NilContext(t *testing.T) {
	requestID := extractRequestID(nil)

	assert.Equal(t, "", requestID)
}

func TestExtractUserID_ValidContext(t *testing.T) {
	ctx := context.WithValue(context.Background(), "user_id", int64(789))

	userID := extractUserID(ctx)

	assert.Equal(t, int64(789), userID)
}

func TestExtractUserID_MissingKey(t *testing.T) {
	ctx := context.Background()

	userID := extractUserID(ctx)

	assert.Equal(t, int64(0), userID)
}

func TestExtractUserID_WrongType(t *testing.T) {
	ctx := context.WithValue(context.Background(), "user_id", "not-an-int")

	userID := extractUserID(ctx)

	assert.Equal(t, int64(0), userID)
}

func TestExtractUserID_NilContext(t *testing.T) {
	userID := extractUserID(nil)

	assert.Equal(t, int64(0), userID)
}

// Default logger tests
func TestDefaultLogger_Info(t *testing.T) {
	ctx := context.Background()

	output := captureLog(func() {
		Info(ctx, "default logger test")
	})

	assert.Contains(t, output, "[INFO]")
	assert.Contains(t, output, "app:")
	assert.Contains(t, output, "default logger test")
}

func TestDefaultLogger_Error(t *testing.T) {
	ctx := context.Background()

	output := captureLog(func() {
		Error(ctx, "default error")
	})

	assert.Contains(t, output, "[ERROR]")
	assert.Contains(t, output, "default error")
}

func TestDefaultLogger_Warn(t *testing.T) {
	ctx := context.Background()

	output := captureLog(func() {
		Warn(ctx, "default warning")
	})

	assert.Contains(t, output, "[WARN]")
	assert.Contains(t, output, "default warning")
}

func TestDefaultLogger_Debug(t *testing.T) {
	ctx := context.Background()

	output := captureLog(func() {
		Debug(ctx, "default debug")
	})

	assert.Contains(t, output, "[DEBUG]")
	assert.Contains(t, output, "default debug")
}

func TestLogger_MultipleLevels(t *testing.T) {
	logger := New("test")
	ctx := context.Background()

	output := captureLog(func() {
		logger.Info(ctx, "info message")
		logger.Warn(ctx, "warn message")
		logger.Error(ctx, "error message")
		logger.Debug(ctx, "debug message")
	})

	assert.Contains(t, output, "[INFO]")
	assert.Contains(t, output, "[WARN]")
	assert.Contains(t, output, "[ERROR]")
	assert.Contains(t, output, "[DEBUG]")
	assert.Contains(t, output, "info message")
	assert.Contains(t, output, "warn message")
	assert.Contains(t, output, "error message")
	assert.Contains(t, output, "debug message")
}

func TestLogger_DifferentPrefixes(t *testing.T) {
	logger1 := New("api")
	logger2 := New("worker")
	ctx := context.Background()

	output := captureLog(func() {
		logger1.Info(ctx, "api message")
		logger2.Info(ctx, "worker message")
	})

	assert.Contains(t, output, "api:")
	assert.Contains(t, output, "worker:")
}

func TestLogger_LongMessages(t *testing.T) {
	logger := New("test")
	ctx := context.Background()

	longMessage := strings.Repeat("x", 1000)

	output := captureLog(func() {
		logger.Info(ctx, "%s", longMessage)
	})

	assert.Contains(t, output, longMessage)
}

func TestLogger_SpecialCharacters(t *testing.T) {
	logger := New("test")
	ctx := context.Background()

	output := captureLog(func() {
		logger.Info(ctx, "message with \"quotes\" and 'apostrophes' and \n newlines")
	})

	assert.Contains(t, output, "quotes")
	assert.Contains(t, output, "apostrophes")
}

func TestLogger_ConcurrentLogging(t *testing.T) {
	logger := New("test")
	ctx := context.Background()

	// Concurrent logging should not cause issues
	done := make(chan bool)

	for i := 0; i < 10; i++ {
		go func(id int) {
			logger.Info(ctx, "concurrent message %d", id)
			done <- true
		}(i)
	}

	// Wait for all goroutines
	for i := 0; i < 10; i++ {
		<-done
	}

	// Should not panic or cause race conditions
}
