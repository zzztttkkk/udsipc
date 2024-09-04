package udsipc

import (
	"bytes"
	"compress/gzip"
	"crypto/md5"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"reflect"
	"strconv"
	"sync"
)

type IMessage interface {
	ToBuffer(buf *bytes.Buffer) error
	FromBuffer(buf *bytes.Buffer) error
}

const (
	FlagIsCompressed   = 0b0000_0001
	FlagIsPing         = 0b0000_0010
	FlagIsHandshake    = 0b0000_0100
	FlagLengthIsUInt64 = 0b0000_1000
	FlagSelfCanRecv    = 0b0001_0000
)

type pack struct {
	flags     byte
	eventname string
	plyload   *bytes.Buffer
}

func (p *pack) reset() *pack {
	p.flags = 0
	p.eventname = ""
	p.plyload.Reset()
	return p
}

var (
	packpool = sync.Pool{
		New: func() any {
			val := &pack{
				plyload: bytes.NewBuffer(make([]byte, 1024)),
			}
			return val
		},
	}
)

var (
	bufpool = sync.Pool{
		New: func() any {
			return bytes.NewBuffer(nil)
		},
	}
)

var (
	ErrorIpcMessageTooLarge = errors.New("IpcMessageTooLarge")
)

func readpack(r io.Reader, dest *pack) error {
	var rtmp [64]byte

	// read flags
	_, err := io.ReadFull(r, rtmp[:1])
	if err != nil {
		return err
	}
	dest.flags = rtmp[0]

	// read eventname
	var nametmp = make([]byte, 0, 24)
	for {
		_, err = io.ReadFull(r, rtmp[:1])
		if err != nil {
			return err
		}
		if rtmp[0] == 0 {
			break
		}
		nametmp = append(nametmp, rtmp[0])
	}
	dest.eventname = string(nametmp)

	var playloadlen uint64

	// read msglen
	if dest.flags&FlagLengthIsUInt64 != 0 {
		_, err = io.ReadFull(r, rtmp[:8])
		if err != nil {
			return err
		}
		playloadlen = binary.BigEndian.Uint64(rtmp[:8])
	} else {
		_, err = io.ReadFull(r, rtmp[:2])
		if err != nil {
			return err
		}
		playloadlen = uint64(binary.BigEndian.Uint16((rtmp[:2])))
	}

	if playloadlen > uint64(math.MaxInt) {
		return ErrorIpcMessageTooLarge
	}

	remain := int(playloadlen)
	for {
		if remain < 1 {
			break
		}
		var rtmp = rtmp[:]
		if remain < 128 {
			rtmp = rtmp[:remain]
		}

		rl, err := r.Read(rtmp)
		if err != nil {
			return err
		}
		dest.plyload.Write(rtmp[:rl])
		remain -= rl
		if remain < 1 {
			break
		}
	}
	return nil
}

type IFlushWriter interface {
	io.Writer
	Flush() error
}

func (ipc *UdsIpc) sendpack(w IFlushWriter, src *pack) error {
	var playload = src.plyload
	if ipc.opts.EnableCompression && ipc.opts.CompressionLimit > 0 && uint64(src.plyload.Len()) > uint64(ipc.opts.CompressionLimit) {
		buf := bufpool.Get().(*bytes.Buffer)
		defer bufpool.Put(buf)

		cw := gzip.NewWriter(buf)
		_, e := cw.Write(src.plyload.Bytes())
		if e != nil {
			return e
		}
		cw.Flush()
		playload = buf
		src.flags = src.flags | FlagIsCompressed
	}

	w.Write([]byte{src.flags})
	w.Write([]byte(src.eventname))
	_, err := w.Write([]byte{0})
	if err != nil {
		return err
	}

	var wtmp = [8]byte{}
	if playload.Len() > math.MaxUint16 {
		binary.BigEndian.PutUint64(wtmp[:], uint64(playload.Len()))
		_, err = w.Write(wtmp[:8])
		if err != nil {
			return err
		}
	} else {
		binary.BigEndian.PutUint16(wtmp[:2], uint16(playload.Len()))
		_, err = w.Write(wtmp[:2])
		if err != nil {
			return err
		}
	}
	w.Write(playload.Bytes())
	return w.Flush()
}

func (ipc *UdsIpc) mkhandshakepack(dest *pack) *pack {
	pid := strconv.FormatInt(int64(os.Getpid()), 10)
	hv := md5.New()
	hv.Write([]byte(pid))
	hv.Write([]byte(ipc.opts.Secret))
	dest.plyload.WriteString(fmt.Sprintf("%s:%s", pid, hex.EncodeToString(hv.Sum(nil))))
	return dest
}

func (ipc *UdsIpc) mkpingpack(dest *pack) {
	dest.flags = FlagIsPing
}

func (ipc *UdsIpc) mkhandshakeresppack(dest *pack) {
	dest.flags = FlagIsHandshake
}

func (ipc *UdsIpc) mkmessagepack(dest *pack, msg IMessage) error {
	ipc.rwlock.Lock()
	name := ipc.registertype(reflect.TypeOf(msg))
	ipc.rwlock.Unlock()

	dest.eventname = name
	return msg.ToBuffer(dest.plyload)
}
