// _example/example.go
package main

import (
	"bytes"
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"path"
	"sort"
	"text/template"
	"time"

	"github.com/kenshaw/diskcache"
	"github.com/kenshaw/httplog"
	"github.com/kenshaw/webfonts"
)

func main() {
	verbose := flag.Bool("v", false, "verbose")
	addr := flag.String("l", ":9090", "listen")
	key := flag.String("key", "", "webfonts key")
	text := flag.String("text", "Lorem Ipsum Dolor", "text")
	prefix := flag.String("prefix", "/_/", "prefix")
	flag.Parse()
	if err := run(context.Background(), *verbose, *addr, *key, *text, *prefix, flag.Args()...); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}

func run(ctx context.Context, verbose bool, addr, key, text, prefix string, allowed ...string) error {
	if key == "" {
		return errors.New("must provide -key\n\n  see: https://developers.google.com/fonts/docs/developer_api\n")
	}
	// create cache transport
	cache, err := buildCache(verbose)
	if err != nil {
		return err
	}
	// retrieve all available families
	families, err := webfonts.Available(ctx, webfonts.WithTransport(cache), webfonts.WithKey(key))
	if err != nil {
		return err
	}
	fmt.Printf("families: %d\n", len(families))
	sort.Slice(families, func(i, j int) bool {
		return families[i].Family < families[j].Family
	})
	// retrieve fonts
	var fonts []webfonts.Font
	cl := webfonts.NewClient(webfonts.WithTransport(cache))
	for _, font := range families {
		if len(allowed) != 0 && !contains(allowed, font.Family) {
			continue
		}
		fmt.Printf("retrieving: %s", font.Family)
		face, err := cl.WOFF2(ctx, font.Family, webfonts.WithDisplay("block"), webfonts.WithText(text))
		if err != nil {
			return err
		}
		if face.Src == "" {
			fmt.Printf(" --skipped--\n")
			continue
		}
		fmt.Printf(" %s\n", face.Src)
		fonts = append(fonts, face)
	}
	// create server and build routes
	s := newServer()
	if err := webfonts.BuildRoutes(prefix, fonts, s.build(ctx, prefix, fonts, cache)); err != nil {
		return err
	}
	// index
	buf := new(bytes.Buffer)
	if err := tpl.Execute(buf, map[string]interface{}{
		"text":   text,
		"prefix": prefix,
		"fonts":  fonts,
	}); err != nil {
		return err
	}
	s.index = buf.Bytes()
	// listen and serve
	fmt.Printf("listening: %v\n", addr)
	l, err := (&net.ListenConfig{}).Listen(ctx, "tcp", addr)
	if err != nil {
		return err
	}
	defer l.Close()
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
	index []byte
}

// newServer creates the server.
func newServer() *Server {
	s := &Server{
		ServeMux: http.NewServeMux(),
	}
	s.HandleFunc("/", s.indexHandler)
	return s
}

func (s *Server) indexHandler(res http.ResponseWriter, req *http.Request) {
	_, _ = res.Write(s.index)
}

// build builds server routes.
func (s *Server) build(ctx context.Context, prefix string, fonts []webfonts.Font, transport http.RoundTripper) func(string, []byte, []webfonts.Route) error {
	return func(family string, buf []byte, routes []webfonts.Route) error {
		// routes
		for _, route := range routes {
			// retrieve
			contentType, buf, err := get(ctx, route.URL, transport)
			if err != nil {
				return err
			}
			s.HandleFunc(path.Join(prefix, url.PathEscape(route.Path)), func(res http.ResponseWriter, req *http.Request) {
				res.Header().Set("Content-Type", contentType)
				_, _ = res.Write(buf)
			})
		}
		// stylesheet
		stylesheetPath := path.Join(prefix, url.PathEscape(family+".css"))
		s.HandleFunc(stylesheetPath, func(res http.ResponseWriter, req *http.Request) {
			res.Header().Set("Content-Type", "text/css")
			_, _ = res.Write(buf)
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
	buf, err := io.ReadAll(res.Body)
	if err != nil {
		return "", nil, err
	}
	return res.Header.Get("Content-Type"), buf, nil
}

func contains(v []string, s string) bool {
	for _, z := range v {
		if s == z {
			return true
		}
	}
	return false
}

var tpl = template.Must(template.New("index.html").Funcs(template.FuncMap{
	"inc": func(i int) int {
		return i + 1
	},
	"join": func(s ...string) string {
		return path.Join(s...)
	},
}).Parse(indexHtml))

const indexHtml = `{{ $text := .text }}{{ $prefix := .prefix }}<html>
<head>{{ range $i, $font := .fonts }}
  <link rel="stylesheet" href="{{ join $prefix $font.Family }}.css">
{{- end }}
</head>
<body>
<div>{{ range $i, $font := .fonts }}
  <p style="font-family: {{ $font.Family }}; font-size: 24pt;">{{ inc $i }}. {{ $font.Family }}:<br/>{{ $text }}</p>
{{- end }}
</div>
</body>
</html>`
