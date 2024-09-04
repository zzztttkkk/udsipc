package udsipc

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"reflect"
	"sync"
	"sync/atomic"
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

	w        IFlushWriter
	onmsgfnc func([]byte)
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

func (ipc *UdsIpc) serveclient(ms *IpcMainStatus, c net.Conn) {
}

var (
	ErrorIpcWriteEmptyConnection = errors.New("write to empty connection")
)

func (ipc *UdsIpc) servemain(listener net.Listener, errchan chan *IpcError) {
	fmt.Println(">>>>>>>>>>>>>>>>>>Main<<<<<<<<<<<<<<<<<<<<<<")

	var ms IpcMainStatus
	ms.conns = map[uint64]*IpcConnInMain{}

	for {
		c, e := listener.Accept()
		if e != nil {
			continue
		}
		go ipc.serveclient(&ms, c)
	}
}

var (
	ErrHandshakeFailed = errors.New("udsipc: handshake failed")
)

func (ipc *UdsIpc) asclient(c net.Conn) *IpcError {
	handshakepack := packpool.Get().(*pack)
	defer packpool.Put(handshakepack.reset())

	rw := bufio.NewReadWriter(bufio.NewReader(c), bufio.NewWriter(c))

	err := ipc.sendpack(rw, ipc.mkhandshakepack(handshakepack))
	if err != nil {
		return &IpcError{kind: IpcErrorConnWriteFailed, err: err}
	}
	err = readpack(rw, handshakepack.reset())
	if err != nil {
		return &IpcError{kind: IpcErrorConnReadFailed, err: err}
	}
	if handshakepack.flags&FlagIsHandshake == 0 {
		return &IpcError{kind: IpcErrorConnHandshakeFailed}
	}

	var lastpingat = time.Now().UnixMilli()
	var pingstep = ipc.opts.PingStepInMills
	var pinged = int32(0)

	var readerrchan = make(chan error)

	go func() {
		rpack := packpool.Get().(*pack)
		defer packpool.Put(rpack.reset())
		var err error
		for {
			err = readpack(rw, rpack.reset())
			if err != nil {
				break
			}
			if rpack.flags&FlagIsPing != 0 {
				atomic.StoreInt64(&lastpingat, time.Now().UnixMilli())
				atomic.StoreInt32(&pinged, 0)
				continue
			}
			// TODO onmessage
		}
		readerrchan <- err
	}()

	wpack := packpool.Get().(*pack)
	defer packpool.Put(wpack)

	for {
		select {
		case rerr := <-readerrchan:
			{
				return &IpcError{kind: IpcErrorConnReadFailed, err: rerr}
			}
		case msgiq := <-ipc.msgqueue:
			{
				err = ipc.mkmessagepack(wpack.reset(), msgiq.msg)
				if err != nil {
					return &IpcError{kind: IpcErrorMakePackFailed, err: err}
				}
				err = ipc.sendpack(rw, wpack)
				if err != nil {
					return &IpcError{kind: IpcErrorConnWriteFailed, err: err}
				}
				if msgiq.waiter != nil {
					msgiq.waiter <- struct{}{}
				}
				break
			}
		default:
			{
				if time.Now().UnixMilli()-atomic.LoadInt64(&lastpingat) < pingstep {
					continue
				}

				if !atomic.CompareAndSwapInt32(&pinged, 0, 1) {
					return &IpcError{kind: IpcErrorPingTimeout}
				}

				ipc.mkpingpack(wpack.reset())
				err = ipc.sendpack(rw, wpack)
				if err != nil {
					return &IpcError{kind: IpcErrorConnWriteFailed, err: err}
				}
			}
		}
	}
}

func (ipc *UdsIpc) dial() (net.Conn, error) {
	var dialer net.Dialer
	ctx, cancel := context.WithTimeout(context.Background(), time.Millisecond*100)
	defer cancel()
	return dialer.DialContext(ctx, "unix", ipc.opts.FilePath)
}

func (ipc *UdsIpc) listen() (net.Listener, error) {
	serv, err := net.Listen("unix", ipc.opts.FilePath)
	if err != nil {
		err = os.Remove(ipc.opts.FilePath)
		if err != nil {
			return nil, err
		}
		serv, err = net.Listen("unix", ipc.opts.FilePath)
	}
	return serv, err
}

func (ipc *UdsIpc) Start() {
	for {
		conn, err := ipc.dial()
		var mainerrchan chan *IpcError
		if err != nil {
			listener, err := ipc.listen()
			if err != nil {
				continue
			}

			mainerrchan = make(chan *IpcError)
			go ipc.servemain(listener, mainerrchan)
			conn, err = ipc.dial()
			if err != nil {
				continue
			}
		}
		ipc.asclient(conn)
	}
}
