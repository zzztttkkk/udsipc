package udsipc

import (
	"bufio"
	"net"
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

			var playload = make([]byte, rpack.plyload.Len())
			copy(playload, rpack.plyload.Bytes())
			go ipc.onmsgfnc(playload)
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
