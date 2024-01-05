// @@
// @ Author       : Eacher
// @ Date         : 2024-01-05 16:22:17
// @ LastEditTime : 2024-01-05 16:37:49
// @ LastEditors  : Eacher
// @ --------------------------------------------------------------------------------<
// @ Description  :
// @ --------------------------------------------------------------------------------<
// @ FilePath     : /20yyq/can-isotp/read.go
// @@
package isotp

import (
	"fmt"
	"runtime"
	"sync"

	"github.com/20yyq/packet/can"
)

type read struct {
	cfg   Config
	mutex sync.RWMutex
	state uint8
	ff    can.Frame
	len   uint16
	n     int
	b     [MAX_PACKET]byte
	bs    int8
	c     chan []byte
}

func (rx *read) single(f can.Frame) {
	fmt.Println("---------------single---------------")
}

func (rx *read) first(f can.Frame) {
	rx.mutex.Lock()
	defer rx.mutex.Unlock()
	if rx.state != ISOTP_IDLE {
		fmt.Println("---------------first errors---------------", f, rx.state)
		return
	}
	rx.len, rx.bs = uint16(f.Data[0]&0x0F)<<8+uint16(f.Data[1]), int8(rx.cfg.BS)-1
	rx.n, rx.ff.Data = 6, [64]byte{N_PCI_FC_CTS, rx.cfg.BS, rx.cfg.STmin}
	copy(rx.b[0:], f.Data[2:rx.n+2])
	go send(rx.ff)
	rx.state = ISOTP_WAIT_DATA
}

func (rx *read) consecutive(f can.Frame) {
	rx.mutex.Lock()
	defer rx.mutex.Unlock()
	if rx.state != ISOTP_WAIT_DATA {
		fmt.Println("---------------consecutive errors---------------", f)
		return
	}
	copy(rx.b[rx.n:], f.Data[1:f.Len])
	rx.n += 7
	if rx.n < int(rx.len) {
		if rx.bs > -1 {
			if rx.bs--; rx.bs == -1 {
				rx.bs = int8(rx.cfg.BS) - 1
				go send(rx.ff)
			}
		}
		return
	}
	rx.state = ISOTP_IDLE
	go func() {
		b := make([]byte, rx.len)
		copy(b, rx.b[:])
		// TODO 是否考虑做丢弃处理
		// for len(rx.c) == cap(rx.c) {
		// 	<-rx.c
		// }
		rx.c <- b
		fmt.Println("---------------consecutive---------------")
		fmt.Println(runtime.NumGoroutine())
		fmt.Println("---------------consecutive---------------")
	}()
}
