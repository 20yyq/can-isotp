// @@
// @ Author       : Eacher
// @ Date         : 2024-01-05 10:21:09
// @ LastEditTime : 2024-01-05 13:41:58
// @ LastEditors  : Eacher
// @ --------------------------------------------------------------------------------<
// @ Description  :
// @ --------------------------------------------------------------------------------<
// @ FilePath     : /20yyq/can-isotp/isotp.go
// @@
package isotp

import (
	"fmt"
	"runtime"
	"sync"
	"time"

	"github.com/20yyq/packet/can"
)

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
			for err == nil {
				go lisetnFrame(frame)
				frame, err = canConn.can.ReadFrame()
			}
		}()
	}
}

func lisetnFrame(frame can.Frame) {
	var run func(can.Frame)
	pci, id := frame.Data[0]&0xF0, frame.ID()
	canConn.mutex.RLock()
	itp := canConn.isotp[id]
	canConn.mutex.RUnlock()
	switch {
	case pci != N_PCI_FC_CTS && itp == nil:
		canConn.mutex.RLock()
		for _, v := range canConn.isotp {
			v.mutex.RLock()
			c := v.conn[id]
			switch {
			case pci == N_PCI_SF && c != nil:
				run = c.singleFrame
			case pci == N_PCI_FF && c != nil:
				run = c.firstFrame
			case pci == N_PCI_CF && c != nil:
				run = c.consecutiveFrame
			}
			if run != nil {
				v.mutex.RUnlock()
				break
			}
			v.mutex.RUnlock()
		}
		canConn.mutex.RUnlock()
		if run != nil {
			break
		}
		fallthrough
	case pci == N_PCI_FC_CTS:
		if itp != nil {
			run = itp.checkFrame
			break
		}
		fallthrough
	default:
		run = otherFrame
	}
	go run(frame)
}

// func checkFrame(frame can.Frame) {
// 	var run func(can.Frame)
// 	pci, id := frame.Data[0]&0xF0, frame.ID()
// 	canConn.mutex.RLock()
// 	itp := canConn.list[id]
// 	canConn.mutex.RUnlock()
// 	switch {
// 	case pci == N_PCI_SF && itp != nil:
// 		run = itp.singleFrame
// 	case pci == N_PCI_FF && itp != nil:
// 		run = itp.firstFrame
// 	case pci == N_PCI_CF && itp != nil:
// 		run = itp.consecutiveFrame
// 	case pci == N_PCI_FC_CTS && id == canConn.id && itp == nil:
// 		canConn.mutex.RLock()
// 		for _, v := range canConn.list {
// 			switch frame.Data[0] {
// 			case N_PCI_FC_CTS:
// 				v.mutex.RLock()
// 				if itp.state == ISOTP_WAIT_FIRST_FC || itp.state == ISOTP_WAIT_FC {
// 					run = v.flowFrameCTS
// 				}
// 				v.mutex.RUnlock()
// 			case N_PCI_FC_WT:
// 				run = v.flowFrameWT
// 			case N_PCI_FC_OVFLW:
// 				run = v.flowFrameOVFLW
// 			}
// 			if run != nil {
// 				break
// 			}
// 		}
// 		canConn.mutex.RUnlock()
// 		if run != nil {
// 			break
// 		}
// 		fallthrough
// 	default:
// 		run = otherFrame
// 	}
// 	go run(frame)
// }

func writeFrame(f can.Frame) error {
	if canConn.can != nil {
		return canConn.can.WriteFrame(f)
	}
	return fmt.Errorf("not can")
}

func otherFrame(f can.Frame) {
	fmt.Println("N_PCI---------otherFrame----------N_PCI", f)
}

type isoTP struct {
	mutex *sync.RWMutex
	txid  uint32
	conn  map[uint32]*Conn
}

