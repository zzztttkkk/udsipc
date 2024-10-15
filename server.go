package udsipc

import (
	"bufio"
	"crypto/md5"
	"encoding/hex"
	"errors"
	"fmt"
	"net"
	"os"
	"strconv"
	"strings"
	"sync/atomic"
	"time"
)

func (ipc *UdsIpc) dohandshake(pck *pack) (int64, bool) {
	parts := strings.Split(pck.plyload.String(), ":")
	if len(parts) != 2 {
		return 0, false
	}

	pid, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil {
		return 0, false
	}
	hv := md5.New()
	hv.Write([]byte(parts[0]))
	hv.Write([]byte(ipc.opts.Secret))
	if hex.EncodeToString(hv.Sum(nil)) != parts[1] {
		return 0, false
	}
	return pid, true
}

func timeout[Out any](du time.Duration, fnc func() *Out) (*Out, error) {
	timer := time.NewTimer(du)
	defer timer.Stop()

	vc := make(chan *Out)

	go func() {
		vc <- fnc()
		timer.Stop()
	}()

	for {
		select {
		case out := <-vc:
			{
				return out, nil
			}
		case <-timer.C:
			{
				return nil, errors.New("timeouted")
			}
		}
	}
}

func (ipc *UdsIpc) serveclient(c net.Conn) {
	fmt.Println(">> new conn")

	defer c.Close()

	rw := bufio.NewReadWriter(bufio.NewReader(c), bufio.NewWriter(c))

	tmppack := packpool.Get().(*pack)
	defer packpool.Put(tmppack.reset())

	ep, err := timeout(time.Millisecond*100, func() *error {
		err := readpack(rw, tmppack)
		return &err
	})
	if err != nil {
		return
	}
	if ep != nil {
		err = *ep
		if err != nil {
			return
		}
	}
	if tmppack.flags&FlagIsHandshake == 0 {
		return
	}
	pid, ok := ipc.dohandshake(tmppack)
	if !ok {
		return
	}

	tmppack.reset()
	tmppack.flags |= FlagIsHandshake
	err = ipc.sendpack(rw, tmppack)
	if err != nil {
		return
	}

	conninmain := &IpcConnInMain{
		conn:     c,
		rw:       rw,
		lastping: time.Now().UnixMilli(),
	}

	ipc.rwlock.Lock()
	ipc.clis[pid] = conninmain
	ipc.rwlock.Unlock()

	fmt.Println("Server Conn OK", pid)

	defer func() {
		ipc.rwlock.Lock()
		delete(ipc.clis, pid)
		ipc.rwlock.Unlock()
	}()

	readec := make(chan error)

	closeByPid := func(cpid int64) {
		fc := ipc.clis[cpid]
		if fc != nil {
			fc.conn.Close()
		}
		delete(ipc.clis, cpid)
	}

	go func() {
		for {
			tmppack.reset()
			err := readpack(rw, tmppack)
			if err != nil {
				readec <- err
				break
			}
			if tmppack.flags&FlagIsPing != 0 {
				atomic.StoreInt64(&conninmain.lastping, time.Now().UnixMilli())
				fmt.Println("Server: ping from cli", pid)
				continue
			}

			var sendfaileds []int64

			ipc.rwlock.RLock()
			for cpid, cc := range ipc.clis {
				if cpid == pid {
					if tmppack.flags&FlagSelfCanRecv == 0 {
						continue
					}
				}
				se := ipc.sendpack(cc.rw, tmppack)
				if se != nil {
					sendfaileds = append(sendfaileds, cpid)
				}
			}
			ipc.rwlock.RUnlock()

			if len(sendfaileds) > 0 {
				ipc.rwlock.Lock()
				for _, fpid := range sendfaileds {
					closeByPid(fpid)
				}
				ipc.rwlock.Unlock()
			}
		}
	}()

	ticker := time.NewTicker(time.Millisecond * time.Duration(ipc.opts.PingStepInMills))
	max_ping_step := ipc.opts.PingStepInMills * 2

loop:
	for {
		select {
		case re := <-readec:
			{
				fmt.Println(re)
				break loop
			}
		case <-ticker.C:
			{
				lastpingat := atomic.LoadInt64(&conninmain.lastping)
				now := time.Now().UnixMilli()
				if now-lastpingat > max_ping_step {
					break loop
				}

				wpack := packpool.Get().(*pack)

				ipc.mkpingpack(wpack.reset())
				err = ipc.sendpack(rw, wpack)
				if err != nil {
					break loop
				}
			}
		}
	}

}

var (
	ErrorIpcWriteEmptyConnection = errors.New("write to empty connection")
)

func (ipc *UdsIpc) servemain(listener net.Listener) {
	fmt.Println(">>>>>>>>>>>>>>>>>>Main<<<<<<<<<<<<<<<<<<<<<<", os.Getpid())
	ipc.ismain = true
	clear(ipc.clis)

	for {
		c, e := listener.Accept()
		if e != nil {
			continue
		}
		go ipc.serveclient(c)
	}
}
