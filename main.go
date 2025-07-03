package main

import (
	"bytes"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"runtime"
	"runtime/pprof"
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
	// Parse command-line flags
	flag.Parse()
	// stop := make(chan os.Signal, 1)
	// signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
	// ctx, cancel := context.WithCancel(context.Background())
	// defer cancel()

	var wg sync.WaitGroup

	// Start CPU profiling if the cpuprofile flag is set
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

	fmt.Println("go web crawler ready!!")

	links := []string{
		"https://devxonic.com/",
		"https://google.com",
		"https://facebook.com",
		"https://x.com",
		"https://youtube.com",
	}

	// workerque := make([]string, 3)

	jobschan := make(chan string)
	content_res := make(chan []byte)
	linkchan := make(chan string)
	complete := make(chan int)

	for w := 0; w < 3; w++ {
		// fmt.Println("spawing a worker")
		wg.Add(1)
		go worker(content_res, linkchan, jobschan, complete)
		go crawl(content_res, linkchan, &wg)
	}

	for j := 0; j < len(links); j++ {
		// fmt.Println("trying to send jobs to channel", links[j])
		jobschan <- links[j]
	}

	close(jobschan)

	// <-stop
	log.Println("Shutting gracefully")
}

func ParseHtml(content []byte, q *Queue, c *CrawledStatus, done chan bool, link string) {
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
			}
			if t.Data == "a" {
				// fmt.Println("href?", t.String())
				for _, v := range t.Attr {
					if strings.HasPrefix(v.Val, "http") {
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
			}
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

func worker(res chan []byte, linkchan chan string, jobschan <-chan string, shutdown chan int) {
	for {
		select {
		case job, ok := <-jobschan:
			if !ok {
				fmt.Errorf("channel closed error occured in processing the job")
			}
			// fmt.Println("received in job?")
			// fmt.Println("jobschaneel", job)
			content := readLink(job)
			// fmt.Println("content reading done")
			res <- content
			// fmt.Println("sending link to link channel", job)
			linkchan <- job
		case <-time.After(5 * time.Second):
			fmt.Println("no jobs received gracefully shutting down")
			shutdown <- 1
		}
	}
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

func crawl(content_res chan []byte, linkchan chan string, wg *sync.WaitGroup) {
	for {
		// fmt.Println("inside the loop")
		queue := Queue{elements: make([]string, 0)}
		crawler := CrawledStatus{link: make(map[string]string), count: 0}
		content := <-content_res

		linkname := <-linkchan

		// IMPORTANT *****put all of the below code inside a single crawler and run it in a go routine separately
		wg.Add(1)
		go func() {
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
			}
			fmt.Println("queue for link", queue.elements)
		}()
		fmt.Println("now processing the next link")
	}
}

// fmt.Println("queue state?", (buffchan))
// after receiving the value I will check if it already exists in the queue? if it does then I'll skip it
// if it does not thenn we will parse it annd find more links from it
// v := <-done
// fmt.Println("received from channel", v)
// continue
