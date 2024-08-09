// @@
// @ Author       : Eacher
// @ Date         : 2024-01-05 10:21:09
// @ LastEditTime : 2024-07-30 15:01:06
// @ LastEditors  : Eacher
// @ --------------------------------------------------------------------------------<
// @ Description  :
// @ --------------------------------------------------------------------------------<
// @ FilePath     : /20yyq/can-isotp/isotp.go
// @@
package isotp

import (
	"bytes"
	"fmt"
	"io"
	"sync"
	"time"

	"github.com/20yyq/packet/can"
	"golang.org/x/sys/unix"
)

const N_PCI_SF = 0x00 // single frame
const N_PCI_FF = 0x10 // first frame
const N_PCI_CF = 0x20 // consecutive frame

type flow_control uint8

const (
	N_PCI_FC_OVFLW flow_control = 0x32 // flow control Overflow
	N_PCI_FC_CTS   flow_control = 0x30 // flow control Continue To Send
	N_PCI_FC_WT    flow_control = 0x31 // flow control Wait

)

const N_PCI_SZ = 1        /* size of the PCI byte #1 */
const SF_PCI_SZ4 = 1      /* size of SingleFrame PCI including 4 bit SF_DL */
const SF_PCI_SZ8 = 2      /* size of SingleFrame PCI including 8 bit SF_DL */
const FF_PCI_SZ12 = 2     /* size of FirstFrame PCI including 12 bit FF_DL */
const FF_PCI_SZ32 = 6     /* size of FirstFrame PCI including 32 bit FF_DL */
const FC_CONTENT_SZ = 3   /* flow control content size in byte (FS/BS/STmin) */
const MAX_FF_DL12 = 0xFFF /* max 12 bit data length FF_DL */

const (
	ISOTP_IDLE          uint8 = iota // 空闲状态
	ISOTP_WAIT_FIRST_FC              // 等待流控状态
	ISOTP_WAIT_FC                    // 等待流控状态、超时等待
	ISOTP_WAIT_DATA                  // 发送首次流控帧后等待数据状态
	ISOTP_WAIT_FF_SF                 // 等待单帧或者首帧
	ISOTP_SEND_SF                    // 发送单帧
	ISOTP_SEND_FF                    // 发送首帧
	ISOTP_SEND_CF                    // 发送连续帧
	ISOTP_SEND_END                   // 发送结束
	ISOTP_SENDING
)

var canConn = struct {
	mutex sync.RWMutex
	can   Can
	list  map[uint32]*Conn
}{
	list: map[uint32]*Conn{},
}

type Can interface {
	AddCanFilter(unix.CanFilter) error
	WriteFrame(can.Frame) error
	ReadFrame() (can.Frame, error)
}

// 初始化CAN总线数据帧接口
func Init(c Can) {
	canConn.mutex.Lock()
	defer canConn.mutex.Unlock()
	if canConn.can == nil {
		canConn.can = c
		go listener()
	}
}

func listener() {
	rcv := make(chan can.Frame, 30)
	go func() {
		frame, err := canConn.can.ReadFrame()
		for err != io.EOF {
			if err == nil {
				rcv <- frame
			}
			frame, err = canConn.can.ReadFrame()
		}
		close(rcv)
	}()
	for {
		frame, ok := <-rcv
		if !ok {
			break
		}
		id, run := frame.ID(), ignoreFrame
		if frame.Extended {
			id |= can.FlagExtended
		}
		canConn.mutex.RLock()
		if conn := canConn.list[id]; conn != nil {
			pci := frame.Data[0] & 0xF0
			run = nil
			switch pci {
			case byte(N_PCI_FC_CTS):
				(&conn.write).cts(frame)
			case N_PCI_SF:
				(&conn.read).single(frame)
			case N_PCI_FF:
				(&conn.read).first(frame)
			case N_PCI_CF:
				(&conn.read).consecutive(frame)
			default:
				run = flowFrame
			}
		}
		if run != nil {
			go run(frame)
		}
		canConn.mutex.RUnlock()
	}
	fmt.Println("can close......")
}

func ignoreFrame(f can.Frame) {
	fmt.Println("N_PCI---------ignoreFrame----------N_PCI", f)
}

func flowFrame(f can.Frame) {
	fmt.Println("N_PCI---------flowFrame----------N_PCI", f)
}

func send(f can.Frame) error {
	if canConn.can != nil {
		return canConn.can.WriteFrame(f)
	}
	return fmt.Errorf("can not init")
}

