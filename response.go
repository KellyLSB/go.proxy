package proxy

import (
	"bytes"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type Response struct {
	err       error
	proxied   *http.Response
	cacheName string
	cached    bool
}

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

func (response *Response) RemoveHeaders(headers ...string) *Response {
	for _, header := range headers {
		response.proxied.Header.Del(header)
	}

	return response
}

func (response *Response) SetCacheName(name string) *Response {
	response.cacheName = name
	return response
}

func (response *Response) MarkAsCached() *Response {
	response.cached = true
	return response
}

func (response *Response) GetHeaderValues(header string) []string {
	return response.proxied.Header[header]
}

func (response *Response) GetHeader(header string) string {
	return response.proxied.Header.Get(header)
}

func (response *Response) GetHeaders() http.Header {
	return response.proxied.Header
}

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

	// Ensure the CacheDir exists.
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
			ioWriters = append(ioWriters, io.Writer(writer))
		case io.Writer:
			ioWriters = append(ioWriters, writer)
		}
	}

	// Write to everything at once; since the response
	// is a ReadCloser we only get one shot. xD
	response.proxied.Write(io.MultiWriter(ioWriters...))
}
