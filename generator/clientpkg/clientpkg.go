package clientpkg

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"strconv"

	"github.com/Seann-Moser/mserve"
	"github.com/spf13/pflag"
)

var _ HttpClient = &Client{}
var _ HttpClient = &MockClient{}

var UseResponseCache = false

type HttpClient interface {
	Request(ctx context.Context, data RequestData, p *mserve.Pagination, retry bool) (resp *ResponseData)
	RequestWithRetry(ctx context.Context, data RequestData, p *mserve.Pagination) (resp *ResponseData)
	SendRequest(ctx context.Context, data RequestData, p *mserve.Pagination) *ResponseData
	AddOauthClient(prefix string)
	SetEndpoint(e string) error
}

type MockClient struct {
}

func (m MockClient) SetEndpoint(e string) error {
	_, err := url.Parse(e)
	if err != nil {
		return err
	}
	return nil
}

func (m MockClient) AddOauthClient(prefix string) {

}

func NewMockClient() *MockClient {
	return &MockClient{}
}

func (m MockClient) defaultResponse() *ResponseData {
	return &ResponseData{
		Status: http.StatusOK,
		Page: &mserve.Pagination{
			Page:       1,
			Limit:      0,
			Total:      0,
			TotalPages: 0,
		},
		Message: "",
		Err:     nil,
		Data:    []byte{},
	}
}

func (m MockClient) Request(ctx context.Context, data RequestData, p *mserve.Pagination, retry bool) *ResponseData {
	return m.defaultResponse()
}

func (m MockClient) RequestWithRetry(ctx context.Context, data RequestData, p *mserve.Pagination) *ResponseData {
	return m.defaultResponse()
}

func (m MockClient) SendRequest(ctx context.Context, data RequestData, p *mserve.Pagination) *ResponseData {
	return m.defaultResponse()
}

type Client struct {
	endpoint     *url.URL
	client       *http.Client
	serviceName  string
	BackOff      *BackOff
	itemsPerPage uint
	CookieJar    http.CookieJar
	UseCookieJar bool
	skipCache    bool
	OAuthClient  *OAuthClient
}

func NewWithFlags(prefix string, c *http.Client) (HttpClient, error) {
	return nil, nil
}

func Flags(prefix string) (*pflag.FlagSet, error) {
	return mserve.BindFlagSet(prefix, BackOff{}, OAuthClient{})
}
func (c *Client) AddOauthClient(prefix string) {
	//TODO implement me
	panic("implement me")
}

func (c *Client) GetEndpoint() *url.URL {
	return c.endpoint
}
func (c *Client) SetEndpoint(e string) error {
	u, err := url.Parse(e)
	if err != nil {
		return err
	}
	c.endpoint = u
	return nil
}

//func Flags(prefix string) *pflag.FlagSet {
//	fs := pflag.NewFlagSet(prefix, pflag.ExitOnError)
//	fs.String(GetFlagWithPrefix(prefix, "endpoint"), "http://127.0.0.1:8080", fmt.Sprintf("[%s]", strings.ToUpper(ToSnakeCase(GetFlagWithPrefix(prefix, "endpoint")))))
//	fs.String(GetFlagWithPrefix(prefix, "service-name"), "default", fmt.Sprintf("[%s]", strings.ToUpper(ToSnakeCase(GetFlagWithPrefix(prefix, "service-name")))))
//	fs.Bool(GetFlagWithPrefix(prefix, "use-cookie-jar"), false, fmt.Sprintf("[%s]", strings.ToUpper(ToSnakeCase(GetFlagWithPrefix(prefix, "use-cookie-jar")))))
//	fs.Bool(GetFlagWithPrefix(prefix, "skip-cache"), false, fmt.Sprintf("[%s]", strings.ToUpper(ToSnakeCase(GetFlagWithPrefix(prefix, "use-cookie-jar")))))
//	fs.Uint(GetFlagWithPrefix(prefix, "items-per-page"), 100, fmt.Sprintf("[%s]", strings.ToUpper(ToSnakeCase(GetFlagWithPrefix(prefix, "items-per-page")))))
//
//	fs.AddFlagSet(BackOffFlags(prefix))
//	return fs
//}
//
//func NewWithFlags(prefix string, client *http.Client) (*Client, error) {
//	return New(
//		viper.GetString(GetFlagWithPrefix(prefix, "endpoint")),
//		viper.GetString(GetFlagWithPrefix(prefix, "service-name")),
//		viper.GetUint(GetFlagWithPrefix(prefix, "items-per-page")),
//		viper.GetBool(GetFlagWithPrefix(prefix, "use-cookie-jar")),
//		client,
//		NewBackoffFromFlags(prefix),
//	)
//}

