package rpc

import (
	"bytes"
	"time"

	"github.com/xdrpp/goxdr/xdr"
)

type Message struct {
	bytes.Buffer
	Peer string
	pool MessagePool
	//
	enteredQueue time.Time
	ioLatency    time.Duration
	queueLatency time.Duration
	serdeLatency time.Duration
	report       MsgReportFunc
}

type MsgReportFunc func(ioLatency time.Duration, queueLatency time.Duration, serdeLatency time.Duration)

func (m *Message) Recycle() {
	if m.report != nil {
		m.report(m.ioLatency, m.queueLatency, m.serdeLatency)
	}
	m.pool.Recycle(m)
}

func (m *Message) In() xdr.XDR {
	return xdr.XdrIn{In: m}
}

func (m *Message) Out() xdr.XDR {
	return xdr.XdrOut{Out: m}
}

func (m *Message) Xid() uint32 {
	if bs := m.Bytes(); len(bs) >= 4 {
		return uint32(bs[0])<<24 | uint32(bs[1])<<16 |
			uint32(bs[2])<<8 | uint32(bs[3])
	}
	return 0
}

func (m *Message) Serialize(vs ...xdr.XdrType) {
	t0 := time.Now()
	out := m.Out()
	for i := range vs {
		vs[i].XdrMarshal(out, "")
	}
	m.serdeLatency = time.Since(t0)
}

func (m *Message) SetReport(report MsgReportFunc) {
	m.report = report
}

func (m *Message) CancelReport() {
	m.report = nil
}

func (m *Message) SetIOLatency(l time.Duration) {
	m.ioLatency = l
}

func (m *Message) SetSerdeLatency(l time.Duration) {
	m.serdeLatency = l
}

func (m *Message) EnteringQueue() {
	m.enteredQueue = time.Now()
}

func (m *Message) LeavingQueue() {
	m.queueLatency = time.Since(m.enteredQueue)
}
