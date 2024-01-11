package main

import (
	"embed"
	"fmt"
	"log"
	"net/http"
	"sync"
	"time"
)

//go:embed ui
var UI embed.FS

type SSEClientCollection struct {
	m      map[*http.Request]chan string
	_mutex sync.Mutex
}

func NewSSEClientCollection() *SSEClientCollection {
	return &SSEClientCollection{
		m: make(map[*http.Request]chan string),
	}
}
func (c *SSEClientCollection) Get(r *http.Request) chan string {
	c._mutex.Lock()
	defer c._mutex.Unlock()
	return c.m[r]
}
func (c *SSEClientCollection) Add(r *http.Request) {
	c._mutex.Lock()
	defer c._mutex.Unlock()
	c.m[r] = make(chan string)
}
func (c *SSEClientCollection) Del(r *http.Request) {
	c._mutex.Lock()
	defer c._mutex.Unlock()
	close(c.m[r])
	delete(c.m, r)
}

func (c *SSEClientCollection) ForEach(f func(r *http.Request, ch chan string)) {
	c._mutex.Lock()
	defer c._mutex.Unlock()
	for r, c := range c.m {
		f(r, c)
	}
}

func main() {
	// track all SSE clients
	clients := NewSSEClientCollection()

	// event source emulator
	eventSource := make(chan string)
	go func() {
		for {
			eventSource <- time.Now().String()
			time.Sleep(time.Second)
		}
	}()

	// dispatch event to each SSE client
	go func() {
		for {
			e := <-eventSource
			clients.ForEach(func(r *http.Request, ch chan string) {
				select {
				case ch <- e:
				default:
				}
			})
		}
	}()

	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		http.FileServer(http.FS(UI)).ServeHTTP(w, r)
	})

	http.HandleFunc("/sse/", func(w http.ResponseWriter, r *http.Request) {
		clients.Add(r)
		ch := clients.Get(r)
		defer func() {
			clients.Del(r)
		}()

		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-store")

		for {
			select {
			case <-r.Context().Done():
				return
			case event := <-ch:
				fmt.Fprintf(w, "data: %s\n\n", event)
				if f, ok := w.(http.Flusher); ok {
					f.Flush()
				}
			}
		}
	})
	log.Fatal(http.ListenAndServe(":8080", nil))
}
