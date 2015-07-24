package proxy

import (
	"bufio"
	"bytes"
	"crypto/sha1"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

// HopByHopHeaders are removed on load.
// http://www.w3.org/Protocols/rfc2616/rfc2616-sec13.html
var HopByHopHeaders = []string{
	"Connection",
	"Keep-Alive",
	"Proxy-Authenticate",
	"Proxy-Authorization",
	"Te", // canonicalized version of "TE"
	"Trailers",
	"Transfer-Encoding",
	"Upgrade",
}

type Request struct {
	cachePath      string
	cacheName      string
	cacheNameStyle CacheNameStyle

	transport     http.RoundTripper
	original      *http.Request
	proxied       *http.Request
	copiedHeaders bool
}

func LoadRequest(
	original *http.Request,
	hopByHopHeaders ...string,
) (request *Request) {

	// Prepare the Request
	request = &Request{
		original: original,
		proxied:  new(http.Request),
	}

	// Shallow copy the original Request to the Proxied one.
	log.Debug("Cloning Request")
	*request.proxied = *request.original
	request.proxied.Close = false

	// Remove modifying headers to ensure a persistent connection. Due
	// to the shallow copy; we need to copy the headers to allow this.
	log.Debug("Removing HopByHop Headers")
	request.RemoveHeaders(append(
		hopByHopHeaders,
		HopByHopHeaders...,
	)...)

	// Add the requests RemoteAddr to the X-Forwarded-For header chain.
	request.xForwardedFor()

	// Ensure request path has a string.
	if !strings.HasPrefix(request.proxied.URL.Path, "/") {
		request.proxied.URL.Path = "/" + request.proxied.URL.Path
	}

	return
}

func (request *Request) RemoveHeaders(headers ...string) *Request {
	for _, header := range headers {
		if request.proxied.Header.Get(header) != "" {
			request.copyHeaders()
			log.Debug("Removing Header: %s", header)
			request.proxied.Header.Del(header)
		}
	}

	return request
}

func (request *Request) SetTransport(
	transport http.RoundTripper,
) *Request {
	log.Debug("Setting Transport For Request")
	request.transport = transport
	return request
}

func (request *Request) Head() *Request {
	log.Debug("Preparing To Request Only Headers")
	request.proxied.Method = "HEAD"
	return request
}

func (request *Request) Get(forms ...map[string]interface{}) *Request {
	log.Debug("Preparing GET Request")
	request.proxied.Method = "GET"
	request.AddFormData(forms...)
	return request
}

func (request *Request) Put(forms ...map[string]interface{}) *Request {
	log.Debug("Preparing PUT Request")
	request.proxied.Method = "PUT"
	request.AddFormData(forms...)
	return request
}

func (request *Request) Post(forms ...map[string]interface{}) *Request {
	log.Debug("Preparing POST Request")
	request.proxied.Method = "POST"
	request.AddFormData(forms...)
	return request
}

func (request *Request) Delete(forms ...map[string]interface{}) *Request {
	log.Debug("Preparing DELETE Request")
	request.proxied.Method = "DELETE"
	request.AddFormData(forms...)
	return request
}

func (request *Request) OriginalMethod() *Request {
	log.Debug("Restoring To %s Request", request.original.Method)
	request.proxied.Method = request.original.Method
	return request
}

func (request *Request) AddFormData(
	forms ...map[string]interface{},
) *Request {
	log.Warning("No Handler for FormData Injection Yet")

	// for _, form := range forms {
	//
	// }

	return request
}

func (request *Request) AddFormField(key string, value string) *Request {
	log.Warning("No Handler for FormData Injection Yet")
	return request
}

func (request *Request) AddFormFile(key string, value io.Reader) *Request {
	log.Warning("No Handler for FormData Injection Yet")
	return request
}

func (request *Request) HTTP() *Request {
	log.Debug("Preparing HTTP Request")
	request.proxied.Proto = "HTTP/1.1"
	request.proxied.ProtoMajor = 1
	request.proxied.ProtoMinor = 1
	return request
}

func (request *Request) FTP() *Request {
	log.Debug("Preparing FTP Request")
	log.Warning("FTP Requests are not yet supported")
	request.proxied.Proto = "FTP"
	request.proxied.ProtoMajor = 0
	request.proxied.ProtoMinor = 0
	return request
}

func (request *Request) Fetch(transport ...http.RoundTripper) *Response {
	var httpResponse *http.Response
	var err error

	if request.proxied.Method != "GET" {
		goto RoundTrip
	}

FetchCache:
	if response := request.FetchCache(); response != nil {
		return response
	}

RoundTrip:
	log.Debug("Fetching Response From Request")
	var buffer bytes.Buffer
	request.proxied.Write(&buffer)
	log.Info("\n" + buffer.String())

	switch {
	case len(transport) == 1:
		httpResponse, err = transport[0].RoundTrip(request.proxied)
	case request.transport != nil:
		httpResponse, err = request.transport.RoundTrip(request.proxied)
	default:
		httpResponse, err = http.DefaultTransport.RoundTrip(request.proxied)
	}

	if err != nil {
		log.Error(err.Error())
		return nil
	}

	// Handle Location HTTP Header redirects
	log.Debug("Checking If Location Response Header Was Received")
	if location := httpResponse.Header.Get("Location"); location != "" {
		log.Debug("Handling Location Response Header Redirect")

		// If our request url is missing a host
		// (can happen if forwarding request as a proxy)
		if request.proxied.URL.Host == "" {
			request.proxied.URL.Host = request.proxied.Host
		}

		// Parse the url relatively; unless absolute location is returned.
		uri, err := request.proxied.URL.Parse(location)

		// Try not to knock the service down.
		if err != nil {
			log.Error("Could Not Handle Location Redirect")
			goto LoadResponse
		}

		// If we have a returned Host
		// apply the host to the request.
		if uri.Host != "" {
			request.proxied.Host = uri.Host
		}

		// Update the requst URL
		request.proxied.URL = uri

		// Try again
		log.Debug("Fetch The Redirected Request")
		goto FetchCache
	}

LoadResponse:
	return LoadResponse(httpResponse, err).
		SetCacheName(request.CacheName())
}

func (request *Request) FetchCache() *Response {
	log.Debug("Checking If Cached Response Exists")
	if file, err := os.Open(request.CacheName()); err == nil {

		log.Debug("Loading Cached Response")
		response := LoadResponse(http.ReadResponse(
			bufio.NewReader(file), request.proxied,
		)).SetCacheName(request.CacheName()).MarkAsCached()

		log.Debug("Checking For Cached Response Expiration")
		if !response.CacheExpired(func() *Response {
			response := request.Head().Fetch()
			request.OriginalMethod()
			return response
		}) {
			log.Debug("Serving Cached Response")
			return response
		}
	}

	log.Debug("No Valid Cached Response")
	return nil
}

func (request *Request) SetCachePath(path string) *Request {
	request.cachePath = path
	return request
}

func (request *Request) CachePath() string {
	if request.cachePath == "" {
		return "./cache"
	}

	return request.cachePath
}

func (request *Request) SetCacheNameStyle(style CacheNameStyle) *Request {
	request.cacheNameStyle = style
	return request
}

func (request *Request) SetCacheName(name string) *Request {
	request.cacheName = filepath.Join(request.CachePath(), name)
	return request
}

func (request *Request) CacheName() string {
	if request.cacheName != "" {
		return request.cacheName
	}

	switch request.cacheNameStyle {
	// case CacheNameSHA1:
	default:
		var buffer bytes.Buffer
		log.Debug("Generating SHA1 Hash Of Request")
		request.proxied.WriteProxy(&buffer)
		return filepath.Join(
			request.CachePath(),
			fmt.Sprintf("%x", sha1.Sum(
				buffer.Bytes()),
			),
		)
	}
}

func (request *Request) copyHeaders() {
	if !request.copiedHeaders {
		log.Debug("Copying Request Headers")
		request.proxied.Header = make(http.Header)

		CopyHeaders(
			request.original.Header,
			request.proxied.Header,
		)

		request.copiedHeaders = true
	}
}

func (request *Request) xForwardedFor() {
	if addr, _, e := net.SplitHostPort(
		request.proxied.RemoteAddr,
	); e == nil {
		log.Debug("Adding/Appending X-Forwarded-For Header")
		request.proxied.Header.Add("X-Forwarded-For", addr)
	}
}
