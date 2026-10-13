package main

import (
	"fmt"
	"net"
	"os"
	"time"

	"golang.org/x/net/icmp"
	"golang.org/x/net/ipv4"
)

func doICMPPing(ipStr string, count int, timeout time.Duration) *PingResult {
	conn, err := icmp.ListenPacket("ip4:icmp", "0.0.0.0")
	if err != nil {
		return &PingResult{OK: false, Error: "нет прав на ICMP-сокет: " + err.Error()}
	}
	defer conn.Close()

	dstAddr := &net.IPAddr{IP: net.ParseIP(ipStr)}
	if dstAddr.IP == nil {
		return &PingResult{OK: false, Error: "невалидный IP"}
	}

	id := os.Getpid() & 0xFFFF
	var rtts []float64
	deadline := time.Now().Add(timeout)

	for seq := 1; seq <= count; seq++ {
		if time.Now().After(deadline) {
			break
		}

		echoBody := &icmp.Echo{
			ID:   id,
			Seq:  seq,
			Data: []byte("checker-go"),
		}
		msg := &icmp.Message{
			Type: ipv4.ICMPTypeEcho,
			Code: 0,
			Body: echoBody,
		}
		msgBytes, err := msg.Marshal(nil)
		if err != nil {
			return &PingResult{OK: false, Error: "marshal: " + err.Error()}
		}

		sendTime := time.Now()
		if _, err := conn.WriteTo(msgBytes, dstAddr); err != nil {
			return &PingResult{OK: false, Error: "send: " + err.Error()}
		}

		// Wait for reply
		remaining := time.Until(deadline)
		if remaining < time.Second {
			remaining = time.Second
		}
		conn.SetReadDeadline(time.Now().Add(remaining))

		reply := make([]byte, 1500)
		n, _, err := conn.ReadFrom(reply)
		if err != nil {
			continue
		}
		if n < 8 {
			continue
		}

		replyMsg, err := icmp.ParseMessage(1, reply[:n])
		if err != nil {
			continue
		}
		if replyMsg.Type != ipv4.ICMPTypeEchoReply {
			continue
		}
		if echoReply, ok := replyMsg.Body.(*icmp.Echo); ok {
			if echoReply.ID != id {
				continue
			}
		}
		rtts = append(rtts, time.Since(sendTime).Seconds())

		if seq < count {
			time.Sleep(500 * time.Millisecond)
		}
	}

	lost := count - len(rtts)
	if len(rtts) == 0 {
		return &PingResult{OK: false, Error: "timeout (нет ответов)", Lost: lost}
	}
	var sum float64
	for _, r := range rtts {
		sum += r
	}
	return &PingResult{
		OK:     true,
		AVGRtt: sum / float64(len(rtts)),
		Lost:   lost,
	}
}

var _ = fmt.Sprintf
