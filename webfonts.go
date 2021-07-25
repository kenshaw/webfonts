// Package webfonts provides client for the google webfonts helper and a way to
// easily retrieve and serve webfonts.
package webfonts

import (
	"bytes"
	"context"
	"crypto/md5"
	_ "embed"
	"fmt"
	"html/template"
	"io"
	"io/ioutil"
	"net/url"
	"path"
	"regexp"
	"sort"
	"strings"

	"github.com/vanng822/css"
	gfonts "google.golang.org/api/webfonts/v1"
)

// Available retrieves all available webfonts.
func Available(ctx context.Context, opts ...ClientOption) ([]*gfonts.Webfont, error) {
	return NewClient(opts...).Available(ctx)
}

// FontFaces retrieves the font faces for the specified family.
func FontFaces(ctx context.Context, family string, opts ...ClientOption) ([]FontFace, error) {
	return NewClient(opts...).FontFaces(ctx, family)
}

// AllFontFaces retrieves all font faces for the specified family by using
// multiple user agents.
func AllFontFaces(ctx context.Context, family string, opts ...ClientOption) ([]FontFace, error) {
	return NewClient(opts...).AllFontFaces(ctx, family)
}

// FontFace describes a font face.
type FontFace struct {
	Subset string   `json:"subset,omitempty"`
	Family string   `json:"font-family,omitempty"`
	Style  string   `json:"font-style,omitempty"`
	Weight string   `json:"font-weight,omitempty"`
	Src    string   `json:"src,omitempty"`
	Format string   `json:"format,omitempty"`
	Range  []string `json:"unicode-range,omitempty"`
}

// FontFacesFromStylesheetReader parses stylesheet from the passed reader,
// returning any encountered font face.
func FontFacesFromStylesheetReader(r io.Reader) ([]FontFace, error) {
	// load
	buf, err := ioutil.ReadAll(r)
	if err != nil {
		return nil, err
	}
	// subsets
	s := string(buf)
	subsets := subsetRE.FindAllStringSubmatch(s, -1)
	// parse
	rules := css.Parse(s).GetCSSRuleList()
	var fonts []FontFace
	for i, rule := range rules {
		if rule.Type != css.FONT_FACE_RULE {
			continue
		}
		// build font
		var font FontFace
		if subsets != nil && i < len(subsets) {
			font.Subset = subsets[i][1]
		}
		for _, style := range rule.Style.Styles {
			switch style.Property {
			case "font-family":
				v := style.Value.Text()
				font.Family = v[1 : len(v)-1]
			case "font-style":
				font.Style = style.Value.Text()
			case "font-weight":
				font.Weight = style.Value.Text()
			case "src":
				var err error
				if font.Src, font.Format, err = parseSrcAndFormat(style.Value.Text()); err != nil {
					return nil, err
				}
			case "unicode-range":
				font.Range = strings.Split(style.Value.Text(), ",")
				for i := 0; i < len(font.Range); i++ {
					font.Range[i] = strings.TrimSpace(font.Range[i])
				}
			default:
				return nil, fmt.Errorf("unknown @font-face property %q", style.Property)
			}
		}
		fonts = append(fonts, font)
	}
	return fonts, nil
}

// subsetRE matches subset descriptions in the stylesheet.
var subsetRE = regexp.MustCompile(`(?m)^/\*\s+([a-z0-9-]+)\s+\*/$`)

// parseSrcAndFormat parses the url and format in a stylesheet src property.
func parseSrcAndFormat(src string) (string, string, error) {
	// extract and parse url
	m := srcRE.FindAllStringSubmatch(src, -1)
	if len(m) != 1 {
		return "", "", fmt.Errorf("invalid src %q", src)
	}
	u, err := url.Parse(m[0][1])
	if err != nil {
		return "", "", fmt.Errorf("invalid src url %q", m[0][1])
	}
	// determine file extension
	fileExt := strings.ToLower(strings.TrimPrefix(path.Ext(path.Base(u.Path)), "."))
	if fileExt == "" {
		fileExt = m[0][2]
	}
	return m[0][1], fileExt, nil
}

// srcRE matches src.
var srcRE = regexp.MustCompile(`(?m)^url\(([^\)]+)\)(?:\s+format\('([^']+)'\))?$`)

// Route wraps information about a route. Used for callbacks passed to
// BuildRoutes.
type Route struct {
	Path string
	URL  string
}

// BuildRoutes builds routes for the provided font faces.
func BuildRoutes(prefix string, fonts []FontFace, h func(string, []byte, []Route) error) error {
	families := make(map[string]map[string]map[string][]FontFace)
	// arrange by family, style, weight
	for _, font := range fonts {
		if _, ok := families[font.Family]; !ok {
			families[font.Family] = make(map[string]map[string][]FontFace)
		}
		if _, ok := families[font.Family][font.Style]; !ok {
			families[font.Family][font.Style] = make(map[string][]FontFace)
		}
		families[font.Family][font.Style][font.Weight] = append(families[font.Family][font.Style][font.Weight], font)
	}
	// sort families
	var familyKeys []string
	for k := range families {
		familyKeys = append(familyKeys, k)
	}
	sort.Strings(familyKeys)
	// iterate over families
	for _, family := range familyKeys {
		// sort styles
		var styleKeys []string
		for k := range families[family] {
			styleKeys = append(styleKeys, k)
		}
		sort.Strings(styleKeys)
		buf := new(bytes.Buffer)
		var routes []Route
		// iterate over styles
		for _, style := range styleKeys {
			// sort weights
			var weightKeys []string
			for k := range families[family][style] {
				weightKeys = append(weightKeys, k)
			}
			sort.Strings(weightKeys)
			// iterate over weights
			for _, weight := range weightKeys {
				// process
				r, err := process(buf, prefix, family, style, weight, families)
				if err != nil {
					return err
				}
				routes = append(routes, r...)
			}
		}
		// send to handler
		if err := h(family, buf.Bytes(), routes); err != nil {
			return err
		}
	}
	return nil
}

// process generates the stylesheet and routes for the font family, style, and
// weight combination.
func process(w io.Writer, prefix, family, style, weight string, families map[string]map[string]map[string][]FontFace) ([]Route, error) {
	// build file routes and paths
	var routes []Route
	paths := make(map[string]string)
	for _, font := range families[family][style][weight] {
		if _, ok := paths[font.Format]; !ok {
			hash := fmt.Sprintf("%x", md5.Sum([]byte(font.Src)))[:7]
			path := hash + "." + font.Format
			paths[font.Format] = prefix + path
			routes = append(routes, Route{
				Path: path,
				URL:  font.Src,
			})
		}
	}
	// execute
	if err := tpl.Execute(w, map[string]interface{}{
		"family": family,
		"style":  style,
		"weight": weight,
		"paths":  paths,
	}); err != nil {
		return nil, err
	}
	return routes, nil
}

// tpl is the stylesheet template.
var tpl = template.Must(template.New("stylesheet.css").Parse(string(stylesheetCSS)))

// stylesheetCSS is the embedded stylesheet css.
//
//go:embed stylesheet.css
var stylesheetCSS []byte