func (itp *isoTP) checkFrame(f can.Frame) {
	var is bool
	itp.mutex.RLock()
	for _, c := range itp.conn {
		var run func(can.Frame)
		switch f.Data[0] {
		case N_PCI_FC_CTS:
			c.mutex.RLock()
			if c.state == ISOTP_WAIT_FIRST_FC || c.state == ISOTP_WAIT_FC {
				run = c.flowFrameCTS
			}
			c.mutex.RUnlock()
		case N_PCI_FC_WT:
			run = c.flowFrameWT
		case N_PCI_FC_OVFLW:
			run = c.flowFrameOVFLW
		}
		if run != nil {
			is = true
			go run(f)
		}
	}
	itp.mutex.RUnlock()
	if !is {
		go otherFrame(f)
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
		itp = &isoTP{
			mutex: &sync.RWMutex{},
			txid:  txid,
			conn:  map[uint32]*Conn{},
		}
		canConn.isotp[txid] = itp
	}
	itp.mutex.Lock()
	defer itp.mutex.Unlock()
	c := itp.conn[rxid]
	if c == nil {
		c = &Conn{
			Config: defaultConfig,
			parent: itp,
			mutex:  &sync.RWMutex{},
			state:  ISOTP_IDLE,
			rxid:   rxid,
			ff:     can.Frame{Len: 3},
			read:   make(chan []byte, 1),
		}
		c.ff.Data = [64]byte{N_PCI_FC_CTS, defaultConfig.BS, defaultConfig.STmin}
		(&c.ff).SetID(itp.txid)
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
	Config Config
	parent *isoTP
	mutex  *sync.RWMutex
	state  uint8
	rxid   uint32
	len    uint16
	n      int
	b      [4095]byte
	bs     int8
	sn     byte
	ff     can.Frame
	read   chan []byte
}

func (c *Conn) ReadData() []byte {
	b, _ := <-c.read
	return b
}

func (c *Conn) WriteData(b []byte) error {
	c.mutex.Lock()
	defer c.mutex.Unlock()
	if c.state != ISOTP_IDLE {
		fmt.Println("---------------WriteData errors---------------")
		return fmt.Errorf("busy")
	}
	c.state = ISOTP_WAIT_FIRST_FC
	c.len, c.n, c.sn = uint16(len(b)), 6, 1
	frame := &can.Frame{Len: 8, Data: [64]byte{}}
	frame.SetID(c.rxid)
	frame.Data[0] = byte(c.len>>8) | 0x10
	frame.Data[1] = byte(c.len)
	copy(c.b[:], b)
	copy(frame.Data[2:], c.b[0:c.n])
	go writeFrame(*frame)
	return nil
}

func (c *Conn) singleFrame(f can.Frame) {
	fmt.Println("---------------singleFrame---------------")
	fmt.Println("---------------singleFrame---------------")
}

func (c *Conn) firstFrame(f can.Frame) {
	c.mutex.Lock()
	defer c.mutex.Unlock()
	if c.state != ISOTP_IDLE {
		fmt.Println("---------------firstFrame errors---------------", f, c.state)
		return
	}
	c.len, c.bs = uint16(f.Data[0]&0x0F)<<8+uint16(f.Data[1]), int8(c.Config.BS)-1
	c.n, c.ff.Data = 6, [64]byte{N_PCI_FC_CTS, c.Config.BS, c.Config.STmin}
	copy(c.b[0:], f.Data[2:c.n+2])
	go writeFrame(c.ff)
	c.state = ISOTP_WAIT_DATA
}

func (c *Conn) consecutiveFrame(f can.Frame) {
	c.mutex.Lock()
	defer c.mutex.Unlock()
	if c.state != ISOTP_WAIT_DATA {
		fmt.Println("---------------consecutiveFrame errors---------------")
		return
	}
	copy(c.b[c.n:], f.Data[1:f.Len])
	c.n += 7
	if c.n < int(c.len) {
		if c.bs > -1 {
			if c.bs--; c.bs == -1 {
				c.bs = int8(c.Config.BS) - 1
				go writeFrame(c.ff)
			}
		}
		return
	}
	c.state = ISOTP_IDLE
	go func() {
		b := make([]byte, c.len)
		copy(b, c.b[:])
		c.read <- b
		fmt.Println("---------------consecutiveFrame---------------")
		fmt.Println(runtime.NumGoroutine())
		fmt.Println("---------------consecutiveFrame---------------")
	}()
}

func (c *Conn) flowFrameCTS(f can.Frame) {
	c.mutex.Lock()
	defer c.mutex.Unlock()
	if c.state != ISOTP_WAIT_FIRST_FC && c.state != ISOTP_WAIT_FC {
		fmt.Println("---------------flowFrameCTS errors---------------")
		return
	}
	c.state, c.bs = ISOTP_SENDING, int8(f.Data[1])-1
	endTime := time.Now().Add(time.Millisecond * 127)
	if f.Data[2] > 0 {
		endTime = time.Now().Add(time.Millisecond * time.Duration(f.Data[2]))
	}
	go func(endTime time.Time) {
		for {
			if endTime.Sub(time.Now()) < 1 {
				fmt.Println("-------------------time out-------------------")
				break
			}
			if !c.loop() {
				break
			}
		}
	}(endTime)
}

func (c *Conn) loop() bool {
	c.mutex.RLock()
	state := c.state
	c.mutex.RUnlock()
	switch state {
	case ISOTP_IDLE:
		fallthrough
	case ISOTP_WAIT_FC:
		fallthrough
	default:
		return false
	case ISOTP_SENDING:
		// frame.Data[0] |= (((c.bs ^ 0xFF) % 0x10) + 1) % 0x10
		frame := &can.Frame{Len: 8, Data: [64]byte{N_PCI_CF | c.sn}}
		frame.SetID(c.rxid)
		n := c.n + 7
		if n > int(c.len) {
			n = int(c.len)
		}
		copy(frame.Data[1:], c.b[c.n:n])
		writeFrame(*frame)
		c.mutex.Lock()
		defer c.mutex.Unlock()
		if c.state == ISOTP_SENDING {
			if c.n = n; c.n == int(c.len) {
				c.state = ISOTP_IDLE
				return false
			}
			c.sn++
			if c.sn %= 0x10; c.bs > -1 {
				if c.bs--; c.bs == -1 {
					c.state = ISOTP_WAIT_FIRST_FC
					return false
				}
			}
		}
		return true
	}
}

func (c *Conn) flowFrameWT(f can.Frame) {
	c.mutex.Lock()
	defer c.mutex.Unlock()
	if c.state != ISOTP_SENDING && c.state != ISOTP_WAIT_FIRST_FC {
		if f.Len < 3 {
			fmt.Println("---------------flowFrameWT len errors---------------")
			return
		}
		fmt.Println("---------------flowFrameWT errors---------------")
		return
	}
	c.state = ISOTP_WAIT_FC
	fmt.Println("---------------flowFrameWT---------------")
	fmt.Println(f)
	fmt.Println("---------------flowFrameWT---------------")
}

func (c *Conn) flowFrameOVFLW(f can.Frame) {
	c.mutex.Lock()
	defer c.mutex.Unlock()
	if c.state != ISOTP_SENDING && c.state != ISOTP_WAIT_FIRST_FC {
		fmt.Println("---------------flowFrameOVFLW errors---------------", f)
		return
	}
	c.state = ISOTP_IDLE
	fmt.Println("---------------flowFrameOVFLW---------------")
	fmt.Println(f)
	fmt.Println("---------------flowFrameOVFLW---------------")
}
