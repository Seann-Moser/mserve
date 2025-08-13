package video

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/url"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/schollz/progressbar/v3"
	"golang.org/x/sync/semaphore"

	"github.com/Seann-Moser/mserve/extract"
	"github.com/google/uuid"
)

var nameReg = regexp.MustCompile(`(http(s)*):\/+((\w+)\.*){0,3}\/|.\w{0,4}$|^\/`)

type Client struct {
	fetcher            extract.Fetcher
	dir                string
	Preview            bool
	concurrencyWorkers *semaphore.Weighted
}

func New(f extract.Fetcher, dir string, concurrencyWorkers int64) *Client {
	if concurrencyWorkers <= 0 {
		concurrencyWorkers = 1
	}
	return &Client{
		fetcher:            f,
		dir:                dir,
		concurrencyWorkers: semaphore.NewWeighted(concurrencyWorkers),
	}
}

func (vc *Client) DownloadVideoToFile(ctx context.Context, pageURL, outPath string, rule *extract.Rule) ([]*Video, error) {
	result, err := extract.Scrape(vc.fetcher, pageURL, rule.Rules, nil)
	if err != nil {
		return nil, err
	}
	results, err := extract.MapResultsResults(result, rule)
	if err != nil {
		return nil, err
	}
	//if vc.Preview {
	//	data, err := json.MarshalIndent(results, "", "  ")
	//	if err != nil {
	//		return nil, err
	//	}
	//	fmt.Println(string(data))
	//}
	var videos []*Video
	if v, ok := results["video"]; ok {
		video, err := InterfaceToStruct[Video](v)
		if err != nil {
			return nil, err
		}
		videos = append(videos, video)
	}
	if v, ok := results["videos"]; ok {
		videoList, err := ArrayInterfaceToStruct[Video](v)
		if err != nil {
			return nil, err
		}
		videos = append(videos, videoList...)
	}
	wg := sync.WaitGroup{}
	var bar *progressbar.ProgressBar
	if vc.Preview {
		bar = progressbar.New(len(videos))
		defer func() {
			_ = bar.Finish()
		}()
	}
	for _, v := range videos {
		if v.DownloadedFromURL == "" {
			if bar != nil {
				_ = bar.Add(1)
			}
			continue
		}
		if v.Name == "" {
			if strings.Contains(v.DownloadedFromURL, pageURL) {
				v.Name = strings.ReplaceAll(v.DownloadedFromURL, pageURL, "")
			} else {
				v.Name = v.DownloadedFromURL
			}
			v.Name, _ = url.PathUnescape(v.DownloadedFromURL)
			v.Name = nameReg.ReplaceAllString(v.Name, "")
		}
		err = vc.concurrencyWorkers.Acquire(ctx, 1)
		if err != nil {
			return nil, err
		}
		wg.Add(1)
		go func() {
			defer vc.concurrencyWorkers.Release(1)
			defer wg.Done()
			if strings.HasSuffix(v.DownloadedFromURL, ".m3u8") {
				err = downloadHLS(ctx, v.DownloadedFromURL, filepath.Join(vc.dir, outPath))
				if err != nil {
					slog.Error("failed downloading video", "err", err)
				}
			} else {
				err = downloadFile(ctx, v.DownloadedFromURL, filepath.Join(vc.dir, ToSnakeCase(v.Name)+".mp4"), v)
				if err != nil {
					slog.Error("failed downloading video", "err", err)
				}
			}
			if bar != nil {
				_ = bar.Add(1)
			}
		}()

	}
	wg.Wait()
	// 4) otherwise direct
	return videos, nil
}

var invalidChars = regexp.MustCompile(`[^\p{L}\p{Nd}]+`)

// ToSnakeCase converts s into snake_case:
//   - lower-cases all letters
//   - replaces any run of non-alphanumeric chars with a single underscore
//   - trims leading/trailing underscores
func ToSnakeCase(s string) string {
	// 1) lowercase
	s = strings.ToLower(s)

	// 2) replace invalid chars with underscore
	s = invalidChars.ReplaceAllString(s, "_")

	// 3) trim any leading/trailing underscores
	return strings.Trim(s, "_")
}

func ArrayInterfaceToStruct[T any](i interface{}) ([]*T, error) {
	data, err := json.Marshal(i)
	if err != nil {
		return nil, err
	}
	data = []byte(ReplacePlaceholders(string(data)))
	var t []*T
	err = json.Unmarshal(data, &t)
	if err != nil {
		return nil, err
	}
	return t, nil
}

func InterfaceToStruct[T any](i interface{}) (*T, error) {
	data, err := json.Marshal(i)
	if err != nil {
		return nil, err
	}
	data = []byte(ReplacePlaceholders(string(data)))
	var t T
	err = json.Unmarshal(data, &t)
	if err != nil {
		return nil, err
	}
	return &t, nil
}

func ReplacePlaceholders(s string) string {
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
