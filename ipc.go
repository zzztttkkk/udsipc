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
}

type msginqueue struct {
	msg    IMessage
	waiter chan struct{}
}

type UdsIpc struct {
	opts   UdsIpcOpts
	ismain bool

	rwlock   sync.RWMutex
	types    map[string]reflect.Type
	msgqueue chan msginqueue

	onmsgfnc func(msg IMessage)
}

func NewUnixIpc(opts UdsIpcOpts) *UdsIpc {
	return &UdsIpc{opts: opts}

}

func (*UdsIpc) registertype(typ reflect.Type) string {
	fmt.Println(typ.Name(), typ.PkgPath())
	return ""
}

type IpcConnInMain struct {
	rw interface {
		io.ReadWriter
		Flush() error
	}
	lastping int64
}

type IpcMainStatus struct {
	rw    sync.RWMutex
	conns map[uint64]*IpcConnInMain
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
	serv, err := net.Listen("unix", ipc.opts.FilePath)
	if err != nil {
		err = os.Remove(ipc.opts.FilePath)
		if err != nil {
			return err
		}
		serv, err = net.Listen("unix", ipc.opts.FilePath)
	}

	if err != nil {
		go ipc.servemain(serv)
	}
	return err
}

func (ipc *UdsIpc) Run() {
	for {
		ipc.listen()
		c, e := ipc.dial()
		if e != nil {
			continue
		}
		ipc.asclient(c)
	}
}
