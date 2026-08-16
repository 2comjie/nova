package wlog

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/2comjie/nova/logx/logdef"
)

var builderPool sync.Pool

func init() {
	builderPool.New = func() any {
		return &bytes.Buffer{}
	}
}

type Hook func(level logdef.Level, fields []logdef.Field, msg string)
type Logger struct {
	skip   int
	fields []logdef.Field
	level  logdef.Level
	name   string
	Hook   Hook
	writer io.Writer
	json   bool
}

func NewDefaultLog() logdef.ILogger {
	l := Logger{
		skip:   2,
		writer: os.Stdout,
	}
	l.level = logdef.LevelInfo
	return l
}

func NewLog(writer io.Writer) logdef.ILogger {
	return Logger{
		skip:   2,
		writer: writer,
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
	return c
}

func (l Logger) WithName(name string) logdef.ILogger {
	c := l.deepClone()
	c.name = name
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

func (l Logger) WithJsonFormat() Logger {
	c := l.fastClone()
	c.json = true
	return c
}

func (l Logger) fastClone() Logger {
	c := Logger{
		skip:   l.skip,
		level:  l.level,
		name:   l.name,
		Hook:   l.Hook,
		writer: l.writer,
		json:   l.json,
	}
	c.fields = make([]logdef.Field, len(l.fields))
	copy(c.fields, l.fields)
	return c
}

// cloneFields clones the Logger and reserves extraCap additional capacity
// in the fields slice to avoid reallocation on subsequent appends.
func (l Logger) cloneFields(extraCap int) Logger {
	c := Logger{
		skip:   l.skip,
		level:  l.level,
		name:   l.name,
		Hook:   l.Hook,
		writer: l.writer,
		json:   l.json,
	}
	c.fields = make([]logdef.Field, len(l.fields), len(l.fields)+extraCap)
	copy(c.fields, l.fields)
	return c
}

func (l Logger) deepClone() Logger {
	c := Logger{
		skip:   l.skip,
		level:  l.level,
		name:   l.name,
		Hook:   l.Hook,
		writer: l.writer,
		json:   l.json,
	}
	for _, fd := range l.fields {
		c.fields = append(c.fields, logdef.Field{K: fd.K, V: fd.V})
	}
	return c
}

// skip default is 4
func getCallInfoN(skip int) (string, string) {
	pc, file, line, _ := runtime.Caller(skip)
	funcName := runtime.FuncForPC(pc).Name()
	ss := strings.Split(funcName, "/")
	sLen := len(ss)
	if sLen > 0 {
		funcName = ss[sLen-1]
	}
	return file + ":" + strconv.Itoa(line), funcName
}
func (l Logger) print(level logdef.Level, msg string) {
	caller, fname := getCallInfoN(l.skip + 1)
	timeFormat := time.Now().Format("2006.01.02-15.04.05")
	if !l.json {
		builder := builderPool.Get().(*bytes.Buffer)
		defer func() {
			builder.Reset()
			builderPool.Put(builder)
		}()
		// 预估容量：每个字段约 20 字符 + 消息长度
		builder.Grow(len(l.fields)*20 + len(msg) + 10)
		builder.WriteString(string(level))
		builder.WriteString(" ")
		builder.WriteString(timeFormat)
		builder.WriteString(" ")
		builder.WriteString(caller)
		builder.WriteString(" ")
		builder.WriteString(fname)
		builder.WriteString(" ")
		for _, f := range l.fields {
			builder.WriteString(f.K)
			builder.WriteString(" ")
			fmt.Fprintf(builder, "%+v", f.V)
			builder.WriteString(" ")
		}
		builder.WriteString(msg)
		builder.WriteString("\n")
		if l.Hook != nil {
			result := builder.String()
			l.Hook(level, l.fields, result)
		}
		l.writer.Write(builder.Bytes())
	} else {
		jsonValue := mapPool.Get().(map[string]any)
		defer func() {
			clear(jsonValue)
			mapPool.Put(jsonValue)
		}()
		for _, f := range l.fields {
			jsonValue[f.K] = f.V
		}
		jsonValue["time"] = timeFormat
		jsonValue["level"] = level
		if l.name != "" {
			jsonValue["name"] = l.name
		}
		jsonValue["caller"] = caller
		jsonValue["func"] = fname
		jsonValue["msg"] = msg
		bs, err := json.Marshal(jsonValue)
		if err != nil {
			fmt.Printf("json 日志 序列化失败 jsonValue %+v error %+v\n", jsonValue, err)
		}
		l.writer.Write(append(bs, lineSep...))
		if l.Hook != nil {
			l.Hook(level, l.fields, string(bs))
		}
	}
}

var lineSep = []byte("\n")
var mapPool = sync.Pool{
	New: func() any {
		return make(map[string]any, 8)
	},
}
