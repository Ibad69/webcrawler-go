package main

import (
	"bytes"
	"flag"
	"fmt"
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
	// crawled bool
	link     string
	elements []string
	// count   int
	mu sync.Mutex
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
	link  map[string]string
	count int
	mu    sync.Mutex
}

func (cs *CrawledStatus) UpdateCrawledStatus(link string) {
	cs.mu.Lock()
	defer cs.mu.Unlock()
	cs.link[link] = link
	cs.count++
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
		// "https://devxonic.com/",
		// // "https://linkitsoft.com",
		// "https://google.com",
		// "https://facebook.com",
		// "https://x.com",
		// "https://youtube.com",
		"http://localhost:8080/index.html",
		"http://localhost:8080/loop.html",
	}

	content_res := make(chan []byte)
	linkchan := make(chan string)

	var wg sync.WaitGroup

	// go func() {
	// 	http.ListenAndServe("localhost:6060", nil)
	// }()

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
	crawler := CrawledStatus{link: make(map[string]string), count: 0}

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

			queue.Dequeue()
			for len(queue.elements) != 0 {
				b := []byte(queue.elements[0])
				ParseHtml(b, &queue, &crawler, done, linkname)
				queue.Dequeue()
				crawler.UpdateCrawledStatus(linkname)
				fmt.Println("crawled", linkname)
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
			break
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
				for _, v := range t.Attr {
					if strings.HasPrefix(v.Val, "http") {
						if c.link[v.Val] != "" {
							continue
						}
						q.Enqueue(v.Val)
					}
				}
			}
		}
	}
}

func Enqueue(chx chan string, link string) {
	chx <- link
}

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

func crawl() {
}

// fmt.Println("queue state?", (buffchan))
// after receiving the value I will check if it already exists in the queue? if it does then I'll skip it
// if it does not thenn we will parse it annd find more links from it
// v := <-done
// fmt.Println("received from channel", v)
// continue
