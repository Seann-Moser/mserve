package extract

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
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
	"sync/atomic"
	"time"

	app "github.com/lib4u/fake-useragent"
	"github.com/schollz/progressbar/v3"
	"github.com/tidwall/gjson"

	"github.com/PuerkitoBio/goquery"
	"github.com/google/uuid"
	"golang.org/x/sync/semaphore"
)

var ua *app.UserAgent

type Result map[string]interface{}

type RuleProgress struct {
	Total       atomic.Uint64
	Current     atomic.Uint64
	Concurrency int
	Update      chan struct{}
	bar         *progressbar.ProgressBar
}

func (p *RuleProgress) Log() {
	if p.bar == nil {
		p.bar = progressbar.New(int(p.Total.Load()))
	}
}

func (p *RuleProgress) SetTotal(v int) {
	p.Total.Store(uint64(v))
	if p.bar != nil {
		p.bar.ChangeMax(v)
	}
}
func (p *RuleProgress) Close() {
	if p.bar != nil {
		_ = p.bar.Finish()
	}
}
func (p *RuleProgress) IncrementTotal(v int) {
	p.Total.Add(uint64(v))
	if p.bar != nil {
		p.bar.AddMax(v)
	}

}
func (p *RuleProgress) Add(v int) {
	if p.bar != nil {
		_ = p.bar.Add(v)
	}
	p.Current.Add(uint64(v))
	if p.Update != nil {
		p.Update <- struct{}{}
	}
}

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
			dir = replacePlaceholders(dir, 0)
		}
		if rule.Download {
			//todo check for to key and is object. then convert it to a map instead and read the key
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
		if rule.Multiple {
			results := r.GetResultArray(rule.Name)
			if results == nil {
				continue
			}
			var output []Result
			for _, v := range results {
				p := v.DownloadResults(ctx, rule.Children, dir, concurrency)
				output = append(output, p)
			}
			r[rule.Name] = output
		} else {
			v := r.GetResult(rule.Name)
			if v == nil {
				continue
			}

			p := v.DownloadResults(ctx, rule.Children, dir, concurrency)
			r[rule.Name] = p
		}

	}
	return r
}

