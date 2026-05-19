package log

import (
	"fmt"
	"log"
	"os"
)

var std = log.New(os.Stdout, "", log.LstdFlags)

func Infof(format string, args ...any) {
	std.Printf("[INFO] "+format, args...)
}

func Warnf(format string, args ...any) {
	std.Printf("[WARN] "+format, args...)
}

func Errorf(format string, args ...any) {
	std.Printf("[ERROR] "+format, args...)
}

func Debugf(format string, args ...any) {
	std.Printf("[DEBUG] "+format, args...)
}

func Info(args ...any) {
	std.Println(append([]any{"[INFO]"}, args...)...)
}

func Fatalf(format string, args ...any) {
	std.Fatalf("[FATAL] "+format, args...)
}

func Sprintf(format string, args ...any) string {
	return fmt.Sprintf(format, args...)
}
