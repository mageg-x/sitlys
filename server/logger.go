// Package main - 日志模块
// 提供分级日志功能，支持 debug、info、warn、error 四个级别
package main

import (
	"fmt"
	"io"
	"log"
	"os"
	"sync"
	"time"
)

// LogLevel - 日志级别类型
type LogLevel int

// 日志级别常量定义
const (
	LogLevelDebug LogLevel = iota // 调试级别：详细的调试信息
	LogLevelInfo                  // 信息级别：常规运行信息
	LogLevelWarn                  // 警告级别：潜在问题提示
	LogLevelError                 // 错误级别：错误和异常信息
)

// 日志级别名称映射
var (
	logLevelNames = map[LogLevel]string{
		LogLevelDebug: "DEBUG",
		LogLevelInfo:  "INFO",
		LogLevelWarn:  "WARN",
		LogLevelError: "ERROR",
	}
)

// Logger - 日志器结构
// 支持多级别日志输出，线程安全
type Logger struct {
	mu       sync.RWMutex // 读写锁，保护并发访问
	level    LogLevel     // 当前日志级别
	output   io.Writer    // 日志输出目标
	infoLog  *log.Logger  // 信息日志记录器
	debugLog *log.Logger  // 调试日志记录器
	warnLog  *log.Logger  // 警告日志记录器
	errorLog *log.Logger  // 错误日志记录器
}

// defaultLogger - 默认日志器实例
var defaultLogger *Logger

// init - 初始化默认日志器
func init() {
	defaultLogger = NewLogger(LogLevelInfo, os.Stdout)
}

// NewLogger - 创建新的日志器实例
// 参数:
//   - level: 日志级别
//   - output: 输出目标
//
// 返回:
//   - 配置好的日志器实例
func NewLogger(level LogLevel, output io.Writer) *Logger {
	l := &Logger{
		level:  level,
		output: output,
	}
	l.infoLog = log.New(output, "", 0)
	l.debugLog = log.New(output, "", 0)
	l.warnLog = log.New(output, "", 0)
	l.errorLog = log.New(output, "", 0)
	return l
}

// SetLevel - 设置日志级别
func (l *Logger) SetLevel(level LogLevel) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.level = level
}

// GetLevel - 获取当前日志级别
func (l *Logger) GetLevel() LogLevel {
	l.mu.RLock()
	defer l.mu.RUnlock()
	return l.level
}

// log - 内部日志输出方法
// 根据级别过滤并格式化输出日志
func (l *Logger) log(level LogLevel, format string, args ...any) {
	l.mu.RLock()
	currentLevel := l.level
	l.mu.RUnlock()

	// 低于当前级别的日志不输出
	if level < currentLevel {
		return
	}

	// 格式化日志行：[时间] [级别] 消息
	timestamp := time.Now().Format("2006-01-02 15:04:05")
	levelName := logLevelNames[level]
	msg := fmt.Sprintf(format, args...)
	line := fmt.Sprintf("[%s] [%s] %s", timestamp, levelName, msg)

	// 根据级别选择对应的日志记录器
	switch level {
	case LogLevelDebug:
		l.debugLog.Println(line)
	case LogLevelInfo:
		l.infoLog.Println(line)
	case LogLevelWarn:
		l.warnLog.Println(line)
	case LogLevelError:
		l.errorLog.Println(line)
	}
}

// Debug - 输出调试级别日志
func (l *Logger) Debug(format string, args ...any) {
	l.log(LogLevelDebug, format, args...)
}

// Info - 输出信息级别日志
func (l *Logger) Info(format string, args ...any) {
	l.log(LogLevelInfo, format, args...)
}

// Warn - 输出警告级别日志
func (l *Logger) Warn(format string, args ...any) {
	l.log(LogLevelWarn, format, args...)
}

// Error - 输出错误级别日志
func (l *Logger) Error(format string, args ...any) {
	l.log(LogLevelError, format, args...)
}

// SetLogLevel - 设置默认日志器的级别
func SetLogLevel(level LogLevel) {
	defaultLogger.SetLevel(level)
}

// SetLogOutput - 设置默认日志器的输出目标
func SetLogOutput(output io.Writer) {
	defaultLogger.mu.Lock()
	defer defaultLogger.mu.Unlock()
	defaultLogger.output = output
	defaultLogger.infoLog = log.New(output, "", 0)
	defaultLogger.debugLog = log.New(output, "", 0)
	defaultLogger.warnLog = log.New(output, "", 0)
	defaultLogger.errorLog = log.New(output, "", 0)
}

// Debug - 使用默认日志器输出调试日志
func Debug(format string, args ...any) {
	defaultLogger.Debug(format, args...)
}

// Info - 使用默认日志器输出信息日志
func Info(format string, args ...any) {
	defaultLogger.Info(format, args...)
}

// Warn - 使用默认日志器输出警告日志
func Warn(format string, args ...any) {
	defaultLogger.Warn(format, args...)
}

// Error - 使用默认日志器输出错误日志
func Error(format string, args ...any) {
	defaultLogger.Error(format, args...)
}

// ParseLogLevel - 解析日志级别字符串
// 支持的值：debug、info、warn、error（不区分大小写）
func ParseLogLevel(s string) LogLevel {
	switch s {
	case "debug", "DEBUG":
		return LogLevelDebug
	case "info", "INFO":
		return LogLevelInfo
	case "warn", "WARN", "warning", "WARNING":
		return LogLevelWarn
	case "error", "ERROR":
		return LogLevelError
	default:
		return LogLevelInfo
	}
}
