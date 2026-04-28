package api

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"reflect"
	"regexp"
)

var nextLinkRE = regexp.MustCompile(`<([^>]+)>;\s*rel="next"`)

// paginate walks GitHub's Link-header pagination, appending each page's
// entries into out (which must point to a slice).
func (c *ghClient) paginate(ctx context.Context, path string, out interface{}) error {
	dst := reflect.ValueOf(out).Elem()
	if dst.Kind() != reflect.Slice {
		panic("paginate: out must point to a slice")
	}
	for path != "" {
		resp, err := c.rest.RequestWithContext(ctx, http.MethodGet, path, nil)
		if err != nil {
			return err
		}
		body, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil {
			return err
		}
		page := reflect.New(dst.Type())
		if err := json.Unmarshal(body, page.Interface()); err != nil {
			return err
		}
		dst.Set(reflect.AppendSlice(dst, page.Elem()))
		path = nextLink(resp.Header.Get("Link"))
	}
	return nil
}

func nextLink(header string) string {
	for _, part := range splitLinkHeader(header) {
		if m := nextLinkRE.FindStringSubmatch(part); len(m) == 2 {
			return m[1]
		}
	}
	return ""
}

func splitLinkHeader(h string) []string {
	if h == "" {
		return nil
	}
	var parts []string
	depth := 0
	start := 0
	for i := 0; i < len(h); i++ {
		switch h[i] {
		case '<':
			depth++
		case '>':
			depth--
		case ',':
			if depth == 0 {
				parts = append(parts, h[start:i])
				start = i + 1
			}
		}
	}
	parts = append(parts, h[start:])
	return parts
}
