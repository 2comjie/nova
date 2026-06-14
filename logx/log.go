package logx

import (
	"github.com/2comjie/wali/logx/logdef"
	"github.com/2comjie/wali/logx/stdlog"
)

var _globalLog logdef.ILogger = nil
var _log logdef.ILogger = nil

func init() {
	SetLogger(stdlog.NewDefaultLog())
}

func SetLogger(logger logdef.ILogger) {
	_log = logger
	_globalLog = _log.WithSkip(1)
}

func Debug(args ...interface{}) {
	_globalLog.Debug(args...)
}

func Debugf(format string, args ...interface{}) {
	_globalLog.Debugf(format, args...)
}

func Info(args ...interface{}) {
	_globalLog.Info(args...)
}

func Infof(format string, args ...interface{}) {
	_globalLog.Infof(format, args...)
}

func Warn(args ...interface{}) {
	_globalLog.Warn(args...)
}

func Warnf(format string, args ...interface{}) {
	_globalLog.Warnf(format, args...)
}

func Error(args ...interface{}) {
	_globalLog.Error(args...)
}

func Errorf(format string, args ...interface{}) {
	_globalLog.Errorf(format, args...)
}

func Fatal(args ...interface{}) {
	_globalLog.Fatal(args...)
}

func Fatalf(format string, args ...interface{}) {
	_globalLog.Fatalf(format, args...)
}

func Panic(args ...interface{}) {
	_globalLog.Panic(args...)
}

func Panicf(format string, args ...interface{}) {
	_globalLog.Panicf(format, args...)
}

func WithField(key string, value interface{}) logdef.ILogger {
	return _log.WithField(key, value)
}

func WithFields(fields logdef.Fields) logdef.ILogger {
	return _log.WithFields(fields)
}

func WithSkip(skip int) logdef.ILogger {
	return _log.WithSkip(skip)
}

func WithName(name string) logdef.ILogger {
	return _log.WithName(name)
}

func WithLevel(level logdef.Level) logdef.ILogger {
	return _log.WithLevel(level)
}

func WithError(err error) logdef.ILogger {
	return _log.WithError(err)
}

func WithFieldSlice(fields ...logdef.Field) logdef.ILogger {
	return _log.WithFieldSlice(fields...)
}
