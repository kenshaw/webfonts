package webfonts

import (
	"context"
	"crypto/md5"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/chromedp/omahaproxy"
	"github.com/kenshaw/diskcache"
	"github.com/kenshaw/httplog"
	"golang.org/x/oauth2"
	gtransport "google.golang.org/api/googleapi/transport"
	"google.golang.org/api/option"
	gfonts "google.golang.org/api/webfonts/v1"
)

// DefaultTransport is the default http transport.
var DefaultTransport = http.DefaultTransport

// Client is a webfonts client.
type Client struct {
	userAgent   string
	transport   http.RoundTripper
	appCacheDir string
	key         string
	source      oauth2.TokenSource
	opts        []option.ClientOption
	cl          *http.Client
	svc         *gfonts.Service
	once        sync.Once
}

// NewClient creates a new webfonts client.
func NewClient(opts ...ClientOption) *Client {
	cl := &Client{
		transport: DefaultTransport,
	}
	for _, o := range opts {
		o(cl)
	}
	return cl
}

// init initializes the client.
func (cl *Client) init(ctx context.Context) error {
	var err error
	cl.once.Do(func() {
		if err = cl.buildTransport(ctx); err != nil {
			return
		}
		if err = cl.buildUserAgent(ctx); err != nil {
			return
		}
		if err = cl.buildService(ctx); err != nil {
			return
		}
	})
	return err
}

// buildTransport builds the http client used for retrievals.
func (cl *Client) buildTransport(ctx context.Context) error {
	if cl.appCacheDir != "" {
		var err error
		cl.transport, err = diskcache.New(
			diskcache.WithTransport(cl.transport),
			diskcache.WithAppCacheDir(cl.appCacheDir),
			diskcache.WithTTL(24*time.Hour),
			diskcache.WithHeaderWhitelist("Date", "Set-Cookie", "Content-Type", "Location"),
			diskcache.WithErrorTruncator(),
			diskcache.WithGzipCompression(),
		)
		if err != nil {
			return err
		}
	}
	cl.cl = &http.Client{
		Transport: cl.transport,
	}
	return nil
}

// buildUserAgent builds the user agent.
func (cl *Client) buildUserAgent(ctx context.Context) error {
	if cl.userAgent != "" {
		return nil
	}
	// retrieve latest chrome version
	ver, err := omahaproxy.New(
		omahaproxy.WithTransport(cl.transport),
	).Latest(ctx, "linux", "stable")
	if err != nil {
		return err
	}
	// build user agent
	cl.userAgent = fmt.Sprintf("Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/%s Safari/537.36", ver.Version)
	return nil
}

// buildService builds the webfonts service.
func (cl *Client) buildService(ctx context.Context) error {
	if cl.svc != nil {
		return nil
	}
	// build transport
	transport := cl.transport
	switch {
	case cl.source != nil:
		transport = &oauth2.Transport{
			Source: cl.source,
			Base:   transport,
		}
	case cl.key != "":
		transport = &gtransport.APIKey{
			Key:       cl.key,
			Transport: transport,
		}
	}
	// build service
	opts := append(cl.opts, option.WithHTTPClient(&http.Client{
		Transport: transport,
	}))
	var err error
	cl.svc, err = gfonts.NewService(ctx, opts...)
	return err
}

// Available retrieves all webfonts.
func (cl *Client) Available(ctx context.Context) ([]*gfonts.Webfont, error) {
	// init
	if err := cl.init(ctx); err != nil {
		return nil, err
	}
	if cl.svc == nil {
		return nil, errors.New("service uninitialized")
	}
	// retrieve
	res, err := cl.svc.Webfonts.List().Do()
	if err != nil {
		return nil, err
	}
	return res.Items, nil
}

// get retrieves a stylesheet from the url using the specified user agent,
// return any parsed font faces contained in the stylesheet.
func (cl *Client) get(ctx context.Context, urlstr, userAgent string) ([]FontFace, error) {
	// build request
	urlstr += "&_=" + fmt.Sprintf("%x", md5.Sum([]byte(userAgent)))[:5]
	req, err := http.NewRequest("GET", urlstr, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", userAgent)
	// execute
	res, err := cl.cl.Do(req.WithContext(ctx))
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	// check status
	if res.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("status code %d != 200", res.StatusCode)
	}
	// parse
	return FontFacesFromStylesheetReader(res.Body)
}

// FontFaces retrieves the font faces for the specified family.
func (cl *Client) FontFaces(ctx context.Context, family string, opts ...QueryOption) ([]FontFace, error) {
	// initialize
	if err := cl.init(ctx); err != nil {
		return nil, err
	}
	if cl.cl == nil {
		return nil, errors.New("client uninitialized")
	}
	// build query
	q := NewQuery(family, opts...)
	userAgent := cl.userAgent
	if q.UserAgent != "" {
		userAgent = q.UserAgent
	}
	// retrieve
	return cl.get(ctx, q.String(), userAgent)
}

