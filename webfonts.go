// Package webfonts provides client for the google webfonts helper and a way to
// easily retrieve and serve webfonts.
package webfonts

import (
	"context"

	gfonts "google.golang.org/api/webfonts/v1"
)

// Available retrieves all available webfonts.
func Available(ctx context.Context, opts ...ClientOption) ([]*gfonts.Webfont, error) {
	return NewClient(opts...).Available(ctx)
}

// Faces retrieves the font faces for the specified family.
func Faces(ctx context.Context, family string, opts ...ClientOption) ([]Font, error) {
	return NewClient(opts...).Faces(ctx, family)
}

// All retrieves all font faces for the specified family by using multiple user
// agents.
func All(ctx context.Context, family string, opts ...ClientOption) ([]Font, error) {
	return NewClient(opts...).All(ctx, family)
}

// Format retrieves a font face with the specified format and family.
func Format(ctx context.Context, family, format string, opts ...ClientOption) (Font, error) {
	return NewClient(opts...).Format(ctx, family, format)
}

// EOT retrieves the eot font face for the specified family.
func EOT(ctx context.Context, family string, opts ...ClientOption) (Font, error) {
	return NewClient(opts...).EOT(ctx, family)
}

// SVG retrieves the svg font face for the specified family.
func SVG(ctx context.Context, family string, opts ...ClientOption) (Font, error) {
	return NewClient(opts...).SVG(ctx, family)
}

// TTF retrieves the ttf font face for the specified family.
func TTF(ctx context.Context, family string, opts ...ClientOption) (Font, error) {
	return NewClient(opts...).TTF(ctx, family)
}

// WOFF2 retrieves the woff2 font face for the specified family.
func WOFF2(ctx context.Context, family string, opts ...ClientOption) (Font, error) {
	return NewClient(opts...).WOFF2(ctx, family)
}

// WOFF retrieves the woff font face for the specified family.
func WOFF(ctx context.Context, family string, opts ...ClientOption) (Font, error) {
	return NewClient(opts...).WOFF(ctx, family)
}
