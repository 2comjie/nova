package log

import (
	"os"
	"path/filepath"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

type Option func(*zap.Config)

func WithLevel(l zapcore.Level) Option {
	return func(c *zap.Config) { c.Level = zap.NewAtomicLevelAt(l) }
}

func WithConsole() Option {
	return func(c *zap.Config) { c.OutputPaths = append(c.OutputPaths, "stdout") }
}

func WithFile(path string) Option {
	return func(c *zap.Config) {
		dir := filepath.Dir(path)
		_ = os.MkdirAll(dir, 0o755)
		c.OutputPaths = append(c.OutputPaths, path)
	}
}

func WithJSON() Option {
	return func(c *zap.Config) { c.Encoding = "json" }
}

func defaultConfig(dev bool) zap.Config {
	encoderCfg := zap.NewDevelopmentEncoderConfig()
	encoderCfg.EncodeTime = zapcore.ISO8601TimeEncoder

	return zap.Config{
		Level:             zap.NewAtomicLevelAt(zap.DebugLevel),
		Development:       dev,
		DisableCaller:     false,
		DisableStacktrace: true,
		Encoding:          "console",
		EncoderConfig:     encoderCfg,
		OutputPaths:       nil, // 调用方按需添加
		ErrorOutputPaths:  []string{"stderr"},
	}
}

func Init(opts ...Option) {
	cfg := defaultConfig(false)
	for _, opt := range opts {
		opt(&cfg)
	}
	logger, err := cfg.Build()
	if err != nil {
		panic(err)
	}
	zap.ReplaceGlobals(logger)
}

func Dev() {
	cfg := defaultConfig(true)
	cfg.OutputPaths = []string{"stdout"}
	cfg.ErrorOutputPaths = []string{"stderr"}
	logger, err := cfg.Build()
	if err != nil {
		panic(err)
	}
	zap.ReplaceGlobals(logger)
}

func DevFile(logPath string) {
	cfg := defaultConfig(true)
	cfg.OutputPaths = []string{"stdout", logPath}
	cfg.ErrorOutputPaths = []string{"stderr", logPath}
	_ = os.MkdirAll(filepath.Dir(logPath), 0o755)
	logger, err := cfg.Build()
	if err != nil {
		panic(err)
	}
	zap.ReplaceGlobals(logger)
}

func Prod() {
	cfg := zap.NewProductionConfig()
	cfg.Level = zap.NewAtomicLevelAt(zap.InfoLevel)
	cfg.DisableCaller = false
	cfg.DisableStacktrace = true
	cfg.OutputPaths = []string{"stdout"}
	logger, err := cfg.Build()
	if err != nil {
		panic(err)
	}
	zap.ReplaceGlobals(logger)
}

func ProdFile(logPath string) {
	cfg := zap.NewProductionConfig()
	cfg.Level = zap.NewAtomicLevelAt(zap.InfoLevel)
	cfg.DisableCaller = false
	cfg.DisableStacktrace = true
	cfg.OutputPaths = []string{"stdout", logPath}
	_ = os.MkdirAll(filepath.Dir(logPath), 0o755)
	logger, err := cfg.Build()
	if err != nil {
		panic(err)
	}
	zap.ReplaceGlobals(logger)
}