// AllFontFaces retrieves all font faces for the specified family by using
// multiple user agents.
func (cl *Client) AllFontFaces(ctx context.Context, family string, opts ...QueryOption) ([]FontFace, error) {
	// initialize
	if err := cl.init(ctx); err != nil {
		return nil, err
	}
	if cl.cl == nil {
		return nil, errors.New("client uninitialized")
	}
	// build query
	q := NewQuery(family, opts...)
	var ff []FontFace
	for _, typ := range []string{"eot", "svg", "ttf", "woff2", "woff"} {
		fonts, err := cl.get(ctx, q.String(), userAgents[typ])
		if err != nil {
			return nil, err
		}
		ff = append(ff, fonts...)
	}
	return ff, nil
}

// Query wraps a font request.
type Query struct {
	Family    string
	UserAgent string
	Variants  []string
	Subsets   []string
	Styles    []string
	Effects   []string
}

// NewQuery builds a new webfont query.
func NewQuery(family string, opts ...QueryOption) *Query {
	q := &Query{
		Family: family,
	}
	for _, o := range opts {
		o(q)
	}
	return q
}

// Values returns the url values for the request.
func (q *Query) Values() url.Values {
	family := q.Family
	if q.Variants != nil {
		family += ":" + strings.Join(q.Variants, ",")
	}
	v := url.Values{
		"family": []string{family},
	}
	if q.Subsets != nil {
		v["subset"] = []string{strings.Join(q.Subsets, ",")}
	}
	if q.Effects != nil {
		v["effect"] = []string{strings.Join(q.Effects, "|")}
	}
	return v
}

// String satisfies the fmt.Stringer interface.
//
// Returns the URL for the request.
func (q *Query) String() string {
	return "https://fonts.googleapis.com/css?" + q.Values().Encode()
}

// ClientOption is a webfonts client option.
type ClientOption func(*Client)

// WithTransport is a webfonts client option to set the http transport.
func WithTransport(transport http.RoundTripper) ClientOption {
	return func(cl *Client) {
		cl.transport = transport
	}
}

// WithLogf is a webfonts client option to set a log handler for http requests and
// responses.
func WithLogf(logf interface{}, opts ...httplog.Option) ClientOption {
	return func(cl *Client) {
		cl.transport = httplog.NewPrefixedRoundTripLogger(cl.transport, logf, opts...)
	}
}

// WithAppCacheDir is a webfonts client option to set the app cache dir.
func WithAppCacheDir(appCacheDir string) ClientOption {
	return func(cl *Client) {
		cl.appCacheDir = appCacheDir
	}
}

// WithClientOption is a webfonts client option to set underlying client
// options.
func WithClientOption(opt option.ClientOption) ClientOption {
	return func(cl *Client) {
		cl.opts = append(cl.opts, opt)
	}
}

// WithKey is a webfonts client option to set the webfonts api key.
func WithKey(key string) ClientOption {
	return func(cl *Client) {
		cl.key = key
	}
}

// WithTokenSource is a webfonts client option to set the token source.
func WithTokenSource(source oauth2.TokenSource) ClientOption {
	return func(cl *Client) {
		cl.source = source
	}
}

// QueryOption is a webfonts query option.
type QueryOption func(*Query)

// WithUserAgent is a query option to set the user agent.
func WithUserAgent(userAgent string) QueryOption {
	return func(q *Query) {
		q.UserAgent = userAgent
	}
}

// WithVariants is a query option to set variants.
func WithVariants(variants ...string) QueryOption {
	return func(q *Query) {
		q.Variants = variants
	}
}

// WithSubsets is a query option to set subsets.
func WithSubsets(subsets ...string) QueryOption {
	return func(q *Query) {
		q.Subsets = subsets
	}
}

// WithStyles is a query option to set styles.
func WithStyles(styles ...string) QueryOption {
	return func(q *Query) {
		q.Styles = styles
	}
}

// WithEffects is a query option to set effects.
func WithEffects(effects ...string) QueryOption {
	return func(q *Query) {
		q.Effects = effects
	}
}

// userAgents are user agents that force the service to return paths for
// different file types.
var userAgents = map[string]string{
	"eot":   "Mozilla/4.0 (compatible; MSIE 8.0; Windows NT 6.1; Trident/4.0)",
	"svg":   "Mozilla/4.0 (iPad; CPU OS 4_0_1 like Mac OS X) AppleWebKit/534.46 (KHTML, like Gecko) Version/4.1 Mobile/9A405 Safari/7534.48.3",
	"ttf":   "Mozilla/5.0 (Unknown; Linux x86_64) AppleWebKit/538.1 (KHTML, like Gecko) Safari/538.1 Daum/4.1",
	"woff2": "Mozilla/5.0 (Windows NT 6.1; WOW64; rv:40.0) Gecko/20100101 Firefox/40.0",
	"woff":  "Mozilla/5.0 (Windows NT 6.1; WOW64; rv:27.0) Gecko/20100101 Firefox/27.0",
}
