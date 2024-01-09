// @@
// @ Author       : Eacher
// @ Date         : 2024-01-05 16:22:59
// @ LastEditTime : 2024-01-09 15:41:39
// @ LastEditors  : Eacher
// @ --------------------------------------------------------------------------------<
// @ Description  :
// @ --------------------------------------------------------------------------------<
// @ FilePath     : /20yyq/can-isotp/write.go
// @@
package isotp

import (
	"sync"
	"time"

	"github.com/20yyq/packet/can"
)

type write struct {
	cfg   Config
	mutex sync.RWMutex
	timer *time.Timer
	rxid  uint32
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
		frame.SetID(tx.rxid)
		copy(frame.Data[1:], tx.b[tx.n:n])
		canConn.write <- frame
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

func (tx *write) runLoop(f can.Frame) {
	tx.state, tx.bs = ISOTP_SENDING, int8(f.Data[1])-1
	endTime := time.Now().Add(time.Millisecond * 127)
	if f.Data[2] > 0 {
		endTime = time.Now().Add(time.Millisecond * time.Duration(f.Data[2]))
	}
	go func(endTime time.Time) {
		for {
			if endTime.Sub(time.Now()) < 1 {
				break
			}
			if !tx.loop() {
				break
			}
		}
	}(endTime)
}

func (tx *write) cts(f can.Frame) {
	tx.mutex.Lock()
	defer tx.mutex.Unlock()
	if tx.state != ISOTP_SENDING && tx.state != ISOTP_WAIT_FC && tx.state != ISOTP_WAIT_FIRST_FC {
		return
	}
	if tx.state == ISOTP_WAIT_FIRST_FC {
		tx.runLoop(f)
		return
	}
	if tx.state == ISOTP_SENDING {
		tx.state = ISOTP_WAIT_FC
		// fmt.Println("---------------wait---------------", f)
		return
	}
	tx.runLoop(f)
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
