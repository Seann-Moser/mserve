package extract

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	app "github.com/lib4u/fake-useragent"

	"github.com/PuerkitoBio/goquery"
	"github.com/google/uuid"
	"golang.org/x/sync/semaphore"
)

var ua *app.UserAgent

type Result map[string]interface{}

func (r Result) DownloadResults(ctx context.Context, rules []*ExtractionRule, baseDir string, concurrency int) Result {
	for _, rule := range rules {
		if !rule.Download && len(rule.Children) == 0 {
			continue
		}
		dir := baseDir
		if rule.SaveDir != "" {
			if baseDir != "" {
				dir = filepath.Join(baseDir, rule.SaveDir)
			} else {
				dir = rule.SaveDir
			}
		}
		if strings.Contains(dir, "{{") {
			dir = replacePlaceholders(dir)
		}
		if rule.Download {
			urlResults := r.GetResultStringArray(rule.Name)
			var downloadList = make([]string, len(urlResults))
			if concurrency < 1 {
				concurrency = 1
			}
			s := semaphore.NewWeighted(int64(concurrency))
			wg := sync.WaitGroup{}
			for i, u := range urlResults {
				wg.Add(1)
				err := s.Acquire(ctx, 1)
				if err != nil {
					continue
				}
				go func() {
					defer s.Release(1)
					defer wg.Done()
					p, _ := downloadResource(ctx, u, dir)
					downloadList[i] = p

				}()
			}
			wg.Wait()
			r[rule.Name] = downloadList
		}
		v := r.GetResult(rule.Name)
		if v == nil {
			continue
		}

		p := v.DownloadResults(ctx, rule.Children, dir, concurrency)
		r[rule.Name] = p
	}
	return r
}

func replacePlaceholders(s string) string {
	// 1. Replace {{uuid}}
	if strings.Contains(s, "{{uuid}}") {
		newUUID := uuid.New().String()
		s = strings.ReplaceAll(s, "{{uuid}}", newUUID)
	}

	// 2. Replace {{hex}}
	if strings.Contains(s, "{{hex}}") {
		bytes := make([]byte, 16)
		if _, err := rand.Read(bytes); err != nil {
			panic(err)
		}
		hexString := hex.EncodeToString(bytes)
		s = strings.ReplaceAll(s, "{{hex}}", hexString)
	}

	// 3. Replace {{base64}}
	if strings.Contains(s, "{{base64}}") {
		bytes := make([]byte, 18)
		if _, err := rand.Read(bytes); err != nil {
			panic(err)
		}
		base64String := base64.StdEncoding.EncodeToString(bytes)
		s = strings.ReplaceAll(s, "{{base64}}", base64String)
	}

	// 4. Replace {{unix}}
	if strings.Contains(s, "{{unix}}") {
		unixTime := fmt.Sprintf("%d", time.Now().Unix())
		s = strings.ReplaceAll(s, "{{unix}}", unixTime)
	}

	return s
}

func (r Result) GetResult(key string) Result {
	v, ok := r[key]
	if !ok {
		return nil
	}
	i, ok := v.(Result)
	if ok {
		return i
	}
	return nil
}

func (r Result) GetResultStringArray(key string) []string {
	v, ok := r[key]
	if !ok {
		return nil
	}
	i, ok := v.([]string)
	if ok {
		return i
	}
	a, ok := v.(string)
	if ok {
		return []string{a}
	}

	it, ok := v.([]interface{})
	if ok {
		var output []string
		for _, i := range it {
			if v := safeString(i); v != "" {
				output = append(output, v)
			}
		}
		return output
	}
	if i := safeString(v); i != "" {
		return []string{i}
	}
	return nil
}

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
func downloadResource(ctx context.Context, rawURL string, saveDir string) (string, error) {
	r, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return "", err
	}
	if ua == nil {
		ua, err = app.New()
		if err == nil {
			r.Header.Set("User-Agent", ua.GetRandom())
		}
	} else {
		r.Header.Set("User-Agent", ua.GetRandom())
	}
	resp, err := http.DefaultClient.Do(r)
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
	p := removeDuplicateExt(filename)
	outPath := filepath.Join(saveDir, p)
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

func removeDuplicateExt(filename string) string {
	ext := filepath.Ext(filename)
	if ext == "" {
		return filename
	}

	ext = ext[1:] // remove the dot
	base := strings.TrimSuffix(filename, "."+ext)

	// Check if the base name ends with the same extension
	if strings.HasSuffix(base, "."+ext) {
		// Recursively remove the duplicate extension
		return removeDuplicateExt(base)
	}

	return filename
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
							rec[rule.Attr] = u2.String()
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
			return applyTransforms(list, rule.Transforms...), nil
		}
		// single
		s := sel.Find(rule.Selector).First()
		rec := make(Result)
		if rule.Attr != "" {
			raw, _ := s.Attr(rule.Attr)
			if rule.Download && raw != "" {
				if u2, err := base.Parse(raw); err == nil {

					rec[rule.Attr] = u2.String()

				} else {
					rec[rule.Attr] = applyTransform(raw, rule.Transforms...)
				}
			} else {
				rec[rule.Attr] = applyTransform(raw, rule.Transforms...)
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
				//todo refactor to run downloads after full thing is finished
				if rule.Download && raw != "" {
					if u2, err := base.Parse(raw); err == nil {
						raw = u2.String()
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
		return applyTransforms(vals, rule.Transforms...), nil
	}
	s := sel.Find(rule.Selector).First()
	if rule.Attr != "" {
		raw, _ := s.Attr(rule.Attr)
		return applyTransform(raw, rule.Transforms...), nil
	}
	return applyTransform(strings.TrimSpace(s.Text()), rule.Transforms...), nil
}

func applyTransforms(raw []interface{}, t ...*Transforms) []interface{} {
	var o []interface{}
	for _, r := range raw {
		o = append(o, applyTransform(r, t...)...)
	}
	return o
}

func applyTransform(raw interface{}, t ...*Transforms) []interface{} {
	if len(t) == 0 {
		return []interface{}{raw}
	}
	ss := safeString(raw)
	if ss == "" {
		return []interface{}{raw}
	}
	o := []interface{}{}
	for _, tranform := range t {
		r, err := regexp.Compile(tranform.Match)
		if err != nil {
			slog.Error("failed running regex", "reg", tranform.Match)
			continue
		}
		if tranform.Split {
			for _, v := range r.Split(ss, -1) {
				if len(v) > 0 {
					o = append(o, v)
				}
			}
		} else {
			ss = r.ReplaceAllString(ss, tranform.Replace)
		}
	}
	if len(o) > 0 {
		return o
	}
	return []interface{}{ss}
}
func safeString(r interface{}) string {
	switch v := r.(type) {
	case string:
		return v
	case int:
		return strconv.Itoa(v)
	default:
		return ""
	}
}