func replacePlaceholders(s string, index int) string {
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
	if strings.Contains(s, "{{index}}") {
		unixTime := fmt.Sprintf("%d", index)
		s = strings.ReplaceAll(s, "{{index}}", unixTime)
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

func (r Result) GetResultArray(key string) []Result {
	v, ok := r[key]
	if !ok {
		return nil
	}
	i, ok := v.([]Result)
	if ok {
		return i
	}
	if i, ok := v.([]interface{}); ok {
		var output []Result
		for _, v := range i {
			output = append(output, v.(Result))
		}
		return output

	}
	return nil
}

func (r Result) GetResultStringArray(key ...string) []string {
	v, ok := r[key[0]]
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
			if len(key) > 1 {
				raw, err := json.Marshal(i)
				if err != nil {
					slog.Error("failed getting results", "err", err)
					continue
				}
				r := gjson.GetBytes(raw, strings.Join(key[1:], "."))
				if r.Exists() {
					output = append(output, r.String())
				}
			} else {
				if v := safeString(i); v != "" {
					output = append(output, v)
				}
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
func Scrape(fetcher Fetcher, pageURL string, rules []*ExtractionRule, rp *RuleProgress) (Result, error) {
	doc, err := fetcher.Fetch(pageURL)
	if err != nil {
		return nil, err
	}
	base, err := url.Parse(pageURL)
	if err != nil {
		return nil, err
	}

	root := Result{}
	rp.SetTotal(len(rules))
	defer rp.Close()
	for _, rule := range rules {
		v, err := applyRule(fetcher, doc.Selection, rule, base, rp)
		if err != nil {
			return nil, err
		}
		root[rule.Name] = v
		rp.Add(1)

	}
	return root, nil
}

// ScrapeToJSON scrapes the page and returns the data as a JSON byte slice.
func ScrapeToJSON(fetcher Fetcher, pageURL string, rules []*ExtractionRule, rp *RuleProgress) ([]byte, error) {
	result, err := Scrape(fetcher, pageURL, rules, rp)
	if err != nil {
		return nil, err
	}
	return json.Marshal(result)
}

// ScrapeIntoStruct scrapes the page, marshals to JSON, then unmarshals into v.
// v must be a pointer to your target struct (or slice, map, etc).
func ScrapeIntoStruct(fetcher Fetcher, pageURL string, rules []*ExtractionRule, v interface{}, rp *RuleProgress) error {
	data, err := ScrapeToJSON(fetcher, pageURL, rules, rp)
	if err != nil {
		return err
	}
	return json.Unmarshal(data, v)
}

// 3) downloadResource: fetches a URL and writes it under saveDir, returning the local path.
func downloadResource(ctx context.Context, rawURL string, saveDir string) (string, error) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return "", err
	}
	filename := filepath.Base(u.Path)
	if filename == "" || filename == "/" {
		filename = fmt.Sprintf("%d", time.Now().UnixNano())
	}
	p := removeDuplicateExt(filename)
	outPath := filepath.Join(saveDir, p)

	if _, err := os.Stat(outPath); !errors.Is(err, os.ErrNotExist) {
		return outPath, nil
	}

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

	// use last segment as filename, or fall back to a timestamp/UUID if empty

	if saveDir == "" {
		saveDir = "./"
	}
	if err := os.MkdirAll(saveDir, 0o755); err != nil {
		return "", err
	}

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
func applyRule(fetcher Fetcher, sel *goquery.Selection, rule *ExtractionRule, base *url.URL, rp *RuleProgress) (interface{}, error) {
	if len(rule.Transforms) == 0 {
		rule.Transforms = []*Transforms{
			{
				Match:   "^//",
				Replace: "https://",
			},
		}
	}
	// Nested rules take priority
	sel = sel.Clone()
	if len(rule.Children) > 0 {

		if rule.Multiple {
			var list []interface{}

			rp.IncrementTotal(sel.Find(rule.Selector).Length())
			sel.Find(rule.Selector).Each(func(i int, s *goquery.Selection) {
				rec := make(Result)
				defer rp.Add(1)
				// record-level attribute
				if rule.Attr != "" {
					raw, _ := s.Attr(rule.Attr)
					if (rule.Download || rule.Visit) && raw != "" {
						if u2, err := base.Parse(raw); err == nil {
							//todo visit site using fetcher
							if rule.Visit {

								doc, err := fetcher.Fetch(u2.String())
								if err == nil {
									var output []interface{}

									for _, c := range rule.Children {
										if v, err := applyRule(fetcher, doc.Selection, c, base, rp); err == nil {
											rec[c.Name] = v
											//output = append(output, v)
										}

									}
									if len(output) > 1 {
										rec[rule.Attr] = output
									}
								} else {
									rec[rule.Attr] = u2.String()
								}
							} else {
								rec[rule.Attr] = u2.String()
							}

						} else {
							rec[rule.Attr] = raw
						}
					} else {
						rec[rule.Attr] = raw

					}
				}
				if !rule.Visit {
					// child rules
					rp.IncrementTotal(len(rule.Children))
					for _, child := range rule.Children {
						if v, err := applyRule(fetcher, s, child, base, rp); err == nil {
							rec[child.Name] = v
						}
						rp.Add(1)
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
			if (rule.Download || rule.Visit) && raw != "" {
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
			if v, err := applyRule(fetcher, s.Clone(), child, base, rp); err == nil {
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
				if (rule.Download || rule.Visit) && raw != "" {
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

func replacePlaceholdersInterface(raw interface{}, index int) interface{} {
	s := safeString(raw)
	if s == "" {
		return raw
	}
	if strings.EqualFold(s, "{{unix}}") {
		return time.Now().Unix()
	}
	if strings.EqualFold(s, "{{time}}") {
		return time.Now()
	}
	if strings.EqualFold(s, "{{index}}") {
		return index
	}
	if strings.EqualFold(s, "{{index+1}}") {
		return index + 1
	}

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
