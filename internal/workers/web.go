package workers

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"golang.org/x/net/html"
	"golang.org/x/net/html/atom"
)

type WebWorker struct {
	httpClient *http.Client
}

func NewWebWorker() *WebWorker {
	return &WebWorker{
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

func (w *WebWorker) GetTools() []ToolDef {
	return []ToolDef{
		{Name: "fetch", Description: "Fetch a web page"},
		{Name: "scrape", Description: "Scrape structured data from page"},
		{Name: "extract_links", Description: "Extract all links from page"},
		{Name: "extract_images", Description: "Extract all images from page"},
		{Name: "search", Description: "Search for text in page"},
		{Name: "extract_metadata", Description: "Extract page metadata"},
	}
}

func (w *WebWorker) Execute(ctx context.Context, name string, input json.RawMessage) ([]byte, error) {
	switch name {
	case "fetch", "web_fetch":
		return w.fetch(ctx, input)
	case "scrape", "web_scrape":
		return w.scrape(ctx, input)
	case "extract_links", "web_extract_links":
		return w.extractLinks(ctx, input)
	case "extract_images", "web_extract_images":
		return w.extractImages(ctx, input)
	case "search", "web_search":
		return w.search(ctx, input)
	case "extract_metadata", "web_extract_metadata":
		return w.extractMetadata(ctx, input)
	default:
		return nil, nil
	}
}

type FetchInput struct {
	URL     string            `json:"url"`
	Headers map[string]string `json:"headers"`
}

func (w *WebWorker) fetch(ctx context.Context, input json.RawMessage) ([]byte, error) {
	var req FetchInput
	json.Unmarshal(input, &req)

	if req.URL == "" {
		return nil, fmt.Errorf("url is required")
	}

	httpReq, err := http.NewRequestWithContext(ctx, "GET", req.URL, nil)
	if err != nil {
		return nil, err
	}

	for k, v := range req.Headers {
		httpReq.Header.Set(k, v)
	}
	httpReq.Header.Set("User-Agent", "Mozilla/5.0 (compatible; MCP-Bot/1.0)")

	resp, err := w.httpClient.Do(httpReq)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	return json.Marshal(map[string]interface{}{
		"url":          req.URL,
		"status":       resp.Status,
		"status_code":  resp.StatusCode,
		"headers":      resp.Header,
		"content":      string(body),
		"content_type": resp.Header.Get("Content-Type"),
	})
}

type ScrapeInput struct {
	URL      string `json:"url"`
	Selector string `json:"selector"`
}

func (w *WebWorker) scrape(ctx context.Context, input json.RawMessage) ([]byte, error) {
	var req ScrapeInput
	json.Unmarshal(input, &req)

	if req.URL == "" {
		return nil, fmt.Errorf("url is required")
	}

	httpReq, err := http.NewRequestWithContext(ctx, "GET", req.URL, nil)
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("User-Agent", "Mozilla/5.0 (compatible; MCP-Bot/1.0)")

	resp, err := w.httpClient.Do(httpReq)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	doc, err := html.Parse(resp.Body)
	if err != nil {
		return nil, err
	}

	var results []map[string]string
	selector := req.Selector
	if selector == "" {
		selector = "p, h1, h2, h3, a"
	}

	var traverse func(n *html.Node)
	traverse = func(n *html.Node) {
		if n.Type == html.ElementNode {
			tag := strings.ToLower(n.DataAtom.String())
			wantTags := strings.Split(selector, ",")
			for _, want := range wantTags {
				want = strings.TrimSpace(want)
				if tag == want {
					var text string
					for c := n.FirstChild; c != nil; c = c.NextSibling {
						if c.Type == html.TextNode {
							text += c.Data
						}
					}
					results = append(results, map[string]string{
						"tag":  tag,
						"text": strings.TrimSpace(text),
					})
					break
				}
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			traverse(c)
		}
	}
	traverse(doc)

	return json.Marshal(map[string]interface{}{
		"url":     req.URL,
		"results": results,
		"count":   len(results),
	})
}

type ExtractLinksInput struct {
	URL          string   `json:"url"`
	InnerText    bool     `json:"inner_text"`
	FilterScheme []string `json:"filter_scheme"`
}

func (w *WebWorker) extractLinks(ctx context.Context, input json.RawMessage) ([]byte, error) {
	var req ExtractLinksInput
	json.Unmarshal(input, &req)

	if req.URL == "" {
		return nil, fmt.Errorf("url is required")
	}

	httpReq, err := http.NewRequestWithContext(ctx, "GET", req.URL, nil)
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("User-Agent", "Mozilla/5.0 (compatible; MCP-Bot/1.0)")

	resp, err := w.httpClient.Do(httpReq)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	doc, err := html.Parse(resp.Body)
	if err != nil {
		return nil, err
	}

	var links []map[string]string
	var findLinks func(n *html.Node)
	findLinks = func(n *html.Node) {
		if n.Type == html.ElementNode && n.DataAtom == atom.A {
			var href, text string
			for _, attr := range n.Attr {
				if attr.Key == "href" {
					href = attr.Val
				}
			}
			if req.InnerText {
				for c := n.FirstChild; c != nil; c = c.NextSibling {
					if c.Type == html.TextNode {
						text += c.Data
					}
				}
			}

			if href != "" {
				if len(req.FilterScheme) > 0 {
					for _, scheme := range req.FilterScheme {
						if strings.HasPrefix(href, scheme+":") {
							links = append(links, map[string]string{
								"href": href,
								"text": strings.TrimSpace(text),
							})
							break
						}
					}
				} else {
					links = append(links, map[string]string{
						"href": href,
						"text": strings.TrimSpace(text),
					})
				}
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			findLinks(c)
		}
	}
	findLinks(doc)

	return json.Marshal(map[string]interface{}{
		"url":   req.URL,
		"links": links,
		"count": len(links),
	})
}

type ExtractImagesInput struct {
	URL string `json:"url"`
	Alt bool   `json:"alt"`
}

func (w *WebWorker) extractImages(ctx context.Context, input json.RawMessage) ([]byte, error) {
	var req ExtractImagesInput
	json.Unmarshal(input, &req)

	if req.URL == "" {
		return nil, fmt.Errorf("url is required")
	}

	httpReq, err := http.NewRequestWithContext(ctx, "GET", req.URL, nil)
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("User-Agent", "Mozilla/5.0 (compatible; MCP-Bot/1.0)")

	resp, err := w.httpClient.Do(httpReq)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	doc, err := html.Parse(resp.Body)
	if err != nil {
		return nil, err
	}

	var images []map[string]string
	var findImages func(n *html.Node)
	findImages = func(n *html.Node) {
		if n.Type == html.ElementNode && n.DataAtom == atom.Img {
			var src, alt string
			for _, attr := range n.Attr {
				switch attr.Key {
				case "src":
					src = attr.Val
				case "alt":
					alt = attr.Val
				}
			}
			if src != "" {
				img := map[string]string{"src": src}
				if req.Alt {
					img["alt"] = alt
				}
				images = append(images, img)
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			findImages(c)
		}
	}
	findImages(doc)

	return json.Marshal(map[string]interface{}{
		"url":    req.URL,
		"images": images,
		"count":  len(images),
	})
}

type SearchInput struct {
	URL           string `json:"url"`
	Query         string `json:"query"`
	CaseSensitive bool   `json:"case_sensitive"`
}

func (w *WebWorker) search(ctx context.Context, input json.RawMessage) ([]byte, error) {
	var req SearchInput
	json.Unmarshal(input, &req)

	if req.URL == "" {
		return nil, fmt.Errorf("url is required")
	}
	if req.Query == "" {
		return nil, fmt.Errorf("query is required")
	}

	httpReq, err := http.NewRequestWithContext(ctx, "GET", req.URL, nil)
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("User-Agent", "Mozilla/5.0 (compatible; MCP-Bot/1.0)")

	resp, err := w.httpClient.Do(httpReq)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	content := string(body)
	if !req.CaseSensitive {
		content = strings.ToLower(content)
		req.Query = strings.ToLower(req.Query)
	}

	matches := strings.Count(content, req.Query)

	return json.Marshal(map[string]interface{}{
		"url":     req.URL,
		"query":   req.Query,
		"matches": matches,
	})
}

type MetadataInput struct {
	URL string `json:"url"`
}

func (w *WebWorker) extractMetadata(ctx context.Context, input json.RawMessage) ([]byte, error) {
	var req MetadataInput
	json.Unmarshal(input, &req)

	if req.URL == "" {
		return nil, fmt.Errorf("url is required")
	}

	httpReq, err := http.NewRequestWithContext(ctx, "GET", req.URL, nil)
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("User-Agent", "Mozilla/5.0 (compatible; MCP-Bot/1.0)")

	resp, err := w.httpClient.Do(httpReq)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	doc, err := html.Parse(resp.Body)
	if err != nil {
		return nil, err
	}

	metadata := make(map[string]string)

	var findMeta func(n *html.Node)
	findMeta = func(n *html.Node) {
		if n.Type == html.ElementNode && n.DataAtom == atom.Meta {
			var name, content string
			for _, attr := range n.Attr {
				switch attr.Key {
				case "name", "property":
					name = attr.Val
				case "content":
					content = attr.Val
				}
			}
			if name != "" && content != "" {
				metadata[name] = content
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			findMeta(c)
		}
	}
	findMeta(doc)

	title := extractTitle(doc)
	if title != "" {
		metadata["title"] = title
	}

	metadata["url"] = req.URL
	metadata["status"] = resp.Status

	return json.Marshal(metadata)
}

func extractTitle(n *html.Node) string {
	if n.Type == html.ElementNode && n.DataAtom == atom.Title {
		if n.FirstChild != nil && n.FirstChild.Type == html.TextNode {
			return n.FirstChild.Data
		}
	}
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		if title := extractTitle(c); title != "" {
			return title
		}
	}
	return ""
}
