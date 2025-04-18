package webfonts

import (
	"bytes"
	"crypto/md5"
	_ "embed"
	"fmt"
	"io"
	"sort"
	"strings"
	"text/template"
)

// BuildRoutes builds routes for the provided font faces.
func BuildRoutes(prefix string, fonts []Font, h func(string, []byte, []Route) error) error {
	families := make(map[string]map[string]map[string][]Font)
	// arrange by family, style, weight
	for _, font := range fonts {
		if _, ok := families[font.Family]; !ok {
			families[font.Family] = make(map[string]map[string][]Font)
		}
		if _, ok := families[font.Family][font.Style]; !ok {
			families[font.Family][font.Style] = make(map[string][]Font)
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

// Route wraps information about a route. Used for callbacks passed to
// BuildRoutes.
type Route struct {
	Path string
	URL  string
}

// process generates the stylesheet and routes for the font family, style, and
// weight combination found in families.
func process(w io.Writer, prefix, family, style, weight string, families map[string]map[string]map[string][]Font) ([]Route, error) {
	// build file routes and paths
	var routes []Route
	var display string
	var stretch string
	paths := make(map[string]string)
	for _, font := range families[family][style][weight] {
		if _, ok := paths[font.Format]; !ok {
			hash := fmt.Sprintf("%x", md5.Sum([]byte(font.Src)))[:7]
			path := hash + "." + font.Format
			paths[font.Format] = prefix + path
			if font.Display != "" && display == "" {
				display = font.Display
			}
			if font.Stretch != "" && stretch == "" {
				stretch = font.Stretch
			}
			routes = append(routes, Route{
				Path: path,
				URL:  font.Src,
			})
		}
	}
	// execute
	if err := tpl.Execute(w, map[string]any{
		"family":  family,
		"style":   style,
		"weight":  weight,
		"display": display,
		"stretch": stretch,
		"paths":   paths,
	}); err != nil {
		return nil, err
	}
	return routes, nil
}

// tpl is the stylesheet template.
var tpl = template.Must(template.New("stylesheet.css.tpl").Funcs(template.FuncMap{
	"src": func(indent string, m map[string]string) string {
		var prefix string
		if path, ok := m["eot"]; ok {
			prefix = fmt.Sprintf("url('%s');\n%ssrc: url('%s?#iefix') format('embedded-opentype'), ", path, indent, path)
		}
		paths := []string{"local('')"}
		for _, s := range []string{"woff2", "woff", "ttf", "svg"} {
			if path, ok := m[s]; ok {
				paths = append(paths, fmt.Sprintf("url('%s') format('%s')", path, s))
			}
		}
		return prefix + strings.Join(paths, ", ")
	},
}).Parse(string(stylesheetCSSTpl)))

// stylesheetCSSTpl is the embedded stylesheet css.
//
//go:embed stylesheet.css.tpl
var stylesheetCSSTpl []byte
