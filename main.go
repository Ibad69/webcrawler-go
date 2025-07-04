package main

import (
	"bytes"
	"flag"
	"fmt"
	"hash/fnv"
	"io"
	"log"
	"net/http"
	_ "net/http/pprof"
	"os"
	"runtime"
	"runtime/pprof"
	"runtime/trace"
	"strings"
	"sync"
	"time"

	"golang.org/x/net/html"
)

// the concept is to create a buffered channel queue which will take in the urls, firslty we will be parsing through the urls so once we find a url we add it in the queue
// adding it in the queue meaning send an update to the channel once any update is received the queue processing will start and in the background queue will start processing the links
// the links processing will further crawl through the inner links and find if it is completely crawled or not
// if it is completely crawled the crawled will be set to true
// if not the crawling is still going on

type Queue struct {
	link     string
	elements []string
	mu       sync.Mutex
}

func (q *Queue) Enqueue(url string) {
	q.mu.Lock()
	defer q.mu.Unlock()
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
	link  map[uint64]bool
	count int
	mu    sync.Mutex
}

func (cs *CrawledStatus) UpdateCrawledStatus(link string) {
	cs.mu.Lock()
	defer cs.mu.Unlock()
	cs.link[hashUrl(link)] = true
	cs.count++
}

func (cs *CrawledStatus) contains(link string) bool {
	cs.mu.Lock()
	defer cs.mu.Unlock()
	return cs.link[hashUrl(link)]
}

func (cs *CrawledStatus) size() int {
	cs.mu.Lock()
	defer cs.mu.Unlock()
	return cs.count
}

func hashUrl(url string) uint64 {
	h := fnv.New64a()
	h.Write([]byte(url))
	return h.Sum64()
}

var (
	cpuProfile    = flag.String("cpuprofile", "", "write cpu profile to `file`")
	memProfile    = flag.String("memprofile", "", "write memory profile to 'file'")
	realisticSize = flag.Int("size", 2000000, "size of data to process in workload")
)

func main() {
	fmt.Println("go web crawler ready!!")
	flag.Parse()

	start := time.Now()

	if *cpuProfile != "" {
		file, err := os.Create(*cpuProfile)
		if err != nil {
			log.Fatal("Could not create CPU profile: ", err)
		}
		defer file.Close()

		if err := pprof.StartCPUProfile(file); err != nil {
			log.Fatal("Could not start CPU profile: ", err)
		}
		defer pprof.StopCPUProfile()
	}

	f, _ := os.Create("trace.out")
	trace.Start(f)
	defer trace.Stop()

	links := []string{
		"http://localhost:8080/index.html",
		"http://localhost:8080/loop.html",
		// "http://localhost:8080/page0.html",
	}

	content_res := make(chan []byte)
	linkchan := make(chan string)

	var wg sync.WaitGroup

	for w := 0; w < len(links); w++ {
		fmt.Println("spawing a worker")
		wg.Add(1)
		go worker(links[w], content_res, linkchan, &wg)
	}

	go func() {
		wg.Wait()
		close(content_res)
		close(linkchan)
		fmt.Println("time taken", time.Since(start))
	}()

	queue := Queue{elements: make([]string, 0)}
	crawler := CrawledStatus{link: make(map[uint64]bool), count: 0}

	go func() {
		for {
			seen := make(map[string]bool) // Create a map to store seen elements

			for _, num := range queue.elements {
				if seen[num] { // If the element is already in the map, it's a duplicate
					fmt.Println("duplicate found in queue??", num, queue.elements)
					// return true
				}
				seen[num] = true // Mark the element as seen
			}
			fmt.Println("no duplicate")
			time.Sleep(2 * time.Second)
			// return false // No duplicates found
		}
	}()

	go logGoroutineCount(&crawler)

	for {
		fmt.Println("waiting for content")

		fmt.Println("received content")

		content, ok := <-content_res

		if !ok {
			fmt.Printf("Worker %d: content channel closed, exiting.\n")
			return
		}

		linkname := <-linkchan
		if !ok {
			fmt.Println("linkchan closed")
			return
		}

		// IMPORTANT *****put all of the below code inside a single crawler and run it in a go routine separately
		wg.Add(1)
		go func(wg *sync.WaitGroup) {
			defer wg.Done()

			done := make(chan bool)
			fmt.Println("linkchan", linkname)

			queue.Enqueue(linkname)
			ParseHtml(content, &queue, &crawler, done, linkname)

			crawler.UpdateCrawledStatus(linkname)
			queue.Dequeue()

			fmt.Println("queue at this time", queue.elements)

			for len(queue.elements) != 0 && crawler.size() < 100 {
				fmt.Println("after deque", queue.elements)
				b := readLink(queue.elements[0])
				ParseHtml(b, &queue, &crawler, done, queue.elements[0])
				crawler.UpdateCrawledStatus(queue.elements[0])
				queue.Dequeue()
			}
			fmt.Println("queue for link", linkname, queue.elements)
			fmt.Println("crawled status", crawler.count)
		}(&wg)
		fmt.Println("now processing the next link")
		// case <-time.After(10 * time.Second):
		// fmt.Println(runtime.NumGoroutine())
		// wg.Wait()
		// fmt.Println(
		// "no content received for 10 seconds so trying to exit after closing all go routines",
		// )
		// fmt.Println("crawled status", crawler.count)
		// return
		// }
		// pprof.Lookup("goroutine").WriteTo(os.Stdout, 1)
		// fmt.Println("breaking?")

	}
}

