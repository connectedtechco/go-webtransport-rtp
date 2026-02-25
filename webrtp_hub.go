package webrtp

import (
	"encoding/binary"
	"sync"
	"sync/atomic"
	"time"
)

type Hub struct {
	mu          sync.RWMutex
	clients     map[chan []byte]struct{}
	init        []byte
	bytesRecv   atomic.Uint64
	frameNo     atomic.Uint64
	clientCount atomic.Int32
	ready       atomic.Bool
	readyAt     atomic.Pointer[time.Time]
	lastFrameAt atomic.Pointer[time.Time]
	codec       string
	width       int
	height      int
	frameRate   float64
}

func NewHub() *Hub {
	return &Hub{
		clients: make(map[chan []byte]struct{}),
	}
}

func (r *Hub) SetInit(data []byte) {
	r.mu.Lock()
	r.init = make([]byte, len(data))
	copy(r.init, data)
	r.mu.Unlock()
	r.ready.Store(true)
	now := time.Now()
	r.readyAt.Store(&now)
}

func (r *Hub) Reset() {
	r.ready.Store(false)
	r.readyAt.Store(nil)
	r.lastFrameAt.Store(nil)
	r.mu.Lock()
	r.init = nil
	r.mu.Unlock()
}

func (r *Hub) IsReceivingFrames() bool {
	lastFrameAt := r.lastFrameAt.Load()
	if lastFrameAt == nil {
		return false
	}
	return time.Since(*lastFrameAt) < time.Second
}

func (r *Hub) SetInfo(codec string, width, height int, frameRate float64) {
	r.mu.Lock()
	r.codec = codec
	r.width = width
	r.height = height
	r.frameRate = frameRate
	r.mu.Unlock()
}

func (r *Hub) SetFramerate(framerate float64) {
	r.mu.Lock()
	r.frameRate = framerate
	r.mu.Unlock()
}

func (r *Hub) GetInit() []byte {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.init
}

func (r *Hub) Subscribe() chan []byte {
	ch := make(chan []byte, 1)
	r.mu.Lock()
	r.clients[ch] = struct{}{}
	r.mu.Unlock()
	r.clientCount.Add(1)
	return ch
}

func (r *Hub) Unsubscribe(ch chan []byte) {
	r.mu.Lock()
	delete(r.clients, ch)
	close(ch)
	r.mu.Unlock()
	r.clientCount.Add(-1)
}

func (r *Hub) Broadcast(data []byte) {
	frameNo := r.frameNo.Add(1)
	r.bytesRecv.Add(uint64(len(data)))

	now := time.Now()
	r.lastFrameAt.Store(&now)

	frameData := make([]byte, 8+len(data))
	binary.BigEndian.PutUint64(frameData[:8], frameNo)
	copy(frameData[8:], data)

	r.mu.RLock()
	defer r.mu.RUnlock()
	for ch := range r.clients {
		select {
		case <-ch:
		default:
		}
		select {
		case ch <- frameData:
		default:
		}
	}
}

type Status struct {
	Streams []*StreamStats `json:"streams"`
}

type StreamStats struct {
	Name        string        `json:"name"`
	Ready       bool          `json:"ready"`
	Codec       string        `json:"codec"`
	Width       int           `json:"width"`
	Height      int           `json:"height"`
	Framerate   float64       `json:"framerate"`
	FrameNo     uint64        `json:"frameNo"`
	ClientCount int32         `json:"clientCount"`
	BytesRecv   uint64        `json:"bytesRecv"`
	Bitrate     float64       `json:"bitrateKbps"`
	Uptime      time.Duration `json:"uptime"`
}

func (r *Hub) GetStats(name string) StreamStats {
	bytes := r.bytesRecv.Load()
	frameNo := r.frameNo.Load()
	readyAt := r.readyAt.Load()
	var elapsed time.Duration
	var bitrate float64
	if readyAt != nil {
		elapsed = time.Since(*readyAt)
		if elapsed > 0 {
			bitrate = float64(bytes) * 8 / elapsed.Seconds() / 1000
		}
	}
	r.mu.RLock()
	codec := r.codec
	width := r.width
	height := r.height
	frameRate := r.frameRate
	r.mu.RUnlock()
	return StreamStats{
		Name:        name,
		Ready:       r.ready.Load(),
		Codec:       codec,
		Width:       width,
		Height:      height,
		Framerate:   frameRate,
		FrameNo:     frameNo,
		ClientCount: r.clientCount.Load(),
		BytesRecv:   bytes,
		Bitrate:     bitrate,
		Uptime:      elapsed,
	}
}

func (r *Hub) GetStatus() Status {
	stats := r.GetStats("")
	return Status{
		Streams: []*StreamStats{&stats},
	}
}
