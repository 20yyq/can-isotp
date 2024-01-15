// @@
// @ Author       : Eacher
// @ Date         : 2024-01-05 16:22:59
// @ LastEditTime : 2024-01-15 16:08:42
// @ LastEditors  : Eacher
// @ --------------------------------------------------------------------------------<
// @ Description  :
// @ --------------------------------------------------------------------------------<
// @ FilePath     : /20yyq/can-isotp/write.go
// @@
package isotp

import (
	"fmt"
	"sync"
	"time"

	"github.com/20yyq/packet/can"
)

type write struct {
	cfg   Config
	mutex sync.RWMutex
	timer *time.Timer
	txid  uint32
	state uint8
	len   uint16
	n     int
	b     []byte
	bs    int8
	sn    byte
}

func (tx *write) loop() bool {
	tx.mutex.RLock()
	state := tx.state
	tx.mutex.RUnlock()
	switch state {
	case ISOTP_SENDING:
		// frame.Data[0] |= (((tx.bs ^ 0xFF) % N_PCI_FF) + 1) % N_PCI_FF
		n := tx.n + int(tx.cfg.dlc) - 1
		if n > int(tx.len) {
			n = int(tx.len)
		}
		frame := &can.Frame{Len: uint8(n - tx.n + 1), Data: [64]byte{N_PCI_CF | tx.sn}}
		frame.SetID(tx.txid)
		copy(frame.Data[1:], tx.b[tx.n:n])
		if err := send(*frame); err != nil {
			// TODO debug 写入错误
			fmt.Println("---------------write loop send frame err---------------", err)
			return true
		}
		tx.mutex.Lock()
		defer tx.mutex.Unlock()
		if tx.state != ISOTP_SENDING {
			break
		}
		if tx.n = n; tx.n == int(tx.len) {
			tx.timer.Reset(0)
			break
		}
		tx.sn++
		if tx.sn %= N_PCI_FF; tx.bs > -1 {
			if tx.bs--; tx.bs == -1 {
				tx.state = ISOTP_WAIT_FC
				break
			}
		}
		return true
	case ISOTP_IDLE:
	case ISOTP_WAIT_FC:
	default:
	}
	return false
}

func (tx *write) cts(f can.Frame) {
	tx.mutex.Lock()
	defer tx.mutex.Unlock()
	switch tx.state {
	case ISOTP_SENDING:
		tx.state = ISOTP_WAIT_FC
		// fmt.Println("---------------wait---------------", f)
	case ISOTP_WAIT_FIRST_FC:
		if f.Data[2] > 0 && f.Data[2] < 0x80 {
			tx.timer.Reset(time.Millisecond * time.Duration(f.Data[2]+5) * time.Duration(tx.len/uint16(f.Len)))
		} else if f.Data[2] > 0xF0 && f.Data[2] < 0xFA {
			tx.timer.Reset(time.Microsecond * 105 * time.Duration(f.Data[2]&0x0F) * time.Duration(tx.len/uint16(f.Len)))
		}
		fallthrough
	case ISOTP_WAIT_FC:
		tx.state, tx.bs = ISOTP_SENDING, int8(f.Data[1])-1
		var d time.Duration
		if f.Data[2] < 0x80 {
			d = time.Millisecond * time.Duration(f.Data[2])
		} else if f.Data[2] > 0xF0 && f.Data[2] < 0xFA {
			d = time.Microsecond * 100 * time.Duration(f.Data[2]&0x0F)
		}
		go func(d time.Duration) {
			for {
				time.Sleep(d)
				if !tx.loop() {
					break
				}
			}
		}(d)
	}
}

func (tx *write) overflow(f can.Frame) {
	tx.mutex.Lock()
	defer tx.mutex.Unlock()
	if tx.state != ISOTP_SENDING && tx.state != ISOTP_WAIT_FC {
		return
	}
	tx.timer.Reset(0)
	// fmt.Println("---------------overflow---------------", f)
}
