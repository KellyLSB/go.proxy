package proxy

import (
	"bufio"
	"bytes"
	"net/http"
	"path/filepath"
)

// CacheNameStyle is used
// to determine the available
// cache name structures
type CacheNameStyle int

const (
	// CacheNameSHA1 defines *http.Request Sum naming for cache.
	CacheNameSHA1 CacheNameStyle = iota
	// CacheNameURI defines *http.Request Host/URI naming for cache.
	CacheNameURI
)

// Proxy provides a gateway to HTTP caching.
type Proxy struct {
	cachePath      string
	cacheNameStyle CacheNameStyle
	transport      http.RoundTripper
}

// NewProxy creates a Proxy object that helps us manipulate
// HTTP requests and responses with a caching layer.
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

// UseCachePath sets the directory where we should save
// the cache responses to and were we should seek cached requests.
func (proxy *Proxy) UseCachePath(path string) *Proxy {
	proxy.cachePath = path
	return proxy
}

// UseCacheNameStyle sets the method of naming cache filenames.
//
// CacheNameSHA1: stores cached requests by the SHA1 Sum of the entire request.
// CacheNameURI: stores cached requests by the HOST/URI of the enture request.
func (proxy *Proxy) UseCacheNameStyle(style CacheNameStyle) *Proxy {
	proxy.cacheNameStyle = style
	return proxy
}

// ServeHTTP provides a Middleware for a HTTP Server
// that also implements tools such as a caching layer.
func (proxy *Proxy) ServeHTTP(
	writer http.ResponseWriter,
	httpRequest *http.Request,
) {
	proxy.prepareRequest(httpRequest).
		HTTP().Fetch().WriteTo(writer)
}

// RoundTrip provides a Middleware *http.Request that
// also provides tools such as a caching layer.
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

// Fetch takes a *http.Request and returns a *Response object
func (proxy *Proxy) Fetch(httpRequest *http.Request, _ ...error) *Response {
	return proxy.prepareRequest(httpRequest).HTTP().Fetch()
}

func (proxy *Proxy) prepareRequest(
	httpRequest *http.Request,
) *Request {
	log.Debug("Received Request")
	request := LoadRequest(httpRequest).
		SetTransport(proxy.transport).
		SetCachePath(proxy.cachePath).
		SetCacheNameStyle(proxy.cacheNameStyle)

	if proxy.cacheNameStyle == CacheNameURI {
		request.SetCacheName(filepath.Join(
			httpRequest.URL.Host,
			httpRequest.URL.Path,
		))
	}

	return request
}
