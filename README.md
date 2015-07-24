# Proxy with Cache

I've seen so many Proxy Caches out there but so many don't meet my needs; maybe it's the way in which they tend to work. I wanted to create a near CDN quality Proxy Cache with a simple and flexible API.

## Basic Usage

		package main

		import (
			"net/http"
			"os"

			proxy "github.com/KellyLSB/go.proxy"
		)

		func main() {
			req, err := http.NewRequest("GET", "http://example.com", nil)

			response, err := proxy.NewProxy().
				UseCacheNameSyle(proxy.CacheNameSHA1).RoundTrip(req)

			// Haha!
			response.Write(os.Stdout)
		}

**A `ServeHTTP()` method also is available for consumption**

## Proxy Features

As a proxy I wanted to ensure the highest quality of service. As a result you will find caching options, header injections, `RoundTrip()`, `ServeHTTP()`, `Location` header redirects and `GunzipBodyTo()` helers on the Response; among other features.

## Cache Features

As a Cache which could be used transparently, say on a CDN. We need to be respectful of the HTTP Headers that are implemented. Many Caches do not honor all the headers; I'm sure I've missing some too. If you think of one not listed here please open an issue for me (and/or submit a Pull Request); it would be much appreciated.

**Honored Cache Specific Headers:**
- Cache-Control: no-cache, max-age, s-maxage, private
- Date (with max-age, s-maxage)
- Pragma: no-cache, (#todo no-store)
- Expires
- Last-Modified (with HTTP/1.1 HEAD request)
- ETag
- Content-MD5
- Content-SHA1

**Known Not Yet Implemented Cache Specific Headers:**
- Vary

**Cache Store Options**
- Naming by SHA1 Sum of `*http.Request`
- Naming by Resourceful URI (Host + "/" + Path)

## Why, specifically did you write this?

Mostly for fun, but also because I am working on a implementation of Apt-Get with the basic functionality of DPKG as a statically linked binary. A operational Apt-Get alternative that also provides P2P package sharing. The busybox of package management; or something.

The truth is I recently went through some trauma; I'm bored and am having fun.

## There are No Docs!

Patience my friend; I will write them. As I said this is intended to be a fun relaxing project. That means I don't really care about adhering strict work flows.

## There are No Tests!

Again patience. Though, I'm not really big on TDD; I respect them in CI environment. This is not going to be on a CI server at the moment.

I have another idea around this process anyway; we will see.

## Are you daft?

Yes

## License

The MIT License (MIT)

Copyright (c) 2015 Kelly Lauren-Summer Becker-Neuding

Permission is hereby granted, free of charge, to any person obtaining a copy
of this software and associated documentation files (the "Software"), to deal
in the Software without restriction, including without limitation the rights
to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
copies of the Software, and to permit persons to whom the Software is
furnished to do so, subject to the following conditions:

The above copyright notice and this permission notice shall be included in
all copies or substantial portions of the Software.

THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN
THE SOFTWARE.
