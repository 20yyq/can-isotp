// @@
// @ Author       : Eacher
// @ Date         : 2024-01-05 16:22:17
// @ LastEditTime : 2024-07-30 15:58:23
// @ LastEditors  : Eacher
// @ --------------------------------------------------------------------------------<
// @ Description  :
// @ --------------------------------------------------------------------------------<
// @ FilePath     : /20yyq/can-isotp/read.go
// @@
package isotp

import (
	"bytes"
	"fmt"
	"sync/atomic"
	"time"

	"github.com/20yyq/packet/can"
)

type read struct {
	cfg     Config
	timer   *time.Timer
	rcv     chan can.Frame
	pip     chan []byte
	state   atomic.Uint32
	close   atomic.Bool
	buf     *bytes.Buffer
	send_fc func(flow_control, uint8, uint8) error
	len     uint32
	bs      int8
	sn      byte
}

func (rx *read) single(f can.Frame) {
	if rx.close.Load() {
		fmt.Println("sf close......", f)
		return
	}
	if uint8(rx.state.Load()) != ISOTP_WAIT_FF_SF {
		fmt.Println("rx.state != ISOTP_WAIT_SF", rx.state.Load(), f)
		return
	}
	go func(b []byte) {
		rx.pip <- b
		if rx.close.Load() {
			close(rx.pip)
		}
	}(f.Data[1:f.Len])
}

func (rx *read) first(f can.Frame) {
	if rx.close.Load() {
		fmt.Println("ff close......", f)
		return
	}
	if uint8(rx.state.Load()) != ISOTP_WAIT_FF_SF {
		fmt.Println("rx.state != ISOTP_WAIT_FF", rx.state.Load(), f)
		return
	}
	rx.state.Store(uint32(ISOTP_WAIT_DATA))
	item := uint8(2)
	rx.sn, rx.bs, rx.len = 1, 0, uint32(f.Data[0]&0x0F)<<8+uint32(f.Data[1])
	if rx.len < 1 {
		rx.len, item = uint32(f.Data[2])<<24, 6
		rx.len += uint32(f.Data[3]) << 16
		rx.len += uint32(f.Data[4]) << 8
		rx.len += uint32(f.Data[5])
	}
	rx.buf.Reset()
	rx.buf.Write(f.Data[item:f.Len])
	if err := rx.send_fc(N_PCI_FC_CTS, rx.cfg.BS, rx.cfg.STmin); err != nil {
		// rx.timer.Reset(0)
		fmt.Println("send_fc error", err)
	}
	go rx.receive()
}

func (rx *read) consecutive(f can.Frame) {
	if uint8(rx.state.Load()) != ISOTP_WAIT_DATA {
		fmt.Println("rx.state != ISOTP_WAIT_DATA", rx.state.Load(), f)
		return
	}
	rx.rcv <- f
}

func (rx *read) receive() {
	rx.timer.Reset(time.Second)
	for {
		select {
		case <-rx.timer.C:
			fmt.Println("read stop", rx.len, rx.buf.Len(), rx.state.Load())
			rx.timer.Stop()
			rx.state.Store(uint32(ISOTP_WAIT_FF_SF))
			return
		case frame := <-rx.rcv:
			if (frame.Data[0] & 0x0F) != rx.sn {
				fmt.Println("frame.Data[0] & 0x0F != rx.sn", frame.Data[0]&0x0F, rx.sn)
				break
			}
			rx.sn++
			rx.sn %= N_PCI_FF
			rx.buf.Write(frame.Data[1:frame.Len])
			if rx.buf.Len() >= int(rx.len) {
				b := make([]byte, rx.buf.Len())
				copy(b, rx.buf.Bytes())
				rx.timer.Reset(0)
				go func() {
					rx.pip <- b
					if rx.close.Load() {
						close(rx.pip)
					}
				}()
				break
			}
			rx.timer.Reset(time.Millisecond * 200)
			if rx.bs++; rx.cfg.BS > 0 && rx.bs == int8(rx.cfg.BS) {
				rx.bs = 0
				if err := rx.send_fc(N_PCI_FC_CTS, rx.cfg.BS, rx.cfg.STmin); err != nil {
					fmt.Println("send_fc error", err)
					// rx.timer.Reset(0)
				}
			}
		}
	}
}
