package stdlog

import (
	"fmt"
	"log"
	"os"
	"strings"
	"sync"

	"github.com/2comjie/wali/logx/logdef"
)

var builderPool sync.Pool

func init() {
	builderPool.New = func() any {
		return &strings.Builder{}
	}
}

type Hook func(level logdef.Level, fields []logdef.Field, msg string)
type Logger struct {
	skip   int
	fields []logdef.Field
	level  logdef.Level
	logger *log.Logger
	name   string
	Hook   Hook
}

func NewDefaultLog() logdef.ILogger {
	lg := log.New(os.Stdout, "", log.Lshortfile|log.LstdFlags|log.Lmicroseconds)
	l := &Logger{
		logger: lg,
		skip:   3,
	}
	l.level = logdef.LevelInfo
	l.updatePrefix()
	return l
}

func NewLog(lg *log.Logger) logdef.ILogger {
	return &Logger{
		logger: lg,
	}
}

func (l Logger) Debug(args ...interface{}) {
	if logdef.LevelDebug.IntValue() < l.level.IntValue() {
		return
	}
	l.print(logdef.LevelDebug, fmt.Sprint(args...))
}

func (l Logger) Debugf(format string, args ...interface{}) {
	if logdef.LevelDebug.IntValue() < l.level.IntValue() {
		return
	}
	l.print(logdef.LevelDebug, fmt.Sprintf(format, args...))
}

func (l Logger) Info(args ...interface{}) {
	if logdef.LevelInfo.IntValue() < l.level.IntValue() {
		return
	}
	l.print(logdef.LevelInfo, fmt.Sprint(args...))
}

func (l Logger) Infof(format string, args ...interface{}) {
	if logdef.LevelInfo.IntValue() < l.level.IntValue() {
		return
	}
	l.print(logdef.LevelInfo, fmt.Sprintf(format, args...))
}

func (l Logger) Warn(args ...interface{}) {
	if logdef.LevelWarn.IntValue() < l.level.IntValue() {
		return
	}
	l.print(logdef.LevelWarn, fmt.Sprint(args...))
}

func (l Logger) Warnf(format string, args ...interface{}) {
	if logdef.LevelWarn.IntValue() < l.level.IntValue() {
		return
	}
	l.print(logdef.LevelWarn, fmt.Sprintf(format, args...))
}

func (l Logger) Error(args ...interface{}) {
	if logdef.LevelError.IntValue() < l.level.IntValue() {
		return
	}
	l.print(logdef.LevelError, fmt.Sprint(args...))
}

func (l Logger) Errorf(format string, args ...interface{}) {
	if logdef.LevelError.IntValue() < l.level.IntValue() {
		return
	}
	l.print(logdef.LevelError, fmt.Sprintf(format, args...))
}

func (l Logger) Fatal(v ...interface{}) {
	l.print(logdef.LevelFatal, fmt.Sprint(v...))
	os.Exit(1)
}

func (l Logger) Fatalf(format string, v ...interface{}) {
	l.print(logdef.LevelFatal, fmt.Sprintf(format, v...))
	os.Exit(1)
}

func (l Logger) Panic(v ...interface{}) {
	l.print(logdef.LevelPanic, fmt.Sprint(v...))
}

func (l Logger) Panicf(format string, v ...interface{}) {
	l.print(logdef.LevelPanic, fmt.Sprintf(format, v...))
}

func (l Logger) WithField(key string, value interface{}) logdef.ILogger {
	c := l.cloneFields(1)
	for i := range c.fields {
		if c.fields[i].K == key {
			c.fields[i].V = value
			return c
		}
	}
	c.fields = append(c.fields, logdef.Field{K: key, V: value})
	return c
}

func (l Logger) WithFields(fields logdef.Fields) logdef.ILogger {
	c := l.cloneFields(len(fields))
	for key, value := range fields {
		found := false
		for i := range c.fields {
			if c.fields[i].K == key {
				c.fields[i].V = value
				found = true
				break
			}
		}
		if !found {
			c.fields = append(c.fields, logdef.Field{K: key, V: value})
		}
	}
	return c
}

func (l Logger) WithSkip(skip int) logdef.ILogger {
	c := l.fastClone()
	c.skip += skip
	return c
}

func (l Logger) WithLevel(level logdef.Level) logdef.ILogger {
	c := l.deepClone()
	c.level = level
	c.updatePrefix()
	return c
}

func (l Logger) WithName(name string) logdef.ILogger {
	c := l.deepClone()
	c.name = name
	c.updatePrefix()
	return c
}

func (l Logger) WithError(err error) logdef.ILogger {
	if err == nil {
		return l
	}
	return l.WithField("error", err.Error())
}

func (l Logger) WithFieldSlice(fields ...logdef.Field) logdef.ILogger {
	c := l.cloneFields(len(fields))
	for _, field := range fields {
		found := false
		for i := range c.fields {
			if c.fields[i].K == field.K {
				c.fields[i].V = field.V
				found = true
				break
			}
		}
		if !found {
			c.fields = append(c.fields, field)
		}
	}
	return c
}

func (l Logger) updatePrefix() {
	l.logger.SetPrefix(fmt.Sprintf("%s %s ", l.level, l.name))
}

func (l Logger) fastClone() Logger {
	c := Logger{
		skip:   l.skip,
		level:  l.level,
		logger: l.logger,
		name:   l.name,
		Hook:   l.Hook,
	}
	c.fields = make([]logdef.Field, len(l.fields))
	copy(c.fields, l.fields)
	return c
}

func (l Logger) cloneFields(extraCap int) Logger {
	c := Logger{
		skip:   l.skip,
		level:  l.level,
		logger: l.logger,
		name:   l.name,
		Hook:   l.Hook,
	}
	c.fields = make([]logdef.Field, len(l.fields), len(l.fields)+extraCap)
	copy(c.fields, l.fields)
	return c
}

func (l Logger) deepClone() Logger {
	c := Logger{
		skip:   l.skip,
		level:  l.level,
		logger: log.New(l.logger.Writer(), l.logger.Prefix(), l.logger.Flags()),
		name:   l.name,
		Hook:   l.Hook,
	}
	for _, fd := range l.fields {
		c.fields = append(c.fields, logdef.Field{K: fd.K, V: fd.V})
	}
	return c
}

func (l Logger) print(level logdef.Level, msg string) {
	builder := builderPool.Get().(*strings.Builder)
	defer func() {
		builder.Reset()
		builderPool.Put(builder)
	}()

	// 预估容量：每个字段约 20 字符 + 消息长度
	builder.Grow(len(l.fields)*20 + len(msg) + 10)

	for _, f := range l.fields {
		builder.WriteString(" ")
		builder.WriteString(f.K)
		builder.WriteString(" ")
		fmt.Fprintf(builder, "%+v", f.V)
	}
	builder.WriteString(" ")
	builder.WriteString(msg)

	result := builder.String()
	if l.Hook != nil {
		l.Hook(level, l.fields, result)
	}
	l.logger.Output(l.skip, result)
}
