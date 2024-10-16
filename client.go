package udsipc

import (
	"bufio"
	"fmt"
	"net"
	"os"
	"reflect"
	"sync/atomic"
	"time"
)

func (ipc *UdsIpc) asclient(c net.Conn) *IpcError {
	rw := bufio.NewReadWriter(bufio.NewReader(c), bufio.NewWriter(c))

	hspack := packpool.Get().(*pack)
	defer packpool.Put(hspack.reset())

	err := ipc.sendpack(rw, ipc.mkhandshakepack(hspack))
	if err != nil {
		return &IpcError{kind: IpcErrorConnWriteFailed, err: err}
	}
	err = readpack(rw, hspack.reset())
	if err != nil {
		return &IpcError{kind: IpcErrorConnReadFailed, err: err}
	}
	if hspack.flags&FlagIsHandshake == 0 {
		return &IpcError{kind: IpcErrorConnHandshakeFailed}
	}

	var lastpingat = time.Now().UnixMilli()
	var max_ping_step = ipc.opts.PingStepInMills * 2
	var ticker = time.NewTicker(time.Millisecond * time.Duration(ipc.opts.PingStepInMills))
	defer ticker.Stop()

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
				fmt.Println("Cli: ping from main", os.Getpid())
				continue
			}

			tpy, ok := ipc.types[rpack.eventname]
			if !ok {
				continue
			}

			msg := reflect.New(tpy).Interface().(IMessage)
			err = msg.FromBuffer(rpack.plyload)
			if err != nil {
				break
			}
			go ipc.onmsgfnc(msg)
		}
		readerrchan <- err
	}()

	fmt.Println("Cli Conn OK")

	if ipc.opts.AfterHandshake != nil {
		ipc.opts.AfterHandshake(ipc)
	}

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
				if msgiq.waiter != nil {
					msgiq.waiter <- err
				}

				if err != nil {
					return &IpcError{kind: IpcErrorConnWriteFailed, err: err}
				}
				break
			}
		case <-ticker.C:
			{
				lastpingatv := atomic.LoadInt64(&lastpingat)
				now := time.Now().UnixMilli()
				if now-lastpingatv > max_ping_step {
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
