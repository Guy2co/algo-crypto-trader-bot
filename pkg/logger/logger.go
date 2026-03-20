package logger

import (
	"fmt"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

// Config holds logging configuration.
type Config struct {
	Level           string `yaml:"level"`
	Format          string `yaml:"format"`
	OutputPath      string `yaml:"output_path"`
	ErrorOutputPath string `yaml:"error_output_path"`
}

// New creates a zap logger from the provided config.
func New(cfg Config) (*zap.Logger, error) {
	level, err := zapcore.ParseLevel(cfg.Level)
	if err != nil {
		return nil, fmt.Errorf("invalid log level %q: %w", cfg.Level, err)
	}

	encoding := cfg.Format
	if encoding == "" {
		encoding = "json"
	}

	outputPath := cfg.OutputPath
	if outputPath == "" {
		outputPath = "stdout"
	}

	errorOutputPath := cfg.ErrorOutputPath
	if errorOutputPath == "" {
		errorOutputPath = "stderr"
	}

	zapCfg := zap.Config{
		Level:             zap.NewAtomicLevelAt(level),
		Development:       false,
		DisableCaller:     false,
		DisableStacktrace: true,
		Sampling:          nil,
		Encoding:          encoding,
		EncoderConfig:     zap.NewProductionEncoderConfig(),
		OutputPaths:       []string{outputPath},
		ErrorOutputPaths:  []string{errorOutputPath},
	}

	if encoding == "console" {
		zapCfg.EncoderConfig = zap.NewDevelopmentEncoderConfig()
	}

	return zapCfg.Build()
}
