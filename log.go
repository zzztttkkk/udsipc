package udsipc

import "time"

type LogLevel int

const (
	LogLevelTrace = LogLevel(iota)
	LogLevelDebug
	LogLevelInfo
	LogLevelWarn
	LogLevelError
)

func (ipc *UdsIpc) log(level LogLevel, msg string, args ...any) {
	if ipc.opts.OnLog == nil {
		return
	}
	ipc.opts.OnLog(level, time.Now().UnixMilli(), msg, args...)
}
