package client

import (
	"context"
	"crypto/tls"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/google/martian/v3"
	log "github.com/sirupsen/logrus"

	"github.com/DVKunion/SeaMoon/pkg/consts"
	"github.com/DVKunion/SeaMoon/pkg/utils"
)

func HttpController(ctx context.Context, sg *SigGroup) {
	c, cancel := context.WithCancel(ctx)
	defer cancel()
	for {
		select {
		case <-sg.HttpStartChannel:
			server, err := net.Listen("tcp", Config().Http.ListenAddr)
			if err != nil {
				log.Errorf(consts.HTTP_LISTEN_ERROR, err)
				return
			}
			var proxyAddr string
			for _, p := range Config().ProxyAddr {
				if strings.HasPrefix(p, "http://") || strings.HasPrefix(p, "https://") {
					proxyAddr = p
				} else if strings.HasPrefix(p, "http-proxy") {
					proxyAddr = "http://" + p
				}
			}
			if proxyAddr == "" {
				log.Error(consts.PROXY_ADDR_ERROR)
				break
			}
			sg.wg.Add(1)
			go func() {
				NewHttpClient(c, server, proxyAddr)
				sg.wg.Done()
			}()
		case <-sg.HttpStopChannel:
			log.Info(consts.HTTP_LISTEN_STOP)
			cancel()
		}
	}
}

func NewRequestModifier(pAddr string) *RequestModifier {
	return &RequestModifier{pAddr}
}

// RequestModifier impl martian.RequestModifier
type RequestModifier struct {
	pAddr string
}

// ModifyRequest is a RequestModifier logs all request url
func (r RequestModifier) ModifyRequest(req *http.Request) error {
	if req.Method == http.MethodConnect {
		return nil
	}
	req.Header.Set("SM-Host", utils.GetUrl(req))
	req.URL, _ = url.Parse(r.pAddr)
	req.Host = req.URL.Host
	return nil
}

func NewHttpClient(ctx context.Context, l net.Listener, pAddr string) {
	log.Infof(consts.HTTP_LISTEN_START, l.Addr())
	p := martian.NewProxy()
	defer p.Close()
	tr := &http.Transport{
		Dial: (&net.Dialer{
			Timeout: 10 * time.Second,
		}).Dial,
		TLSHandshakeTimeout:   10 * time.Second,
		ExpectContinueTimeout: time.Second,
		TLSClientConfig: &tls.Config{
			InsecureSkipVerify: true,
		},
	}
	p.SetRoundTripper(tr)
	logger := NewRequestModifier(pAddr)

	p.SetRequestModifier(logger)

	go func() {
		if err := p.Serve(l); err != nil {
			log.Error("server:", err)
		}
	}()

	<-ctx.Done()
}
