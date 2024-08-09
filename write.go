// @@
// @ Author       : Eacher
// @ Date         : 2024-01-05 16:22:59
// @ LastEditTime : 2024-07-30 15:58:47
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
	"sync/atomic"
	"time"

	"github.com/20yyq/packet/can"
)

type write struct {
	mutex sync.RWMutex
	cfg   Config
	timer *time.Timer
	state atomic.Uint32
	d     time.Duration
	close atomic.Bool
	len   uint16
	n     int
	b     []byte
	bs    int8
	sn    byte
}

func (tx *write) cts(f can.Frame) {
	old := uint8(tx.state.Load())
	if old != ISOTP_WAIT_FIRST_FC && old != ISOTP_WAIT_FC {
		return
	}
	if state := tx.state.Swap(uint32(ISOTP_SENDING)); state != uint32(old) {
		tx.state.Store(uint32(state))
		return
	}
	switch old {
	case ISOTP_WAIT_FIRST_FC:
		tx.bs = int8(f.Data[1]) - 1
		switch {
		case f.Data[2] < 0x80:
			tx.d = time.Millisecond * time.Duration(f.Data[2])
		case f.Data[2] > 0xF0 && f.Data[2] < 0xFA:
			tx.d = time.Microsecond * 100 * time.Duration(f.Data[2]&0x0F)
		default:
			tx.d = time.Millisecond * time.Duration(0x7F)
		}
	case ISOTP_WAIT_FC:

	default:
		fmt.Println("---------------wait---------------", f)
		return
	}
	switch f.Data[0] {
	case byte(N_PCI_FC_CTS):
		time.AfterFunc(tx.d, tx.send_cf)
	case byte(N_PCI_FC_WT):
		tx.state.Store(uint32(old))
		tx.timer.Reset(time.Millisecond * 200)
	case byte(N_PCI_FC_OVFLW):
		fallthrough
	default:
		tx.state.Store(uint32(ISOTP_IDLE))
		return
	}
}

func (tx *write) single() {
	frame := &can.Frame{Len: tx.cfg.dlc, Extended: tx.cfg.IsExt, CanFd: tx.cfg.IsFD}
	frame.SetID(tx.cfg.ID)
	item := 0
	if tx.cfg.ExtAddr > 0 {
		frame.Data[item], item = tx.cfg.ExtAddr, 1
	}
	frame.Data[item] = uint8(N_PCI_SF | tx.len)
	item++
	copy(frame.Data[item:], tx.b)
	if err := send(*frame); err != nil {
		tx.timer.Reset(0)
		fmt.Println(err)
		return
	}
}

func (tx *write) first() {
	frame := &can.Frame{Len: tx.cfg.dlc, Extended: tx.cfg.IsExt, CanFd: tx.cfg.IsFD}
	frame.SetID(tx.cfg.ID)
	item := 0
	if tx.cfg.ExtAddr > 0 {
		frame.Data[item], item = tx.cfg.ExtAddr, 1
	}
	frame.Data[item] = uint8(N_PCI_FF | tx.len>>8)
	frame.Data[item+1] = uint8(tx.len & 0xFF)
	if item += 2; tx.len > MAX_FF_DL12 {
		frame.Data[item-2] = N_PCI_FF
		frame.Data[item-1] = 0
		frame.Data[item] = uint8(tx.len >> 24 & 0xFF)
		frame.Data[item+1] = uint8(tx.len >> 16 & 0xFF)
		frame.Data[item+2] = uint8(tx.len >> 8 & 0xFF)
		frame.Data[item+3] = uint8(tx.len & 0xFF)
		item += 4
	}
	tx.n, tx.sn = int(tx.cfg.dlc)-item, 1
	copy(frame.Data[item:], tx.b[:tx.n])
	if err := send(*frame); err != nil {
		tx.timer.Reset(0)
		fmt.Println(err)
		return
	}
}

func (tx *write) send_cf() {
	tx.timer.Reset(time.Millisecond * 200)
	tx.state.Store(uint32(ISOTP_SEND_CF))
	frame := &can.Frame{Len: tx.cfg.dlc, Extended: tx.cfg.IsExt, CanFd: tx.cfg.IsFD}
	frame.SetID(tx.cfg.ID)
	item := 0
	if tx.cfg.ExtAddr > 0 {
		frame.Data[item], item = tx.cfg.ExtAddr, 1
	}
	pcilen := uint8(N_PCI_SZ + item)
	space := tx.cfg.dlc - uint8(pcilen)
	if tx.len-uint16(tx.n) < uint16(space) {
		frame.Len = uint8(tx.len-uint16(tx.n)) + pcilen
	}
	frame.Data[item] = N_PCI_CF | tx.sn
	item++
	tx.sn++
	tx.sn %= 16
	for i := item; i < int(frame.Len); i++ {
		frame.Data[i] = tx.b[tx.n]
		tx.n++
	}
	copy(frame.Data[item:], tx.b[:tx.n])
	if err := send(*frame); err != nil {
		tx.timer.Reset(0)
		fmt.Println(err)
		return
	}
	if tx.bs++; tx.cfg.BS > 0 && byte(tx.bs) == tx.cfg.BS {
		tx.state.Store(uint32(ISOTP_WAIT_FC))
		return
	}
	if tx.n >= int(tx.len) {
		tx.state.Store(uint32(ISOTP_SEND_END))
		tx.timer.Reset(0)
		return
	}
	time.AfterFunc(tx.d, tx.send_cf)
}

func (tx *write) send_fc(fc flow_control, bs, stmin uint8) error {
	frame := &can.Frame{Len: 3, Extended: tx.cfg.IsExt, CanFd: tx.cfg.IsFD}
	frame.SetID(tx.cfg.ID)
	if frame.Data = [64]byte{uint8(fc), bs, stmin}; tx.cfg.ExtAddr > 0 {
		frame.Len, frame.Data = 4, [64]byte{tx.cfg.ExtAddr, uint8(fc), bs, stmin}
	}
	return send(*frame)
}
