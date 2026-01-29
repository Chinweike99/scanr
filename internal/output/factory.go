package output

import (
	"encoding/json"
	"fmt"
	"os"
)

// FormatterFactory creates formatters based on configuration
type FormatterFactory struct{}

// NewFormatterFactory creates a new formatter factory
func NewFormatterFactory() *FormatterFactory {
    return &FormatterFactory{}
}

// CreateFormatter creates a formatter based on the configuration
func (f *FormatterFactory) CreateFormatter(config Config) (Formatter, error) {
    switch config.Format {
    case "text":
        return NewTextFormatter(config), nil
    case "json":
        return NewJSONFormatter(config), nil
    case "jsonl":
        // JSONL is just JSON with streaming
        config.Format = "json"
        return NewJSONFormatter(config), nil
    default:
        return nil, fmt.Errorf("unsupported format: %s", config.Format)
    }
}

// CreateFormatterFromFlags creates a formatter from CLI flags
func (f *FormatterFactory) CreateFormatterFromFlags(format string, color bool) (Formatter, error) {
    config := DefaultConfig()
    config.Format = format
    config.Color = color && format == "text" && isTerminal()
    
    return f.CreateFormatter(config)
}

// isTerminal checks if stdout is a terminal
func isTerminal() bool {
    fileInfo, _ := os.Stdout.Stat()
    return (fileInfo.Mode() & os.ModeCharDevice) != 0
}

// FormatError formats an error for output
func FormatError(err error, format string) string {
    switch format {
    case "json":
        errorJSON := map[string]string{
            "error": err.Error(),
        }
        jsonBytes, _ := json.Marshal(errorJSON)
        return string(jsonBytes)
    default:
        return fmt.Sprintf("Error: %v", err)
    }
}