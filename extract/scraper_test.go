package extract

import (
	"encoding/json"
	"io/ioutil"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/PuerkitoBio/goquery"
)

// MockFetcher implements Fetcher for testing, returning a static HTML document.
type MockFetcher struct {
	HTML string
}

// Fetch returns a goquery.Document built from the mock HTML.
func (m *MockFetcher) Fetch(pageURL string) (*goquery.Document, error) {
	return goquery.NewDocumentFromReader(strings.NewReader(m.HTML))
}

func TestScrape(t *testing.T) {
	html := `
	<div class="book-list">
	  <div class="book-item">
	    <h2 class="title">Book 1</h2>
	    <div class="byline"><a>Author A</a></div>
	    <div class="tags"><span>tag1</span><span>tag2</span></div>
	  </div>
	  <div class="book-item">
	    <h2 class="title">Book 2</h2>
	    <div class="byline"><a>Author B</a></div>
	    <div class="tags"><span>tag3</span></div>
	  </div>
	</div>`

	rules := []*ExtractionRule{
		{
			Name:     "books",
			Selector: ".book-list .book-item",
			Multiple: true,
			Children: []*ExtractionRule{
				{Name: "title", Selector: "h2.title"},
				{Name: "author", Selector: ".byline a"},
				{Name: "tags", Selector: ".tags span", Multiple: true},
			},
		},
	}

	fetcher := &MockFetcher{HTML: html}
	result, err := Scrape(fetcher, "http://example.com", rules)
	if err != nil {
		t.Fatalf("Scrape failed: %v", err)
	}

	expected := Result{
		"books": []interface{}{
			Result{"title": "Book 1", "author": "Author A", "tags": []interface{}{"tag1", "tag2"}},
			Result{"title": "Book 2", "author": "Author B", "tags": []interface{}{"tag3"}},
		},
	}

	if !reflect.DeepEqual(result, expected) {
		t.Errorf("Result mismatch.\nGot:  %#v\nWant: %#v", result, expected)
	}
}

func TestScrapeToJSONAndIntoStruct(t *testing.T) {
	html := `<ul id="items"><li data-id="1">First</li><li data-id="2">Second</li></ul>`

	rules := []*ExtractionRule{
		{Name: "items", Selector: "#items li", Multiple: true, Attr: "data-id", Children: []*ExtractionRule{{Name: "text", Selector: "*"}}},
	}

	fetcher := &MockFetcher{HTML: html}

	// Test ScrapeToJSON
	jsonData, err := ScrapeToJSON(fetcher, "http://example.com", rules)
	if err != nil {
		t.Fatalf("ScrapeToJSON failed: %v", err)
	}

	// Unmarshal and check structure
	var parsed map[string]interface{}
	if err := json.Unmarshal(jsonData, &parsed); err != nil {
		t.Fatalf("JSON unmarshal failed: %v", err)
	}
	if _, ok := parsed["items"]; !ok {
		t.Errorf("Expected key 'items' in JSON output")
	}

	// Test ScrapeIntoStruct
	type Item struct {
		ID   string `json:"data-id"`
		Text string `json:"text"`
	}
	type Page struct {
		Items []Item `json:"items"`
	}

	var page Page
	if err := ScrapeIntoStruct(fetcher, "http://example.com", rules, &page); err != nil {
		t.Fatalf("ScrapeIntoStruct failed: %v", err)
	}
	if len(page.Items) != 2 || page.Items[0].ID != "1" || page.Items[1].Text != "Second" {
		t.Errorf("Unexpected struct data: %+v", page.Items)
	}
}

func TestRuleSerializationJSONYAML(t *testing.T) {
	rules := []*ExtractionRule{
		{Name: "a", Selector: "s1"},
		{Name: "b", Selector: "s2", Attr: "href", Multiple: true},
	}

	tmpDir, err := ioutil.TempDir("", "rules_test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	jsonPath := filepath.Join(tmpDir, "rules.json")
	yamlPath := filepath.Join(tmpDir, "rules.yaml")

	// JSON
	if err := SaveRulesToJSON(rules, jsonPath); err != nil {
		t.Fatalf("SaveRulesToJSON failed: %v", err)
	}
	loadedJSON, err := LoadRulesFromJSON(jsonPath)
	if err != nil {
		t.Fatalf("LoadRulesFromJSON failed: %v", err)
	}
	if !reflect.DeepEqual(rules, loadedJSON) {
		t.Errorf("JSON round-trip mismatch.\nGot:  %#v\nWant: %#v", loadedJSON, rules)
	}

	// YAML
	if err := SaveRulesToYAML(rules, yamlPath); err != nil {
		t.Fatalf("SaveRulesToYAML failed: %v", err)
	}
	loadedYAML, err := LoadRulesFromYAML(yamlPath)
	if err != nil {
		t.Fatalf("LoadRulesFromYAML failed: %v", err)
	}
	if !reflect.DeepEqual(rules, loadedYAML) {
		t.Errorf("YAML round-trip mismatch.\nGot:  %#v\nWant: %#v", loadedYAML, rules)
	}
}

func TestLoadRulesFromDir(t *testing.T) {
	rules1 := []*ExtractionRule{{Name: "x", Selector: "s1"}}
	rules2 := []*ExtractionRule{{Name: "y", Selector: "s2"}}

	tmpDir, err := ioutil.TempDir("", "dir_rules")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	// write rules1.json and rules2.yaml
	jsonPath := filepath.Join(tmpDir, "r1.json")
	yamlPath := filepath.Join(tmpDir, "r2.yaml")
	if err := SaveRulesToJSON(rules1, jsonPath); err != nil {
		t.Fatal(err)
	}
	if err := SaveRulesToYAML(rules2, yamlPath); err != nil {
		t.Fatal(err)
	}

	loaded, err := LoadRulesFromDir(tmpDir)
	if err != nil {
		t.Fatalf("LoadRulesFromDir failed: %v", err)
	}

	expected := append(rules1, rules2...)
	if !reflect.DeepEqual(expected, loaded) {
		t.Errorf("LoadRulesFromDir mismatch.\nGot:  %#v\nWant: %#v", loaded, expected)
	}
}
