package main

import (
	"crypto/sha512"
	b64 "encoding/base64"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"regexp"
	"strconv"
	"time"
)

type CommandType int

const (
	GetHashCommand = iota
	SetHashCommand
	GetCountCommand
	GetStatsCommand
)
const (
	// ChannelCapacity used to define a buffered channel.
	// This is the number of concurrent, non-blocking requests that server can handle.
	ChannelCapacity = 200
	// PreprocessingDelay is the wait time before processing the inbound request.
	PreprocessingDelay = 5
)

// Command struct holds the request data.
type Command struct {
	requestType     CommandType
	password        string
	id              int
	responseChannel chan string
	requestStartTs  int64
}

// Server is the shared data structure for HTTP handlers.
type Server struct {
	inboundRequests chan<- Command
	isTerminated    bool
}

// Stats defines response structure for '/stats' endpoint.
type Stats struct {
	// TotalNum of requests processed bu the server.
	TotalNum int `json:"total"`
	// AverageTime in microsecond for processing a request.
	AverageTime float64 `json:"average"`
}

// CreatePasswordStore creates a goroutine that provides an in-memory datastore to store passwords received.
// It returns a channel which is used to send commands to operate on password store.
func CreatePasswordStore() chan<- Command {
	// secretStore is in-memory datastore for storing hashed-encoded passwords.
	secretStore := make(map[int]string)
	// counter maintains total number of '/hash' requests received by the server
	counter := 0
	// inboundRequests creates a buffered-channel to handle inbound requests to the server.
	inboundRequests := make(chan Command, ChannelCapacity)
	var totalTime int64

	// Following goroutine will run concurrently to handle requests sent to the channel.
	go func() {
		for r := range inboundRequests {
			switch r.requestType {
			case GetHashCommand:
				if val, ok := secretStore[r.id]; ok {
					r.responseChannel <- val
				} else {
					r.responseChannel <- "Invalid hash id!"
				}
			case SetHashCommand:
				// time.Sleep(500 * time.Millisecond)
				secretStore[r.id] = r.password
				totalTime += time.Now().UnixMicro() - r.requestStartTs
				// log.Printf("totalTime: %d", totalTime) remove
			case GetCountCommand:
				counter++
				r.responseChannel <- strconv.Itoa(counter)
			case GetStatsCommand:
				s := &Stats{
					TotalNum:    counter,
					AverageTime: float64(totalTime) / float64(counter),
				}
				if counter == 0 {
					s.AverageTime = 0
				}
				sJson, _ := json.Marshal(s)
				r.responseChannel <- string(sJson)
			default:
				log.Fatal("Unknown request type", r.requestType)
			}
		}
	}()

	return inboundRequests
}

// getHashHandler handles the `/hash/{id}` endpoint.
func (s *Server) getHashHandler(w http.ResponseWriter, r *http.Request) {
	// If the server is being termintaed, reject new requests.
	if s.isTerminated {
		fmt.Fprintf(w, "Cannot accept new requests, the server is being terminated...\n")
		return
	}
	m := regexp.MustCompile("^(.*?)/hash/")
	id := m.ReplaceAllString(r.URL.Path, "")
	hashId, err := strconv.Atoi(id)
	if err != nil {
		fmt.Fprintf(w, "Invalid hash id!\n")
		log.Println("Invalid hash id!")
		return
	}

	// Retrieve the stored hashed value of the password for given id.
	resChan := make(chan string)
	s.inboundRequests <- Command{requestType: GetHashCommand, id: hashId, responseChannel: resChan}
	log.Println("Hash retrieved for id: ", id)
	fmt.Fprintf(w, "%s\n", <-resChan)
}

