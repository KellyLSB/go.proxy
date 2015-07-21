package proxy

import (
	"bufio"
	"bytes"
	"net/http"
	"path/filepath"
)

type Proxy struct {
	URICacheName bool
	transport    http.RoundTripper
}

func NewProxy(transport ...http.RoundTripper) (proxy *Proxy) {
	proxy = new(Proxy)

	if len(transport) == 1 {
		log.Info("Created Proxy with Transport")
		proxy.transport = transport[0]
	} else {
		log.Info("Created Proxy")
	}

	return
}

func (proxy *Proxy) UseURICacheName() *Proxy {
	proxy.URICacheName = true
	return proxy
}

func (proxy *Proxy) ServeHTTP(
	writer http.ResponseWriter,
	httpRequest *http.Request,
) {
	proxy.prepareRequest(httpRequest).
		HTTP().Fetch().WriteTo(writer)
}

func (proxy *Proxy) RoundTrip(
	httpRequest *http.Request,
) (*http.Response, error) {
	var writer bytes.Buffer

	proxy.prepareRequest(httpRequest).
		HTTP().Fetch().WriteTo(&writer)

	response, err := http.ReadResponse(
		bufio.NewReader(&writer),
		httpRequest,
	)

	if err != nil {
		log.Error(err.Error())
	}

	return response, err
}

func (proxy *Proxy) prepareRequest(
	httpRequest *http.Request,
) *Request {
	log.Debug("Received Request")
	request := LoadRequest(httpRequest)

	if proxy.URICacheName {
		request.SetCacheName(filepath.Join(
			httpRequest.URL.Host,
			httpRequest.URL.Path,
		))
	}

	return request.SetTransport(proxy.transport)
}