//func (c *Client) AddOauthClient(prefix string) {
//	c.OAuthClient = NewOAuthClient(prefix)
//}

func New(endpoint, serviceName string, itemsPerPage uint, useCookieJar bool, client *http.Client, backoff *BackOff) (*Client, error) {
	u, err := url.Parse(endpoint)
	if err != nil {
		return nil, err
	}
	if itemsPerPage == 0 || itemsPerPage > 1000 {
		itemsPerPage = 100
	}

	if client.Jar == nil {
		jar, err := cookiejar.New(nil)
		if err != nil {
			return nil, err
		}
		client.Jar = jar
	}
	return &Client{
		endpoint:     u,
		client:       client,
		serviceName:  serviceName,
		BackOff:      backoff,
		itemsPerPage: itemsPerPage,
		CookieJar:    client.Jar,
		UseCookieJar: useCookieJar,
	}, nil
}
func (c *Client) SkipCache(skip bool) {
	c.skipCache = skip
}
func (c *Client) CacheKey(data RequestData, p *mserve.Pagination) string {
	var key string
	key = c.endpoint.String() + data.Path + data.Method + MapToString(data.Params)
	if p != nil {
		key = fmt.Sprintf("%s%d%d", key, p.Page, p.Limit)
	}
	return key
}

func (c *Client) Request(ctx context.Context, data RequestData, p *mserve.Pagination, retry bool) (resp *ResponseData) {
	if retry {
		return c.RequestWithRetry(ctx, data, p)
	}
	return c.SendRequest(ctx, data, p)
}

func (c *Client) RequestWithRetry(ctx context.Context, data RequestData, p *mserve.Pagination) (resp *ResponseData) {
	_ = c.BackOff.Retry(ctx, func() error {
		resp = c.SendRequest(ctx, data, p)
		if resp.Status == http.StatusTooManyRequests {
			return resp.Err
		}
		return nil
	})
	return
}

func (c *Client) SendRequest(ctx context.Context, data RequestData, p *mserve.Pagination) *ResponseData {
	u, err := url.JoinPath(c.endpoint.String(), data.Path)
	if err != nil {
		return &ResponseData{Err: err}
	}

	if p == nil {
		p = &mserve.Pagination{Limit: int(c.itemsPerPage)}
	} else {
		p.Limit = int(c.itemsPerPage)
	}
	if data.Headers == nil {
		data.Headers = map[string]string{}
	}
	if data.Params == nil {
		data.Params = map[string]string{}
	}
	var rawBody []byte
	if data.Body != nil {
		rawBody, err = json.Marshal(data.Body)
		if err != nil {
			return &ResponseData{Err: err}
		}
	}

	req, err := http.NewRequestWithContext(ctx, data.Method, u, bytes.NewReader(rawBody))
	if err != nil {
		return &ResponseData{Err: err, ErrStr: err.Error()}
	}

	//for k, v := range data.Headers {
	//	req.Header.Set(snakeCaseToHeader(ToSnakeCase(k)), v)
	//}

	queryParams := url.Values{}
	data.Params["items_per_page"] = strconv.Itoa(int(p.Limit))
	data.Params["page"] = strconv.Itoa(int(p.Page))

	for k, v := range data.Params {
		queryParams.Add(k, v)
	}
	req.URL.RawQuery = queryParams.Encode()
	if c.OAuthClient != nil {
		return c.OAuthClient.SendRequest(ctx, req, 0)
	}

	resp := NewResponseData(c.client.Do(req))
	if len(resp.Cookies) > 0 && c.UseCookieJar {
		c.CookieJar.SetCookies(c.endpoint, resp.Cookies)
	}
	return resp
}

func MapToString(m map[string]string) string {
	var output = ""
	for k, v := range m {
		output += k + v
	}
	return output
}

func MergeMap[T any](m1, m2 map[string]T) map[string]T {
	if m1 == nil && m2 == nil {
		return map[string]T{}
	}
	if m1 == nil {
		return m2
	}
	if m2 == nil {
		return m1
	}
	for k, v := range m2 {
		if _, found := m1[k]; found {
			continue
		}
		m1[k] = v
	}
	return m1
}
