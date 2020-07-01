package main

/* important setting
$ cat /etc/udev/rules.d/99-local.rules
# for USB-RS422
KERNEL=="ttyUSB*",  ATTRS{idVendor}=="0403", ATTRS{idProduct}=="6001", SYMLINK+="ttyUSB_Serial"
*/

import (
	"bufio"
	"flag"
	"github.com/hypebeast/go-osc/osc"
	"github.com/tarm/serial"
	_ "io"
	"log"
	"time"
	"unsafe"
)

var (
	osc_host, osc_addr, serial_device string
	osc_port                          int
	osc_client                        *osc.Client
	serial_port                       *serial.Port
	serial_config                     *serial.Config
	debug_mode                        bool
	last_sec, fps_counter             int
)

func main() {
	setup()
	connect_timer()
}

func setup() {
	flag.StringVar(&osc_host, "osc_host", "224.0.0.0", "OSC Network Address")
	flag.IntVar(&osc_port, "osc_port", 7000, "OSC Network Port")
	flag.StringVar(&osc_addr, "osc_addr", "/camera", "OSC Address")
	flag.StringVar(&serial_device, "device", "/dev/ttyUSB_Serial", "DEVICE NAME")
	flag.BoolVar(&debug_mode, "debug", false, "DEBUG MODE")
	flag.Parse()

	osc_client = osc.NewClient(osc_host, osc_port)

	serial_config = &serial.Config{
		Baud:     38400,
		Name:     serial_device,
		StopBits: serial.Stop1,
		Parity:   serial.ParityOdd,
	}
}

func connect_timer() {
	var err error
	serial_port, err = serial.OpenPort(serial_config)
	if err != nil {
		log.Println(err)
		reconnect_timer()
	}

	scan_timer()
}

func reconnect_timer() {
	time.Sleep(time.Second * 1)
	connect_timer()
}

func scan_timer() {
	scanner := bufio.NewScanner(serial_port)
	scanner.Split(bufio.ScanBytes)

	data, pos := initData()

	for scanner.Scan() {
		b := scanner.Bytes()[0]

		b &= 0xff

		if pos <= 0 && b == '\xd1' {
			data, pos = initData()
			data[0] = b
			pos = 1
			continue
		}

		if pos > 0 && pos < 29 {
			data[pos] = b
			pos += 1
		}

		if pos == 29 {
			if isValidData(data) {
				handleData(data)
				data, pos = initData()
			} else {
				log.Println("false!!!", data)
				pos = -1
			}
		}
	}

	reconnect_timer()
}

func isValidData(b []byte) bool {
	if b[1] != 1 {
		return false
	}
	if !checkSum(b) {
		return false
	}
	return true
}

func checkSum(b []byte) bool {
	s := 0
	for i := 0; i < (len(b) - 1); i++ {
		s += int(b[i])
	}
	cs := uint8(0x40 - (s & 0xff))
	return b[28] == cs
}

func initData() ([]byte, int) {
	return make([]byte, 29), 0
}

func measureFps() {
	m := time.Now()
	if last_sec != m.Second() {
		log.Printf("FPS: %d\n", fps_counter)
		last_sec = m.Second()
		fps_counter = 0
	}
	fps_counter += 1
}

func handleData(b []byte) {
	ry := float32(bytes_to_int32(b[2:5])) / 32768.0
	rx := float32(bytes_to_int32(b[5:8])) / 32768.0
	rz := float32(bytes_to_int32(b[8:11])) / 32768.0
	tx := float32(bytes_to_int32(b[12:15])) / 64.0
	ty := float32(bytes_to_int32(b[15:18])) / 64.0
	tz := float32(bytes_to_int32(b[18:21])) / 64.0
	zoom := bytes_to_int32(b[20:23])
	focus := bytes_to_int32(b[23:26])

	go broadcast(tx, ty, tz, rx, ry, rz, zoom, focus)

	measureFps()
}

func broadcast(tx float32, ty float32, tz float32, rx float32, ry float32, rz float32, zoom int32, focus int32) {
	msg := osc.NewMessage(osc_addr)
	msg.Append(tx)
	msg.Append(ty)
	msg.Append(tz)
	msg.Append(rx)
	msg.Append(ry)
	msg.Append(rz)
	msg.Append(zoom)
	msg.Append(focus)
	osc_client.Send(msg)

	if debug_mode {
		log.Printf("%s %f %f %f %f %f %f %d %d\n", osc_addr, tx, ty, tz, rx, ry, rz, zoom, focus)
	}
}

func bytes_to_int32(b []byte) int32 {
	return int32((_lrotl(uint32(b[0]), 24)&0xff000000 |
		_lrotl(uint32(b[1]), 16)&0x00ff0000 |
		_lrotl(uint32(b[2]), 8)&0x0000ff00)) / 0x100
}

func _lrotl(value uint32, shift int) uint32 {
	max_bits := int(unsafe.Sizeof(value) << 3)
	if shift < 0 {
		return _lrotr(value, -shift)
	}
	if shift > max_bits {
		shift = shift % max_bits
	}
	return (value << shift) | (value >> (max_bits - shift))
}

func _lrotr(value uint32, shift int) uint32 {
	max_bits := int(unsafe.Sizeof(value) << 3)
	if shift < 0 {
		return _lrotl(value, -shift)
	}
	if shift > max_bits {
		shift = shift % max_bits
	}
	return (value >> shift) | (value << (max_bits - shift))
}
