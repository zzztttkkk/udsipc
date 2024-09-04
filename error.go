package udsipc

type IpcErrorKind int

const (
	IpcErrorConnReadFailed = IpcErrorKind(iota)
	IpcErrorConnWriteFailed
	IpcErrorConnHandshakeFailed
	IpcErrorMakePackFailed
	IpcErrorPingTimeout
)

type IpcError struct {
	kind IpcErrorKind
	err  error
}
