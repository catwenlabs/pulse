package opml

import (
	"encoding/xml"
	"fmt"
	"io"
	"sort"
	"strings"
)

type Subscription struct {
	Title   string
	FeedURL string
	SiteURL string
	Folders []string
}

type ImportResult struct {
	CreatedSources  int `json:"created_sources"`
	ExistingSources int `json:"existing_sources"`
	CreatedFolders  int `json:"created_folders"`
}

type document struct {
	XMLName xml.Name `xml:"opml"`
	Version string   `xml:"version,attr,omitempty"`
	Head    head     `xml:"head"`
	Body    body     `xml:"body"`
}

type head struct {
	Title string `xml:"title"`
}

type body struct {
	Outlines []outline `xml:"outline"`
}

type outline struct {
	Text     string    `xml:"text,attr,omitempty"`
	Title    string    `xml:"title,attr,omitempty"`
	Type     string    `xml:"type,attr,omitempty"`
	XMLURL   string    `xml:"xmlUrl,attr,omitempty"`
	HTMLURL  string    `xml:"htmlUrl,attr,omitempty"`
	Children []outline `xml:"outline,omitempty"`
}

func Import(reader io.Reader) ([]Subscription, error) {
	var parsed document
	decoder := xml.NewDecoder(io.LimitReader(reader, 8<<20))
	if err := decoder.Decode(&parsed); err != nil {
		return nil, fmt.Errorf("decode OPML: %w", err)
	}

	var subscriptions []Subscription
	for _, item := range parsed.Body.Outlines {
		collect(item, nil, &subscriptions)
	}
	return subscriptions, nil
}

func collect(item outline, folders []string, subscriptions *[]Subscription) {
	title := strings.TrimSpace(item.Text)
	if title == "" {
		title = strings.TrimSpace(item.Title)
	}
	feedURL := strings.TrimSpace(item.XMLURL)
	if feedURL != "" {
		*subscriptions = append(*subscriptions, Subscription{
			Title:   title,
			FeedURL: feedURL,
			SiteURL: strings.TrimSpace(item.HTMLURL),
			Folders: append([]string(nil), folders...),
		})
		return
	}

	nextFolders := folders
	if title != "" {
		nextFolders = append(append([]string(nil), folders...), title)
	}
	for _, child := range item.Children {
		collect(child, nextFolders, subscriptions)
	}
}

func Export(title string, subscriptions []Subscription) ([]byte, error) {
	folders := make(map[string][]outline)
	var root []outline
	for _, subscription := range subscriptions {
		item := outline{
			Text:    subscription.Title,
			Title:   subscription.Title,
			Type:    "rss",
			XMLURL:  subscription.FeedURL,
			HTMLURL: subscription.SiteURL,
		}
		folderNames := normalizedNames(subscription.Folders)
		if len(folderNames) == 0 {
			root = append(root, item)
			continue
		}
		for _, folder := range folderNames {
			folders[folder] = append(folders[folder], item)
		}
	}

	names := make([]string, 0, len(folders))
	for name := range folders {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		root = append(root, outline{Text: name, Title: name, Children: folders[name]})
	}

	parsed := document{
		Version: "2.0",
		Head:    head{Title: title},
		Body:    body{Outlines: root},
	}
	data, err := xml.MarshalIndent(parsed, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("encode OPML: %w", err)
	}
	return append([]byte(xml.Header), data...), nil
}

func normalizedNames(names []string) []string {
	result := make([]string, 0, len(names))
	seen := make(map[string]struct{}, len(names))
	for _, name := range names {
		name = strings.TrimSpace(name)
		key := strings.ToLower(name)
		if name == "" {
			continue
		}
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		result = append(result, name)
	}
	return result
}
