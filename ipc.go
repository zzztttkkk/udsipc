package udsipc

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"reflect"
	"sync"
	"time"
)

type UdsIpcOpts struct {
	FilePath          string
	Secret            string
	EnableCompression bool
	CompressionLimit  int
	PingStepInMills   int64
	OnLog             func(level LogLevel, timeunix int64, msg string, args ...any)
	AfterHandshake    func(ipc *UdsIpc)
}

type msginqueue struct {
	msg    IMessage
	waiter chan error
}

type UdsIpc struct {
	opts   UdsIpcOpts
	ismain bool

	rwlock   sync.RWMutex
	types    map[string]reflect.Type
	msgqueue chan msginqueue
	clis     map[int64]*IpcConnInMain

	onmsgfnc func(msg IMessage)
}

func NewUnixIpc(opts UdsIpcOpts) *UdsIpc {
	if opts.PingStepInMills == 0 {
		opts.PingStepInMills = 100
	}
	if opts.PingStepInMills < 10 {
		opts.PingStepInMills = 10
	}

	return &UdsIpc{
		opts:     opts,
		types:    make(map[string]reflect.Type),
		msgqueue: make(chan msginqueue),
		clis:     map[int64]*IpcConnInMain{},
	}
}

func (*UdsIpc) registertype(typ reflect.Type) string {
	return fmt.Sprintf("%s:%s", typ.PkgPath(), typ.Name())
}

type IpcConnInMain struct {
	conn net.Conn
	rw   interface {
		io.ReadWriter
		Flush() error
	}
	lastping int64
}

var (
	ErrHandshakeFailed = errors.New("udsipc: handshake failed")
	ErrDailFailed      = errors.New("udsipc: dail failed")
)

func (ipc *UdsIpc) dial() (net.Conn, error) {
	var dialer net.Dialer

	for i := 0; i < 10; i++ {
		ctx, cancel := context.WithTimeout(context.Background(), time.Millisecond*100)
		c, e := dialer.DialContext(ctx, "unix", ipc.opts.FilePath)
		cancel()
		if e != nil {
			continue
		}
		return c, nil
	}
	return nil, ErrDailFailed
}

func (ipc *UdsIpc) listen() error {
	_, err := os.Stat(ipc.opts.FilePath)
	if err == nil {
		return errors.New("sock file exists")
	}
	serv, err := net.Listen("unix", ipc.opts.FilePath)
	if err == nil {
		go ipc.servemain(serv)
	}
	return err
}

func (ipc *UdsIpc) Run() {
	for {
		ipc.listen()
		c, e := ipc.dial()
		if e != nil {
			os.Remove(ipc.opts.FilePath)
			continue
		}
		ace := ipc.asclient(c)
		fmt.Println("ACE:", ace)
	}
}
