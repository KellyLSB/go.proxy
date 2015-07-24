package proxy

import (
	"bytes"
	"compress/gzip"
	"io"
	"io/ioutil"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Response is a tool for interacting
// with *http.Responses including a caching layer
type Response struct {
	cacheName string
	err       error
	proxied   *http.Response
	cached    bool
}

// LoadResponse loads a *http.Response and returns a *Response object
func LoadResponse(httpResponse *http.Response, err error) *Response {
	log.Debug("Loading Response")
	var buffer bytes.Buffer
	httpResponse.Header.Write(&buffer)
	log.Info("\n" + buffer.String())

	return (&Response{
		err:     err,
		proxied: httpResponse,
	}).RemoveHeaders(HopByHopHeaders...)
}

// RemoveHeaders deletes the named headers from the response headers.
func (response *Response) RemoveHeaders(headers ...string) *Response {
	for _, header := range headers {
		response.proxied.Header.Del(header)
	}

	return response
}

// SetCacheName sets the filename relative to the working directory
// that is used when saving / retrieving cached responses.
func (response *Response) SetCacheName(name string) *Response {
	response.cacheName = name
	return response
}

// MarkAsCached is used by the Request when loading
// a response from a cached file.
func (response *Response) MarkAsCached() *Response {
	response.cached = true
	return response
}

// GetHeaderValues returns an string slice
// of values of a named response header.
func (response *Response) GetHeaderValues(header string) []string {
	return response.proxied.Header[header]
}

// GetHeader returns the string value of a named response header.
func (response *Response) GetHeader(header string) string {
	return response.proxied.Header.Get(header)
}

// GetHeaders returns the http.Header object from the resonse.
func (response *Response) GetHeaders() http.Header {
	return response.proxied.Header
}

// HasHeaderValue performs if checking for
// header multi-values including assigned subvalues.
func (response *Response) HasHeaderValue(
	header string, has string,
) (string, bool) {
	has = strings.ToLower(has)

	for _, value := range response.GetHeaderValues(header) {
		keyval := append(strings.Split(value, "="), "")
		key, value := keyval[0], keyval[1]

		if strings.ToLower(key) == has {
			return value, true
		}
	}

	return "", false
}

// CacheExpired checks if the Response is cached and is expired.
// This is done by comparing information from a HEAD only response.
//
// Note: The HEAD only response is retrieved by
// a function passed from a Request object.
func (response *Response) CacheExpired(
	latestHeadFunc func() *Response,
) bool {
	log.Debug("Response Cached? (should be true): %v", response.cached)

	// If this Response is new;
	// then it's not expired.
	if !response.cached {
		return false
	}

	// Check Cache-Control: s-maxage and max-age
	responseDate := response.GetHeader("Date")
	if responseDate != "" {
		date, err := time.Parse(time.RFC1123, responseDate)

		log.Debug("Date: %v", date)
		if err != nil {
			log.Error(err.Error())
		}

		for _, maxage := range []string{"s-maxage", "max-age"} {
			if value, yes := response.HasHeaderValue(
				"Cache-Control", maxage,
			); yes {
				age, err := time.ParseDuration(value)

				log.Debug("Cache-Control: has %s of %v", maxage, age)
				if err != nil {
					log.Error(err.Error())
				}

				if err == nil && date.Add(age).Before(time.Now()) {
					return true
				}
			}
		}
	}

	// Check Expires header
	responseExpires := response.GetHeader("Expires")
	if responseExpires != "" {
		expires, err := time.Parse(time.RFC1123, responseExpires)

		log.Debug("Expires: on %v", expires)
		if err != nil {
			log.Error(err.Error())
		}

		if err == nil && expires.Before(time.Now()) {
			return true
		}
	}

	// The LatestHead should never be cached.
	// Assume expiration.
	latestHead := latestHeadFunc()
	if latestHead.cached {
		return true
	}

	// Check ETag and Content-MD5 headers
	for _, header := range []string{
		"ETag", "Content-MD5", "Content-SHA1",
	} {
		latestHeader := latestHead.GetHeader(header)
		responseHeader := response.GetHeader(header)

		if latestHeader != "" && responseHeader != "" {
			log.Debug("%s: ...", header)

			if latestHeader != responseHeader {
				return true
			}
		}
	}

	// Check Last-Modified header
	latestModified := latestHead.GetHeader("Last-Modified")
	responseModified := response.GetHeader("Last-Mofified")
	if latestModified != "" && responseModified != "" {
		lmod, err1 := time.Parse(time.RFC1123, latestModified)
		cmod, err2 := time.Parse(time.RFC1123, responseModified)

		log.Debug("Last-Modified: latest %v", lmod)
		if err1 != nil {
			log.Error(err1.Error())
		}

		log.Debug("Last-Modified: cached %v", cmod)
		if err2 != nil {
			log.Error(err2.Error())
		}

		if err1 == nil && err2 == nil && lmod.After(cmod) {
			return true
		}
	}

	return false
}

// WriteHeaderTo writes the response headers to the writers.
func (response *Response) WriteHeaderTo(writers ...io.Writer) {
	response.proxied.Header.Write(io.MultiWriter(writers...))
}

// WriteBodyTo writes the response body to the writers...
func (response *Response) WriteBodyTo(writers ...io.Writer) {
	reader := response.copyBody()
	if reader == nil {
		return
	}

	io.Copy(io.MultiWriter(writers...), reader)
}

// GunzipBodyTo using gunzip on the body then
// writes the uncompressed body to the writers.
func (response *Response) GunzipBodyTo(writers ...io.Writer) {
	reader := response.copyBody()
	if reader == nil {
		return
	}

	gzread, err := gzip.NewReader(reader)
	if err != nil {
		log.Error(err.Error())
		return
	}

	io.Copy(io.MultiWriter(writers...), gzread)
}

// WriteTo handles the caching process and writing the
// full response body (including) headers to the writers.
//
// Note: WriteTo also handle *http.ResponseWriter
func (response *Response) WriteTo(writers ...interface{}) {

	// Don't overwrite if the Reponse is from cache.
	if response.cached {
		goto WriteIt
	}

	// Cache-Control, do not cache if present
	for _, key := range []string{"private", "no-cache", "no-store"} {
		if _, yes := response.HasHeaderValue("Cache-Control", key); yes {
			log.Debug("Cache-Control: has %s", key)
			goto WriteIt
		}
	}

	// @TODO: Need to figure out where
	// Vary: Accept-Enacoding, User-Agent, etc... fit in.

	// Pragma, do not cache if present (backwards compatability)
	if _, yes := response.HasHeaderValue("Pragma", "no-cache"); yes {
		log.Debug("Pragma: has no-cache")
		goto WriteIt
	}

	// Ensure the cache file path exists.
	if os.MkdirAll(filepath.Dir(response.cacheName), 0700) != nil {
		log.Error("Cache Directory is not writeable!\n")
		goto WriteIt
	}

	// Ok, the checks passed; go ahead and cache the content.
	if file, err := os.Create(response.cacheName); err == nil {
		log.Debug("Preparing Cache Writer")
		writers = append(writers, file)
	}

WriteIt:
	response.writeTo(writers...)
}

func (response *Response) writeTo(writers ...interface{}) {
	var ioWriters []io.Writer

	// NO, NO, NO: I need io.Writers ;)
	for _, writer := range writers {
		switch writer := writer.(type) {
		case http.ResponseWriter:
			// Also http.ResponseWriter won't validate as an io.Writer
			CopyHeaders(writer.Header(), response.proxied.Header)
			writer.WriteHeader(response.proxied.StatusCode)
			response.WriteBodyTo(io.Writer(writer))
		case io.PipeWriter:
			response.WriteBodyTo(io.Writer(&writer))
		case io.Writer:
			ioWriters = append(ioWriters, writer)
		}
	}

	// Write to everything at once; since the response
	// is a ReadCloser we only get one shot. xD
	response.proxied.Write(io.MultiWriter(ioWriters...))
}

func (response *Response) copyBody() (reader io.ReadCloser) {
	var buf bytes.Buffer
	var err error

	_, err = buf.ReadFrom(response.proxied.Body)
	err = response.proxied.Body.Close()

	if err != nil {
		log.Error(err.Error())
	}

	response.proxied.Body = ioutil.NopCloser(&buf)
	return ioutil.NopCloser(bytes.NewReader(buf.Bytes()))
}
