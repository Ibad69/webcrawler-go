package main

import (
	"testing"
)

func TestParseHtml(t *testing.T) {
	htmlContent := []byte(`
		<html>
			<head><title>Test Page</title></head>
			<body>
				<a href="http://example.com/page1">Link1</a>
				<a href="http://example.com/page2">Link2</a>
				<a href="/internal">Relative</a>
				<a href="http://example.com/page1">Duplicate</a>
				<script>console.log("ignore")</script>
				<style>.hidden{}</style>
			</body>
		</html>
	`)

	q := &Queue{}
	c := &CrawledStatus{link: make(map[string]string)}

	ParseHtml(htmlContent, q, c)

	expectedLinks := []string{
		"http://example.com/page1",
		"http://example.com/page2",
	}

	if len(q.elements) != len(expectedLinks) {
		t.Fatalf("expected %d URLs in queue, got %d", len(expectedLinks), len(q.elements))
	}

	// Check for correctness of queued links
	for _, expected := range expectedLinks {
		found := false
		for _, actual := range q.elements {
			if actual == expected {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("expected URL %s not found in queue", expected)
		}
	}

	// Ensure relative link is not added
	for _, actual := range q.elements {
		if actual == "/internal" {
			t.Errorf("relative URL /internal should not be enqueued")
		}
	}

	// Check for duplicate count prevention
	if c.count != 2 {
		t.Errorf("expected crawled count to be 2, got %d", c.count)
	}
}
