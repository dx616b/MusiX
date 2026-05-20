// Package log provides level-aware, color-coded logging (StreamX-style).
package log

import (
	"fmt"
	stdlog "log"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	fiblog "github.com/gofiber/fiber/v2/log"
)

// Level is the minimum severity emitted by this package.
type Level = fiblog.Level

const (
	LevelDebug = fiblog.LevelDebug
	LevelInfo  = fiblog.LevelInfo
	LevelWarn  = fiblog.LevelWarn
	LevelError = fiblog.LevelError
	LevelFatal = fiblog.LevelFatal
)

var minLevel Level = LevelInfo

// SetLevel sets the minimum log level for MusiX and Fiber's logger.
func SetLevel(l Level) {
	fiblog.SetLevel(l)
	minLevel = l
}

// SetLevelFromString sets the log level from "debug", "info", "warn", "error", or "fatal".
func SetLevelFromString(level string) {
	switch strings.ToLower(strings.TrimSpace(level)) {
	case "debug", "0":
		SetLevel(LevelDebug)
	case "info", "1", "":
		SetLevel(LevelInfo)
	case "warn", "warning", "2":
		SetLevel(LevelWarn)
	case "error", "3":
		SetLevel(LevelError)
	case "fatal", "4":
		SetLevel(LevelFatal)
	default:
		SetLevel(LevelInfo)
	}
}

// IsDebug reports whether debug-level messages are emitted.
func IsDebug() bool {
	return minLevel <= LevelDebug
}

const (
	colorReset  = "\x1b[0m"
	colorRed    = "\x1b[31m"
	colorGreen  = "\x1b[32m"
	colorYellow = "\x1b[33m"
	colorCyan   = "\x1b[36m"
)

func init() {
	stdlog.SetFlags(stdlog.Ldate | stdlog.Ltime)
}

func colorForLevel(level string) string {
	if os.Getenv("NO_COLOR") != "" {
		return ""
	}
	switch strings.ToLower(level) {
	case "debug":
		return colorCyan
	case "warn", "warning":
		return colorYellow
	case "error", "fatal":
		return colorRed
	default:
		return colorGreen
	}
}

func levelTag(level string) string {
	c := colorForLevel(level)
	if c == "" {
		return "[" + strings.ToUpper(level) + "]"
	}
	return c + "[" + strings.ToUpper(level) + "]" + colorReset
}

func sourceTag() string {
	_, file, _, ok := runtime.Caller(2)
	if !ok || file == "" {
		return "unknown"
	}
	base := filepath.Base(file)
	return strings.TrimSuffix(base, filepath.Ext(base))
}

func fmtMsg(format string, args ...any) string {
	if format == "" {
		return strings.TrimSpace(fmt.Sprint(args...))
	}
	return strings.TrimSpace(fmt.Sprintf(format, args...))
}

func emit(level string, msg string) {
	if msg == "" {
		return
	}
	stdlog.Printf("%s [%s] %s", levelTag(level), sourceTag(), msg)
}

func Infof(format string, args ...any) {
	emit("info", fmtMsg(format, args...))
}

func Warnf(format string, args ...any) {
	if minLevel > LevelWarn {
		return
	}
	emit("warn", fmtMsg(format, args...))
}

func Debugf(format string, args ...any) {
	if minLevel > LevelDebug {
		return
	}
	emit("debug", fmtMsg(format, args...))
}

func Errorf(format string, args ...any) {
	if minLevel > LevelError {
		return
	}
	emit("error", fmtMsg(format, args...))
}

func Fatalf(format string, args ...any) {
	msg := fmtMsg(format, args...)
	if msg == "" {
		stdlog.Fatal(levelTag("fatal"))
	}
	stdlog.Fatalf("%s [%s] %s", levelTag("fatal"), sourceTag(), msg)
}

func Info(args ...any) {
	emit("info", strings.TrimSpace(fmt.Sprint(args...)))
}

func Sprintf(format string, args ...any) string {
	return fmt.Sprintf(format, args...)
}
