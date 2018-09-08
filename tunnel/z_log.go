package tunnel

import (
	"io"
	"os"
	"runtime"
	"time"
	"github.com/op/go-logging"
	"fmt"
)

var log = logging.MustGetLogger("example")

var format_stderr = logging.MustStringFormatter(
	`%{time:15:04:05.000} %{level:.3s} %{message}`,
)
var format_file = logging.MustStringFormatter(
	`%{time:2006-01-02 15:04:05.000} %{level:.3s} %{message}`,
)

const (
	LLError uint8 = iota
	LLWarn
	LLInfo
	LLDebug
)

func InitLogger(w io.Writer, pre uint16) {
	preStr := fmt.Sprintf("%5d ", pre)
	backend_stderr := logging.NewLogBackend(os.Stderr, preStr, 0)
	ft_stderr := logging.NewBackendFormatter(backend_stderr, format_stderr)

	backend_wfile := logging.NewLogBackend(w, preStr, 0)
	ft_wfile := logging.NewBackendFormatter(backend_wfile, format_file)
	wfile_leveled := logging.AddModuleLevel(ft_wfile)
	wfile_leveled.SetLevel(logging.WARNING, "")

	logging.SetBackend(wfile_leveled, ft_stderr)

}

func init() {

}

func Debug(format string, a ...interface{}) {
	if LogLevel >= LLDebug {
		log.Debugf(format, a...)
	}
}

func Info(format string, a ...interface{}) {
	if LogLevel >= LLInfo {
		log.Infof(format, a...)
	}
}

func Error(format string, a ...interface{}) {
	if LogLevel >= LLError {
		log.Errorf(format, a...)

	}
	if ExitOnError {
		time.Sleep(1)
		os.Exit(4)
	}
}

func Warn(format string, a ...interface{}) {
	if LogLevel >= LLWarn {
		log.Warningf(format, a...)

	}
}

func LogStack(format string, a ...interface{}) {
	log.Errorf(format, a...)

	buf := make([]byte, 32768)
	runtime.Stack(buf, true)
	log.Errorf("!!!!!stack!!!!!: %s", buf)

}

func Panic(format string, a ...interface{}) {
	LogStack(format, a...)
	panic("!!")
}

func Fail() {
	log.Error("Fail never")
	panic("NEVER")
}

func Recover() {
	if err := recover(); err != nil {
		LogStack("goroutine failed:%v", err)
	}
}

const Milli = int64(time.Millisecond) / int64(time.Nanosecond)

func TimeNowMs() int64 {
	return time.Now().UnixNano() / Milli
}

func Reverse(numbers []byte) []byte {
	newNumbers := make([]byte, len(numbers))
	for i, j := 0, len(numbers)-1; i < j; i, j = i+1, j-1 {
		newNumbers[i], newNumbers[j] = numbers[j], numbers[i]
	}
	return newNumbers
}
