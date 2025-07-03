package main

import (
	"bytes"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"runtime"
	"strings"
	"time"

	"golang.org/x/net/html"
)

// the concept is to create a buffered channel queue which will take in the urls, firslty we will be parsing through the urls so once we find a url we add it in the queue
// adding it in the queue meaning send an update to the channel once any update is received the queue processing will start and in the background queue will start processing the links
// the links processing will further crawl through the inner links and find if it is completely crawled or not
// if it is completely crawled the crawled will be set to true
// if not the crawling is still going on

type Queue struct {
	// crawled bool
	link     string
	elements []string
	// count   int
}

func (q *Queue) Enqueue(url string) {
	q.elements = append(q.elements, url)
}

func (q *Queue) Dequeue() (string, bool) {
	if len(q.elements) == 0 {
		return "", false // or return an error if you prefer
	}
	url := q.elements[0]
	q.elements = q.elements[1:]
	return url, true
}

type CrawledStatus struct {
	link  map[string]string
	count int
}

func (cs *CrawledStatus) UpdateCrawledStatus(link string) {
	cs.link[link] = link
}

var (
	cpuProfile    = flag.String("cpuprofile", "", "write cpu profile to `file`")
	memProfile    = flag.String("memprofile", "", "write memory profile to 'file'")
	realisticSize = flag.Int("size", 2000000, "size of data to process in workload")
)

func main() {
	fmt.Println("go web crawler ready!!")
	// start := time.Now()

	links := []string{
		"https://devxonic.com/",
		"https://google.com",
		"https://facebook.com",
		"https://x.com",
		"https://youtube.com",
	}
	// content := readLink()

	content_res := make(chan []byte)
	linkchan := make(chan string)

	for w := 0; w < len(links); w++ {
		fmt.Println("spawing a worker")
		go worker(links[w], content_res, linkchan)
	}

	for {
		content := <-content_res

		linkname := <-linkchan

		// IMPORTANT *****put all of the below code inside a single crawler and run it in a go routine separately
		// go func() {
		queue := Queue{elements: make([]string, 0)}
		crawler := CrawledStatus{link: make(map[string]string), count: 0}

		done := make(chan bool)
		fmt.Println("linkchan", linkname)

		queue.Enqueue(linkname)
		ParseHtml(content, &queue, &crawler, done, linkname)

		queue.Dequeue()
		for len(queue.elements) != 0 {
			b := []byte(queue.elements[0])
			ParseHtml(b, &queue, &crawler, done, linkname)
			queue.Dequeue()
		}
		// elapsed := time.Since(start)
		// fmt.Println("time taken", elapsed)
		fmt.Println("queue for link", queue.elements)

		// fmt.Println("the state of crawler", crawler)
		// }()
		fmt.Println("now processing the next link")
	}
}

// fmt.Println("queue state?", (buffchan))
// after receiving the value I will check if it already exists in the queue? if it does then I'll skip it
// if it does not thenn we will parse it annd find more links from it
// v := <-done
// fmt.Println("received from channel", v)
// continue

func ParseHtml(content []byte, q *Queue, c *CrawledStatus, done chan bool, link string) {
	// fmt.Printf("🛠️  Started parsing %s, %s at %s\n", link, time.Now().Format("15:04:05"))
	z := html.NewTokenizer(bytes.NewReader(content))

	for {
		tt := z.Next()
		if tt == html.ErrorToken {
			break
		}
		t := z.Token()
		// fmt.Println("Token:", t.Data)
		if t.Type == html.StartTagToken {
			if t.Data == "body" {
				// body = true
			}
			if t.Data == "javascript" || t.Data == "script" || t.Data == "style" {
				// Skip script and style tags
				z.Next()
				continue
			}
			if t.Data == "title" {
				z.Next()
				// title := z.Token().Data // data disappears after z.Token() is called
				// webpage.Title = title
				// fmt.Printf("Count: %d | %s -> %s\n", title)
			}
			if t.Data == "a" {
				// fmt.Println("href?", t.String())
				for _, v := range t.Attr {
					// fmt.Println("printing all values", v.Val)
					// for _, v1 := range v.Val {
					// 	fmt.Println("priting inner values", string(v1))
					// }
					if strings.HasPrefix(v.Val, "http") {
						// fmt.Println("printing actual value now", v.Val, "title ===> ")
						// push the link to the queue
						// Enqueue(chx, v.Val)
						if c.link[v.Val] != "" {
							continue
						}
						q.Enqueue(v.Val)
						c.link[v.Val] = v.Val
						c.count++
						// fmt.Println("sending to channel now?")
						// done <- true
					}
				}
				// ok, href := getHref(t)
				// if !ok {
				// 	continue
				// }
				// if crawled.contains(href) {
				// Already crawled
				// continue
				// } else {
				// q.enqueue(href)
				// }
			}
			// fmt.Printf(
			// 	"✅ Finished parsing %s, %s at %s\n",
			// 	link,
			// 	time.Now().Format("15:04:05"),
			// )
		}
	}
}

func Enqueue(chx chan string, link string) {
	chx <- link
}

func logGoroutineCount() {
	for {
		log.Printf("Number of goroutines: %d\n",
			runtime.NumGoroutine())
		time.Sleep(2 * time.Second)
	}
}

func worker(link string, res chan []byte, linkchan chan string) {
	content := readLink(link)
	res <- content
	fmt.Println("sending link to link channel", link)
	linkchan <- link
}

func readLink(link string) []byte {
	res, err := http.Get(link)
	if err != nil {
		fmt.Println("error occured")
		fmt.Println(err)
	}
	defer res.Body.Close()
	content, err := io.ReadAll(res.Body)
	if err != nil {
		fmt.Println("an error occured in opening the file")
	}
	res.Body.Close()
	if err != nil {
		log.Fatal(err)
	}
	return content
}

func crawl() {
}
