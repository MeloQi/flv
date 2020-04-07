package httpflv

import (
	"github.com/MeloQi/flv/muxer"
	stream "github.com/MeloQi/interfaces"
	"log"
	"net"
	"net/http"
)

type HttpFlVServer struct {
	addr   string
	m      stream.Modles
	listen net.Listener
	Stoped bool
}

func NewHttpFlvServer(addr string, modles stream.Modles) *HttpFlVServer {
	return &HttpFlVServer{addr: addr, m: modles, listen: nil, Stoped: false}
}

func (h *HttpFlVServer) Stop() {
	defer func() {
		recover()
	}()
	if h.Stoped {
		return
	}
	h.Stoped = true
	if h.listen != nil {
		h.listen.Close()
	}
}

//Start， 启动服务，阻塞，直到错误退出
func (h *HttpFlVServer) Start() error {
	listen, err := net.Listen("tcp", h.addr)
	if err != nil {
		return err
	}
	h.listen = listen
	m := http.NewServeMux()
	m.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		h.handleLive(w, r)
	})
	log.Println("httpFlv server start on ", listen.Addr().String())
	return http.Serve(listen, m)
}

func (h *HttpFlVServer) handleLive(w http.ResponseWriter, r *http.Request) {
	defer func() {
		if r := recover(); r != nil {
		}
	}()

	id, err := stream.StreamID(r.URL.String())
	if err != nil {
		http.Error(w, "invalid path", http.StatusBadRequest)
		return
	}
	media := h.m.GetStream(stream.MediaInfo{IsLive: true, StreamID: id, Vcodec: stream.H264Codec, Acodec: stream.AACCodec})
	if media == nil {
		http.Error(w, "Not Found", http.StatusNotFound)
		return
	}

	w.Header().Set("Access-Control-Allow-Origin", "*")
	mux := muxer.NewFLVMuxer(w, media.HasVideo, media.HasAudio)

	for !h.Stoped {
		pkg, ok := <-media.FrameChan
		if !ok {
			break
		}
		if pkg == nil {
			continue
		}
		if err := mux.Muxer(pkg); err != nil {
			media.Close()
			break
		}
	}
	media.Close()
	log.Print("handleLive exit")
}
