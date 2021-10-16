package webfonts

import (
	"fmt"
	"io"
	"io/ioutil"
	"net/url"
	"path"
	"regexp"
	"strings"

	"github.com/vanng822/css"
)

// Font describes a font face.
type Font struct {
	Subset  string   `json:"subset,omitempty"`
	Family  string   `json:"font-family,omitempty"`
	Style   string   `json:"font-style,omitempty"`
	Weight  string   `json:"font-weight,omitempty"`
	Display string   `json:"font-display,omitempty"`
	Stretch string   `json:"font-stretch,omitempty"`
	Src     string   `json:"src,omitempty"`
	Format  string   `json:"format,omitempty"`
	Range   []string `json:"unicode-range,omitempty"`
}

// FontsFromStylesheetReader parses stylesheet from the passed reader,
// returning any parsed font face.
func FontsFromStylesheetReader(r io.Reader) ([]Font, error) {
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
	var fonts []Font
	for i, rule := range rules {
		if rule.Type != css.FONT_FACE_RULE {
			continue
		}
		// build font
		var font Font
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
			case "font-display":
				font.Display = style.Value.Text()
			case "font-stretch":
				font.Stretch = style.Value.Text()
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
				panic(fmt.Sprintf("unknown @font-face property %q", style.Property))
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
