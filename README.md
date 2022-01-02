# README

A server to support **/hash**, **/hash/{id}**, **/stats**, **/shutdown** endpoints, implemented in Go.

## How to run

Clone the repository and run:

```
go run main.go
```

## How to test

First, run the server from terminal using above command.  Curl, ab can be used to make calls to the server as follow:

### /hash call (Must be POST)
```
curl -X POST localhost:8090/hash -d password="myPassword"
```
or, create a `postdata` file with content: `password=abcdefg`, then run:
```
ab -n 100 -c 10 -v 4 -T application/x-www-form-urlencoded -p ./postdata http://localhost:8090/hash
```

### /hash/{id} call
```
curl localhost:8090/hash/1
```

### /stats call (Must be GET)
```
curl -X GET localhost:8090/stats
```

### /shutdown call
```
curl localhost:8090/shutdown
```


## Instructions

* Uses **Channel** to support concurrent requests.
* /hash endpoint waits for **5 seconds** before processing the request.
* A buffered channel of capacity is **200** is used for proccessing incoming requests.
This can be changed using **'ChannelCapacity'** config.
* /stats endpoint returns the total number of requests and average time in **microseconds** required to process each request.
* /shutdown endpoint does a graceful shutdown, waits for **'PreprocessingDelay'** for any pending requests.


Cheers!