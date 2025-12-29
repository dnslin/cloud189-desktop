package main

import (
	"log"
	"os"
)

// TUILogger 适配 core/httpclient.Logger，将日志打印到终端。
type TUILogger struct {
	logger *log.Logger
}

// NewTUILogger 返回默认的终端日志记录器。
func NewTUILogger() *TUILogger {
	return &TUILogger{
		logger: log.New(os.Stderr, "", log.LstdFlags),
	}
}

// Debugf 输出调试级别日志，前缀为中文。
func (l *TUILogger) Debugf(format string, args ...any) {
	if l == nil || l.logger == nil {
		return
	}
	l.logger.Printf("[调试] "+format, args...)
}

// Errorf 输出错误级别日志，前缀为中文。
func (l *TUILogger) Errorf(format string, args ...any) {
	if l == nil || l.logger == nil {
		return
	}
	l.logger.Printf("[错误] "+format, args...)
}