// setHashHandler handles the POST requests to `/hash` endpoint.
func (s *Server) setHashHandler(w http.ResponseWriter, r *http.Request) {
	// If the server is being termintaed, reject new requests.
	if s.isTerminated {
		fmt.Fprintf(w, "Cannot accept new requests, the server is being terminated...\n")
		return
	}
	password := r.FormValue("password")

	// Reject the request if not of type 'POST'.
	if r.Method != http.MethodPost {
		fmt.Fprintf(w, "Only POST methods are supported for `/hash` endpoint!\n")
		log.Println("Rejecting the request as it is not of type 'POST'.")
		return
	}

	// Get the current request counter value and return it to the caller.
	resChan := make(chan string)
	s.inboundRequests <- Command{requestType: GetCountCommand, password: "", id: 0, responseChannel: resChan}
	id, _ := strconv.Atoi(<-resChan)
	fmt.Fprintf(w, "%d\n", id)

	// Push the request to inboundRequests after 5 sec.
	c := &Command{requestType: SetHashCommand, password: password, id: id}
	go func() {
		time.Sleep(PreprocessingDelay * time.Second)
		c.requestStartTs = time.Now().UnixMicro()

		// Perform Sha512 and base64 encode.
		s512 := sha512.Sum512([]byte(c.password))
		s512Str := string(s512[:])
		c.password = b64.StdEncoding.EncodeToString([]byte(s512Str))
		s.inboundRequests <- *c
	}()
}

// statsHandler handles the GET requests to `/stats` endpoint.
func (s *Server) statsHandler(w http.ResponseWriter, r *http.Request) {
	// If the server is being termintaed, reject new requests.
	if s.isTerminated {
		fmt.Fprintf(w, "Cannot accept new requests, the server is being terminated...\n")
		return
	}
	// Reject the request if not of type 'POST'.
	if r.Method != http.MethodGet {
		fmt.Fprintf(w, "Only GET methods are supported for `/stats` endpoint!\n")
		log.Println("Rejecting the request as it is not of type 'GET'.")
		return
	}

	// Get current stats.
	resChan := make(chan string)
	s.inboundRequests <- Command{requestType: GetStatsCommand, responseChannel: resChan}
	resp := <-resChan
	fmt.Fprintf(w, "%s\n", resp)
}

// shutdownHandler handles the `/shutdown` endpoint.
func (s *Server) shutdownHandler(w http.ResponseWriter, r *http.Request) {
	s.isTerminated = true
	fmt.Fprintf(w, "Terminating the server...%d\n", len(s.inboundRequests))

	// Do a graceful shutdown. Wait for pending requests to finish before termintaing.
	time.Sleep(PreprocessingDelay * time.Second)
	go func() {
		for len(s.inboundRequests) > 0 {
			time.Sleep(1 * time.Second)
			log.Printf("Pending inboundRequests: %d", len(s.inboundRequests))
			log.Printf("Waiting for pending requests to finish...")
		}
		os.Exit(0)
	}()
}

var setHashRegex = regexp.MustCompile(`/hash$`)      // to match `/hash` endpoint
var getHashRegex = regexp.MustCompile(`/hash/\d`)    // to match `/hash/{id}` endpoint
var statsRegex = regexp.MustCompile(`/stats$`)       // to match `/stats` endpoint
var shutdownRegex = regexp.MustCompile(`/shutdown$`) // to match `/shutdown` endpoint

// MatchHandlers matches endpoints to their handlers.
func (s *Server) matchHandlers(w http.ResponseWriter, r *http.Request) {
	switch {
	case getHashRegex.MatchString(r.URL.Path):
		s.getHashHandler(w, r)
	case setHashRegex.MatchString(r.URL.Path):
		s.setHashHandler(w, r)
	case statsRegex.MatchString(r.URL.Path):
		s.statsHandler(w, r)
	case shutdownRegex.MatchString(r.URL.Path):
		s.shutdownHandler(w, r)
	default:
		w.Write([]byte("This endopint is not supported by the server. Try ['/hash'|'/hash/{id}'|'/stats'|'/shutdown']\n"))
	}
}

// main starts the server.
func main() {
	server := &Server{inboundRequests: CreatePasswordStore()}
	http.HandleFunc("/", server.matchHandlers)
	http.ListenAndServe(":8090", nil)
}
