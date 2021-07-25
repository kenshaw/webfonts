// _example/example.go
package main

import (
	"context"
	"flag"
	"fmt"
	"io/ioutil"
	"net"
	"net/http"
	"os"
	"path"
	"time"

	"github.com/kenshaw/diskcache"
	"github.com/kenshaw/httplog"
	"github.com/kenshaw/webfonts"
)

func main() {
	verbose := flag.Bool("v", false, "verbose")
	addr := flag.String("l", ":9090", "listen")
	key := flag.String("k", "", "webfonts key")
	prefix := flag.String("prefix", "/_/", "prefix")
	flag.Parse()
	if err := run(context.Background(), *verbose, *addr, *key, *prefix); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}

func run(ctx context.Context, verbose bool, addr, key, prefix string) error {
	// create cache transport
	cache, err := buildCache(verbose)
	if err != nil {
		return err
	}
	// retrieve all font faces
	fonts, err := webfonts.AllFontFaces(
		ctx,
		"Roboto",
		webfonts.WithTransport(cache),
		webfonts.WithKey(key),
	)
	if err != nil {
		return err
	}
	// create server and build routes
	s := newServer(prefix)
	if err := webfonts.BuildRoutes(prefix, fonts, s.build(ctx, cache)); err != nil {
		return err
	}
	// listen and serve
	l, err := (&net.ListenConfig{}).Listen(ctx, "tcp", addr)
	if err != nil {
		return err
	}
	return http.Serve(l, s)
}

// buildCache creates a disk cache transport.
func buildCache(verbose bool) (*diskcache.Cache, error) {
	opts := []diskcache.Option{
		diskcache.WithAppCacheDir("webfonts"),
		diskcache.WithTTL(24 * time.Hour),
		diskcache.WithHeaderWhitelist("Date", "Set-Cookie", "Content-Type", "Location"),
		diskcache.WithErrorTruncator(),
		diskcache.WithGzipCompression(),
	}
	if verbose {
		opts = append(opts, diskcache.WithTransport(
			httplog.NewPrefixedRoundTripLogger(
				http.DefaultTransport,
				fmt.Printf,
				httplog.WithReqResBody(false, false),
			),
		))
	}
	return diskcache.New(opts...)
}

type Server struct {
	*http.ServeMux
	prefix string
}

// newServer creates the server.
func newServer(prefix string) *Server {
	return &Server{
		ServeMux: http.NewServeMux(),
		prefix:   prefix,
	}
}

// build builds server routes.
func (s *Server) build(ctx context.Context, transport http.RoundTripper) func(string, []byte, []webfonts.Route) error {
	return func(family string, buf []byte, routes []webfonts.Route) error {
		// routes
		for _, route := range routes {
			// retrieve
			contentType, buf, err := get(ctx, route.URL, transport)
			if err != nil {
				return err
			}
			s.HandleFunc(path.Join(s.prefix, route.Path), func(res http.ResponseWriter, req *http.Request) {
				res.Header().Set("Content-Type", contentType)
				_, _ = res.Write(buf)
			})
		}
		// stylesheet
		stylesheetPath := path.Join(s.prefix, family) + ".css"
		s.HandleFunc(stylesheetPath, func(res http.ResponseWriter, req *http.Request) {
			res.Header().Set("Content-Type", "text/css")
			_, _ = res.Write(buf)
		})
		// index
		index := []byte(fmt.Sprintf(indexTemplate, stylesheetPath, family))
		s.HandleFunc("/", func(res http.ResponseWriter, req *http.Request) {
			_, _ = res.Write(index)
		})
		return nil
	}
}

// get retrieves the url using the transport.
func get(ctx context.Context, urlstr string, transport http.RoundTripper) (string, []byte, error) {
	// request
	req, err := http.NewRequest("GET", urlstr, nil)
	if err != nil {
		return "", nil, err
	}
	cl := &http.Client{
		Transport: transport,
	}
	// execute
	res, err := cl.Do(req.WithContext(ctx))
	if err != nil {
		return "", nil, err
	}
	defer res.Body.Close()
	buf, err := ioutil.ReadAll(res.Body)
	if err != nil {
		return "", nil, err
	}
	return res.Header.Get("Content-Type"), buf, nil
}

const indexTemplate = `<html>
<head>
<link rel="stylesheet" href="%s">
</head>
<body>
<div style="font-family: %s">
lorem ipsum dolor
</div>
</body>
</html>`
