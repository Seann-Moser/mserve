package extract

import (
	"encoding/json"
	"fmt"
	"github.com/PuerkitoBio/goquery"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type Result map[string]interface{}

// Scrape applies the given extraction rules against the document
// obtained via the provided Fetcher.
func Scrape(fetcher Fetcher, pageURL string, rules []*ExtractionRule) (Result, error) {
	doc, err := fetcher.Fetch(pageURL)
	if err != nil {
		return nil, err
	}
	base, err := url.Parse(pageURL)
	if err != nil {
		return nil, err
	}

	root := Result{}
	for _, rule := range rules {
		v, err := applyRule(doc.Selection, rule, base)
		if err != nil {
			return nil, err
		}
		root[rule.Name] = v
	}
	return root, nil
}

// ScrapeToJSON scrapes the page and returns the data as a JSON byte slice.
func ScrapeToJSON(fetcher Fetcher, pageURL string, rules []*ExtractionRule) ([]byte, error) {
	result, err := Scrape(fetcher, pageURL, rules)
	if err != nil {
		return nil, err
	}
	return json.Marshal(result)
}

// ScrapeIntoStruct scrapes the page, marshals to JSON, then unmarshals into v.
// v must be a pointer to your target struct (or slice, map, etc).
func ScrapeIntoStruct(fetcher Fetcher, pageURL string, rules []*ExtractionRule, v interface{}) error {
	data, err := ScrapeToJSON(fetcher, pageURL, rules)
	if err != nil {
		return err
	}
	return json.Unmarshal(data, v)
}

// 3) downloadResource: fetches a URL and writes it under saveDir, returning the local path.
func downloadResource(rawURL string, saveDir string) (string, error) {
	resp, err := http.Get(rawURL)
	if err != nil {
		return "", err
	}
	defer func(Body io.ReadCloser) {
		err := Body.Close()
		if err != nil {
			slog.Error("Failed to close response body", "err", err)
		}
	}(resp.Body)
	if resp.StatusCode != 200 {
		return "", fmt.Errorf("bad status downloading %s: %d", rawURL, resp.StatusCode)
	}

	u, err := url.Parse(rawURL)
	if err != nil {
		return "", err
	}
	// use last segment as filename, or fall back to a timestamp/UUID if empty
	filename := filepath.Base(u.Path)
	if filename == "" || filename == "/" {
		filename = fmt.Sprintf("%d", time.Now().UnixNano())
	}
	if saveDir == "" {
		saveDir = "./"
	}
	if err := os.MkdirAll(saveDir, 0o755); err != nil {
		return "", err
	}
	outPath := filepath.Join(saveDir, filename)
	f, err := os.Create(outPath)
	if err != nil {
		return "", err
	}
	defer func(f *os.File) {
		_ = f.Close()
	}(f)

	if _, err := io.Copy(f, resp.Body); err != nil {
		return "", err
	}
	return outPath, nil
}

// flattenList recursively flattens a slice of interface{}
func flattenList(vals []interface{}) []interface{} {
	var flat []interface{}
	for _, v := range vals {
		if nested, ok := v.([]interface{}); ok {
			flat = append(flat, flattenList(nested)...)
		} else {
			flat = append(flat, v)
		}
	}
	return flat
}

// 4) applyRule now takes a baseURL to resolve relatives
// applyRule recursively extracts data per rule
func applyRule(sel *goquery.Selection, rule *ExtractionRule, base *url.URL) (interface{}, error) {
	// Nested rules take priority
	sel = sel.Clone()
	if len(rule.Children) > 0 {
		if rule.Multiple {
			var list []interface{}
			sel.Find(rule.Selector).Each(func(i int, s *goquery.Selection) {
				rec := make(Result)
				// record-level attribute
				if rule.Attr != "" {
					raw, _ := s.Attr(rule.Attr)
					if rule.Download && raw != "" {
						if u2, err := base.Parse(raw); err == nil {
							if path, err := downloadResource(u2.String(), rule.SaveDir); err == nil {
								rec[rule.Attr] = path
							} else {
								rec[rule.Attr] = raw
							}
						} else {
							rec[rule.Attr] = raw
						}
					} else {
						rec[rule.Attr] = raw
					}
				}
				// child rules
				for _, child := range rule.Children {
					if v, err := applyRule(s, child, base); err == nil {
						rec[child.Name] = v
					}
				}
				list = append(list, rec)
			})
			return list, nil
		}
		// single
		s := sel.Find(rule.Selector).First()
		rec := make(Result)
		if rule.Attr != "" {
			raw, _ := s.Attr(rule.Attr)
			if rule.Download && raw != "" {
				if u2, err := base.Parse(raw); err == nil {
					if path, err := downloadResource(u2.String(), rule.SaveDir); err == nil {
						rec[rule.Attr] = path
					} else {
						rec[rule.Attr] = raw
					}
				} else {
					rec[rule.Attr] = raw
				}
			} else {
				rec[rule.Attr] = raw
			}
		}
		for _, child := range rule.Children {
			if v, err := applyRule(s.Clone(), child, base); err == nil {
				rec[child.Name] = v
			}
		}
		return rec, nil
	}

	// leaf rule
	if rule.Multiple {
		var vals []interface{}
		sel.Find(rule.Selector).Each(func(i int, s *goquery.Selection) {
			if rule.Attr != "" {
				raw, _ := s.Attr(rule.Attr)
				if rule.Download && raw != "" {
					if u2, err := base.Parse(raw); err == nil {
						if path, err := downloadResource(u2.String(), rule.SaveDir); err == nil {
							raw = u2.String() + ":::" + path
						}
					}
				}

				vals = append(vals, raw)
			} else {
				vals = append(vals, strings.TrimSpace(s.Text()))
			}
		})
		if rule.Flatten {
			vals = flattenList(vals)
		}
		return vals, nil
	}
	s := sel.Find(rule.Selector).First()
	if rule.Attr != "" {
		raw, _ := s.Attr(rule.Attr)
		return raw, nil
	}
	return strings.TrimSpace(s.Text()), nil
}