func ParseHtml(content []byte, q *Queue, c *CrawledStatus, done chan bool, link string) {
	z := html.NewTokenizer(bytes.NewReader(content))

	for {
		tt := z.Next()
		if tt == html.ErrorToken {
			return
		}
		t := z.Token()
		if t.Type == html.StartTagToken {
			if t.Data == "body" {
			}
			if t.Data == "javascript" || t.Data == "script" || t.Data == "style" {
				z.Next()
				continue
			}
			if t.Data == "title" {
				z.Next()
			}
			if t.Data == "a" {
				// fmt.Println("found links?", t.Attr)
				// for _, v := range t.Attr {
				// 	if strings.HasPrefix(v.Val, "http") {
				// 		if c.contains(v.Val) {
				// 			fmt.Println("cotinuing at", v)
				// 			continue
				// 		}
				// 		// fmt.Println("adding to queue", v)
				// 		fmt.Println("enqueing", v.Val)
				// 		q.Enqueue(v.Val)
				// 	}
				// }
				ok, href := getHref(t)
				fmt.Println("href", href)
				fmt.Println(q.elements)
				fmt.Println(c.contains(href))
				if !ok {
					continue
				}
				if c.contains(href) {
					continue
				} else {
					q.Enqueue(href)
				}
			}
		}
	}
}

// func Enqueue(chx chan string, link string) {
// 	chx <- link
// }

func logGoroutineCount(crawler *CrawledStatus) {
	for {
		fmt.Println(crawler.count)
		log.Printf("Number of goroutines: %d\n",
			runtime.NumGoroutine())
		time.Sleep(2 * time.Second)

	}
}

func worker(link string, res chan []byte, linkchan chan string, wg *sync.WaitGroup) {
	defer wg.Done()
	content := readLink(link)
	fmt.Println("sending content to res channel")
	res <- content
	// fmt.Println("sending link to link channel", link)
	linkchan <- link
	return
}

func readLinkV2(link string) []byte {
	client := http.Client{
		Timeout: 5 * time.Second,
	}
	res, err := client.Get(link)
	if err != nil {
		fmt.Println("error occured")
		fmt.Println(err)
	}
	defer res.Body.Close()
	content, err := io.ReadAll(res.Body)
	if err != nil {
		fmt.Println("an error occured in opening the file")
	}
	if err != nil {
		log.Fatal(err)
	}
	return content
}

func readLink(link string) []byte {
	client := http.Client{
		Timeout: 5 * time.Second, // always set a timeout!
	}

	res, err := client.Get(link)
	if err != nil {
		log.Printf("error fetching link %s: %v", link, err)
		return nil
	}
	defer res.Body.Close()

	content, err := io.ReadAll(res.Body)
	if err != nil {
		log.Printf("error reading body from %s: %v", link, err)
		return nil
	}

	return content
}

func getHref(t html.Token) (ok bool, href string) {
	for _, a := range t.Attr {
		if a.Key == "href" {
			if len(a.Val) == 0 || !strings.HasPrefix(a.Val, "http") {
				ok = false
				href = a.Val
				return ok, href
			}
			href = a.Val
			ok = true
		}
	}
	return ok, href
}

// fmt.Println("queue state?", (buffchan))
// after receiving the value I will check if it already exists in the queue? if it does then I'll skip it
// if it does not thenn we will parse it annd find more links from it
// v := <-done
// fmt.Println("received from channel", v)
// continue