// txid 目标ID rxid 本地监听ID
func IsoTP(rxcfg, txcfg Config) *Conn {
	canConn.mutex.Lock()
	defer canConn.mutex.Unlock()
	filter, id := unix.CanFilter{Id: rxcfg.ID, Mask: 0x7FF}, rxcfg.ID
	if rxcfg.IsExt {
		filter.Mask = 0x8FFFFFFF
		id |= can.FlagExtended
	}
	conn := canConn.list[id]
	if conn == nil {
		err := canConn.can.AddCanFilter(filter)
		if err == nil {
			conn = &Conn{buf: bytes.NewBuffer(nil)}
			conn.read.cfg, conn.read.pip, conn.read.rcv = rxcfg, make(chan []byte, 5), make(chan can.Frame, 10)
			conn.write.cfg, conn.read.send_fc = txcfg, (&conn.write).send_fc
			canConn.list[id] = conn
			conn.read.buf, conn.read.timer = bytes.NewBuffer(nil), time.NewTimer(0)
			conn.read.state.Store(uint32(ISOTP_WAIT_FF_SF))
		}
	}
	return conn
}

type Config struct {
	ID      uint32 // CAN ID
	STmin   byte   // 连续帧最小间隔时间（STmin，8bit）
	BS      byte   // 块大小（BS，4bit）最大为0x0F 0x00 表示再无流控帧
	ExtAddr uint8  // 每帧首个字节为ISOTP扩展ID
	IsFD    bool
	IsExt   bool
	dlc     uint8
}

type Conn struct {
	mutex sync.RWMutex
	buf   *bytes.Buffer
	read  read
	write write
}

func (c *Conn) Read(b []byte) (int, error) {
	n, err := c.buf.Read(b)
	if n < len(b) {
		b1, ok := <-c.read.pip
		if !ok {
			return 0, io.ErrClosedPipe
		}
		if len(b[n:]) < len(b1) {
			c.buf.Write(b1[len(b[n:]):])
			b1 = b1[:len(b[n:])]
		}
		copy(b[n:], b1)
		n += len(b1)
	}
	return n, err
}

func (c *Conn) Write(b []byte) (int, error) {
	if c.write.close.Load() {
		return 0, io.ErrClosedPipe
	}
	if uint8(c.write.state.Load()) != ISOTP_IDLE {
		return 0, fmt.Errorf("busy")
	}
	if old := c.write.state.Swap(uint32(ISOTP_WAIT_FIRST_FC)); old != uint32(ISOTP_IDLE) {
		c.write.state.Store(old)
		return 0, fmt.Errorf("busy")
	}
	c.write.mutex.Lock()
	defer c.write.mutex.Unlock()
	c.write.n, c.write.b = 0, b
	c.write.len = uint16(len(b))
	c.write.timer.Reset(time.Second)
	// max sf_dl
	// sf_dl := c.write.cfg.dlc - SF_PCI_SZ8 - 1
	// not ext addr and one byte
	// if (!(sk->tx.addr.flags & ISOTP_PKG_EXT_ADDR))
	// 	sf_dl++;
	// // not can fd and one byte
	// if (!(sk->tx.addr.flags & ISOTP_PKG_FDF))
	// 	sf_dl++;
	// if (len > sf_dl)
	// 	func = isotp_send_ff;
	go c.write.first()
	<-c.write.timer.C
	switch uint8(c.write.state.Load()) {
	case ISOTP_WAIT_FIRST_FC:
		fmt.Println("--------ISOTP_WAIT_FIRST_FC-------")
	case ISOTP_WAIT_FC:
		fmt.Println("--------ISOTP_WAIT_FC-------")
	case ISOTP_SEND_SF:
		fmt.Println("--------ISOTP_SEND_SF-------")
	case ISOTP_SEND_FF:
		fmt.Println("--------ISOTP_SEND_FF-------")
	case ISOTP_SEND_CF:
		fmt.Println("--------ISOTP_SEND_CF-------")
	case ISOTP_SEND_END:
		fmt.Println("--------ISOTP_SEND_END-------")
	default:
		fmt.Println("--------------------------")
	}
	c.write.state.Store(uint32(ISOTP_IDLE))
	return len(b), nil
}

func (c *Conn) Close() error {
	c.read.close.Store(true)
	c.write.close.Store(true)
	return nil
}

// func (c *Conn) ResetConfig(cfg Config) error {
// 	if uint8(c.read.state.Load()) != ISOTP_WAIT_FF_SF || uint8(c.write.state.Load()) != ISOTP_IDLE {
// 		return fmt.Errorf("busy")
// 	}
// 	if cfg.dlc = 8; cfg.IsFD {
// 		cfg.dlc = 64
// 	}
// 	if cfg.BS > 0x0F {
// 		cfg.BS = 0x0F
// 	}
// 	c.write.cfg = cfg
// 	c.read.cfg = cfg
// 	return nil
// }
