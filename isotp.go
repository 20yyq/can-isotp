// @@
// @ Author       : Eacher
// @ Date         : 2024-01-05 10:21:09
// @ LastEditTime : 2024-01-05 16:38:01
// @ LastEditors  : Eacher
// @ --------------------------------------------------------------------------------<
// @ Description  :
// @ --------------------------------------------------------------------------------<
// @ FilePath     : /20yyq/can-isotp/isotp.go
// @@
package isotp

import (
	"fmt"
	"io"
	"sync"

	"github.com/20yyq/packet/can"
)

const MAX_PACKET = 0xFFF // 单个协议包最大长度

const N_PCI_SF = 0x00       /* single frame */
const N_PCI_FF = 0x10       /* first frame */
const N_PCI_CF = 0x20       /* consecutive frame */
const N_PCI_FC_OVFLW = 0x32 /* flow control Overflow*/
const N_PCI_FC_CTS = 0x30   /* flow control Continue To Send*/
const N_PCI_FC_WT = 0x31    /* flow control Wait*/

const (
	ISOTP_IDLE          uint8 = iota // 空闲状态
	ISOTP_WAIT_FIRST_FC              // 等待流控状态
	ISOTP_WAIT_FC                    // 等待流控状态、超时等待
	ISOTP_WAIT_DATA                  // 发送首次流控帧后等待数据状态
	ISOTP_SENDING                    // 发送数据状态
)

var canConn = struct {
	mutex sync.RWMutex
	can   Can
	isotp map[uint32]*isoTP
}{
	isotp: map[uint32]*isoTP{},
}

type Can interface {
	WriteFrame(can.Frame) error
	ReadFrame() (can.Frame, error)
}

// 初始化CAN总线数据帧接口
func Init(c Can) {
	canConn.mutex.Lock()
	defer canConn.mutex.Unlock()
	if canConn.can == nil {
		canConn.can = c
		go func() {
			frame, err := canConn.can.ReadFrame()
			for err != io.EOF {
				if err == nil {
					go listener(frame)
				}
				frame, err = canConn.can.ReadFrame()
			}
		}()
	}
}

func listener(frame can.Frame) {
	var run func(can.Frame)
	pci, id := frame.Data[0]&0xF0, frame.ID()
	canConn.mutex.RLock()
	defer canConn.mutex.RUnlock()
	itp := canConn.isotp[id]
	switch pci {
	case N_PCI_SF, N_PCI_FF, N_PCI_CF:
		for _, v := range canConn.isotp {
			v.mutex.RLock()
			c := v.conn[id]
			v.mutex.RUnlock()
			if c != nil {
				switch pci {
				case N_PCI_SF:
					run = (&c.read).single
				case N_PCI_FF:
					run = (&c.read).first
				case N_PCI_CF:
					run = (&c.read).consecutive
				}
				break
			}
		}
		if run != nil {
			break
		}
		fallthrough
	case N_PCI_FC_CTS:
		if itp != nil {
			run = itp.flowFrame
			break
		}
		fallthrough
	default:
		run = ignoreFrame
	}
	go run(frame)
}

func send(f can.Frame) error {
	if canConn.can != nil {
		return canConn.can.WriteFrame(f)
	}
	return fmt.Errorf("not can")
}

func ignoreFrame(f can.Frame) {
	fmt.Println("N_PCI---------ignoreFrame----------N_PCI", f)
}

type isoTP struct {
	mutex sync.RWMutex
	txid  uint32
	conn  map[uint32]*Conn
}

func (itp *isoTP) flowFrame(f can.Frame) {
	var is bool
	itp.mutex.RLock()
	for _, c := range itp.conn {
		var run func(can.Frame)
		switch f.Data[0] {
		case N_PCI_FC_CTS:
			c.write.mutex.RLock()
			if c.write.state == ISOTP_WAIT_FIRST_FC || c.write.state == ISOTP_WAIT_FC {
				run = (&c.write).cts
			}
			c.write.mutex.RUnlock()
		case N_PCI_FC_WT:
			run = (&c.write).wait
		case N_PCI_FC_OVFLW:
			run = (&c.write).overflow
		}
		if run != nil {
			is = true
			go run(f)
		}
	}
	itp.mutex.RUnlock()
	if !is {
		go ignoreFrame(f)
	}
	// fmt.Println("---------------(itp *isoTP) checkFrame---------------")
	// fmt.Println(f)
	// fmt.Println("---------------(itp *isoTP) checkFrame---------------")
}

func IsoTP(txid, rxid uint32) *Conn {
	canConn.mutex.Lock()
	defer canConn.mutex.Unlock()
	itp := canConn.isotp[txid]
	if itp == nil {
		itp = &isoTP{txid: txid, conn: map[uint32]*Conn{}}
		canConn.isotp[txid] = itp
	}
	itp.mutex.Lock()
	defer itp.mutex.Unlock()
	c := itp.conn[rxid]
	if c == nil {
		c = &Conn{parent: itp}
		c.read.cfg, c.read.c = defaultConfig, make(chan []byte, 5)
		c.read.ff.Len, c.read.ff.Data = 3, [64]byte{N_PCI_FC_CTS, defaultConfig.BS, defaultConfig.STmin}
		c.r, c.write.rxid = c.read.c, rxid
		(&c.read.ff).SetID(itp.txid)
		itp.conn[rxid] = c
	}
	return c
}

var defaultConfig = Config{
	STmin: 0x05,
	BS:    0x0F,
}

type Config struct {
	STmin byte // 最小间隔时间（STmin，8bit）
	BS    byte // 块大小（BS，8bit）最大为0x0F 0x00 表示再无流控帧
}

type Conn struct {
	parent *isoTP
	read   read
	write  write
	r      <-chan []byte
}

func (c *Conn) ReadData() []byte {
	b, _ := <-c.r
	return b
}

func (c *Conn) WriteData(b []byte) error {
	c.write.mutex.Lock()
	defer c.write.mutex.Unlock()
	if c.write.state != ISOTP_IDLE {
		fmt.Println("---------------WriteData errors---------------")
		return fmt.Errorf("busy")
	}
	if c.write.len = uint16(len(b)); c.write.len > MAX_PACKET {
		return fmt.Errorf("too lenght")
	}
	c.write.state, c.write.b, c.write.n, c.write.sn = ISOTP_WAIT_FIRST_FC, make([]byte, c.write.len), 6, 1
	frame := &can.Frame{Len: 8}
	frame.SetID(c.write.rxid)
	frame.Data[0], frame.Data[1] = byte(c.write.len>>8)|N_PCI_FF, byte(c.write.len)
	copy(c.write.b, b)
	copy(frame.Data[2:], c.write.b[0:c.write.n])
	go send(*frame)
	return nil
}

func (c *Conn) ResetConfig(cfg Config) {
	c.read.mutex.Lock()
	defer c.read.mutex.Unlock()
	c.read.cfg = cfg
}
