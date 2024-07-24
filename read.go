// @@
// @ Author       : Eacher
// @ Date         : 2024-01-05 16:22:17
// @ LastEditTime : 2024-01-16 10:07:55
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
	rx.mutex.Lock()
	defer rx.mutex.Unlock()
	if rx.state == ISOTP_IDLE {
		go func(b []byte) {
			// TODO 是否考虑做丢弃处理
			// for len(rx.c) == cap(rx.c) {
			// 	<-rx.c
			// }
			rx.c <- b
		}(f.Data[1 : f.Len-1])
	}
}

func (rx *read) first(f can.Frame) {
	rx.mutex.Lock()
	defer rx.mutex.Unlock()
	if rx.state == ISOTP_IDLE {
		rx.ff.Data = [64]byte{N_PCI_FC_CTS, rx.cfg.BS, rx.cfg.STmin}
		copy(rx.b[0:], f.Data[2:f.Len])
		if err := send(rx.ff); err != nil {
			// TODO debug 写入错误
			fmt.Println("---------------read first send frame err---------------", err)
			return
		}
		rx.sn, rx.len, rx.bs = 1, uint16(f.Data[0]&0x0F)<<8+uint16(f.Data[1]), int8(rx.cfg.BS)-1
		rx.state, rx.n = ISOTP_WAIT_DATA, int(f.Len-2)
		d := time.Second
		if rx.cfg.STmin > 0 && rx.cfg.STmin < 0x80 {
			d = time.Millisecond * time.Duration(rx.cfg.STmin+30) * time.Duration(rx.len/uint16(rx.cfg.dlc))
		} else if rx.cfg.STmin > 0xF0 && rx.cfg.STmin < 0xFA {
			d = time.Microsecond * 120 * time.Duration(rx.cfg.STmin&0x0F) * time.Duration(rx.len/uint16(rx.cfg.dlc))
		}
		rx.timer.Reset(d)
		go func() {
			<-rx.timer.C
			if !rx.timer.Reset(time.Hour * 24 * 365) {
				rx.timer.Reset(time.Hour * 24 * 365)
			}
			rx.mutex.Lock()
			defer rx.mutex.Unlock()
			rx.state = ISOTP_IDLE
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
				if err := send(rx.ff); err != nil {
					// TODO debug 写入错误
					fmt.Println("---------------read consecutive send frame err---------------", err)
				}
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
