package udsipc

import (
	"errors"
	"fmt"
	"net"
)

func (ipc *UdsIpc) serveclient(ms *IpcMainStatus, c net.Conn) {
}

var (
	ErrorIpcWriteEmptyConnection = errors.New("write to empty connection")
)

func (ipc *UdsIpc) servemain(listener net.Listener) {
	fmt.Println(">>>>>>>>>>>>>>>>>>Main<<<<<<<<<<<<<<<<<<<<<<")
	ipc.ismain = true

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
