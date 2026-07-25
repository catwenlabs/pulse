package opml

import (
	"strings"
	"testing"
)

func TestImportNestedFolders(t *testing.T) {
	input := `<?xml version="1.0"?>
	<opml version="2.0"><head><title>Subscriptions</title></head><body>
		<outline text="Technology">
			<outline text="Example Feed" type="rss" xmlUrl="https://example.com/feed.xml" htmlUrl="https://example.com"/>
		</outline>
		<outline text="Standalone" xmlUrl="https://standalone.example/feed"/>
	</body></opml>`

	subscriptions, err := Import(strings.NewReader(input))
	if err != nil {
		t.Fatalf("Import() error = %v", err)
	}
	if len(subscriptions) != 2 {
		t.Fatalf("subscription count = %d", len(subscriptions))
	}
	if subscriptions[0].Title != "Example Feed" ||
		subscriptions[0].FeedURL != "https://example.com/feed.xml" ||
		len(subscriptions[0].Folders) != 1 ||
		subscriptions[0].Folders[0] != "Technology" {
		t.Errorf("nested subscription = %+v", subscriptions[0])
	}
	if len(subscriptions[1].Folders) != 0 {
		t.Errorf("standalone folders = %v", subscriptions[1].Folders)
	}
}

func TestExportRoundTrip(t *testing.T) {
	input := []Subscription{
		{Title: "One", FeedURL: "https://one.example/feed", SiteURL: "https://one.example", Folders: []string{"Tech"}},
		{Title: "Two", FeedURL: "https://two.example/feed"},
	}

	data, err := Export("Pulse subscriptions", input)
	if err != nil {
		t.Fatalf("Export() error = %v", err)
	}
	got, err := Import(strings.NewReader(string(data)))
	if err != nil {
		t.Fatalf("round-trip Import() error = %v", err)
	}
	byURL := make(map[string]Subscription, len(got))
	for _, subscription := range got {
		byURL[subscription.FeedURL] = subscription
	}
	if len(got) != 2 ||
		byURL[input[0].FeedURL].Title != input[0].Title ||
		byURL[input[1].FeedURL].Title != input[1].Title {
		t.Errorf("round trip = %+v", got)
	}
}

func TestImportRejectsInvalidXMLAndSkipsEmptyOutlines(t *testing.T) {
	if _, err := Import(strings.NewReader(`<opml>`)); err == nil {
		t.Fatal("invalid XML error = nil")
	}
	got, err := Import(strings.NewReader(`<opml><body><outline text="folder"/></body></opml>`))
	if err != nil {
		t.Fatalf("Import() error = %v", err)
	}
	if len(got) != 0 {
		t.Errorf("subscriptions = %+v, want empty", got)
	}
}
