// @@
// @ Author       : Eacher
// @ Date         : 2024-01-05 16:22:17
// @ LastEditTime : 2024-01-09 15:34:35
// @ LastEditors  : Eacher
// @ --------------------------------------------------------------------------------<
// @ Description  :
// @ --------------------------------------------------------------------------------<
// @ FilePath     : /20yyq/can-isotp/read.go
// @@
package isotp

import (
	"fmt"
	"sync"
	"time"

	"github.com/20yyq/packet/can"
)

type read struct {
	cfg   Config
	mutex sync.Mutex
	timer *time.Timer
	state uint8
	ff    can.Frame
	len   uint16
	n     int
	b     [MAX_PACKET]byte
	bs    int8
	sn    byte
	c     chan []byte
}

func (rx *read) single(f can.Frame) {
	fmt.Println("---------------single---------------")
}

func (rx *read) first(f can.Frame) {
	rx.mutex.Lock()
	defer rx.mutex.Unlock()
	if rx.state == ISOTP_IDLE {
		rx.sn, rx.len, rx.bs = 1, uint16(f.Data[0]&0x0F)<<8+uint16(f.Data[1]), int8(rx.cfg.BS)-1
		rx.state, rx.n, rx.ff.Data = ISOTP_WAIT_DATA, int(f.Len-2), [64]byte{N_PCI_FC_CTS, rx.cfg.BS, rx.cfg.STmin}
		copy(rx.b[0:], f.Data[2:f.Len])
		go func(frame can.Frame) { canConn.write <- &frame }(rx.ff)
		rx.timer.Reset(time.Millisecond * time.Duration(rx.cfg.N_Re))
		go func() {
			// start := time.Now()
			<-rx.timer.C
			if !rx.timer.Reset(time.Hour * 24 * 365) {
				rx.timer.Reset(time.Hour * 24 * 365)
			}
			rx.mutex.Lock()
			defer rx.mutex.Unlock()
			rx.state = ISOTP_IDLE
			// fmt.Println(runtime.NumGoroutine(), time.Now().Sub(start).Milliseconds())
		}()
	}
}

func (rx *read) consecutive(f can.Frame) {
	rx.mutex.Lock()
	defer rx.mutex.Unlock()
	if rx.state != ISOTP_WAIT_DATA {
		return
	}
	if (f.Data[0] & 0x0F) != rx.sn {
		rx.timer.Reset(0)
		return
	}
	rx.sn++
	rx.sn %= N_PCI_FF
	copy(rx.b[rx.n:], f.Data[1:f.Len])
	if rx.n += int(f.Len) - 1; rx.n < int(rx.len) {
		if rx.bs > -1 {
			if rx.bs--; rx.bs == -1 {
				rx.bs = int8(rx.cfg.BS) - 1
				go func(frame can.Frame) { canConn.write <- &frame }(rx.ff)
			}
		}
		return
	}
	b := make([]byte, rx.len)
	copy(b, rx.b[:])
	rx.timer.Reset(0)
	go func(b []byte) {
		// TODO 是否考虑做丢弃处理
		// for len(rx.c) == cap(rx.c) {
		// 	<-rx.c
		// }
		rx.c <- b
	}(b)
}
