// Copyright 2015 The Gorilla WebSocket Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// +build ignore

package main

import (
	"io"
	"os"
	"bufio"
	"flag"
	"log"
	"time"
	"context"
	"net"
	"net/http"
    "sync"

	"github.com/gorilla/websocket"
	"github.com/glerchundi/grpc-boomerang/pkg/api"
	"google.golang.org/grpc"
)

var addr = flag.String("addr", "localhost:8080", "http service address")

var upgrader = websocket.Upgrader{} // use default options

func echo(grpcSide, websocketSide net.Conn) func (http.ResponseWriter, *http.Request) {
	return func (w http.ResponseWriter, r *http.Request) {
		c, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			log.Print("upgrade:", err)
			return
		}
		defer c.Close()

		data := make([]byte, 10 * 1024 * 1024)
		var wg sync.WaitGroup

		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				mt, message, err := c.ReadMessage()
				if err != nil {
					log.Println("c.ReadMessage:", err)
					break
				}

				if mt != websocket.BinaryMessage {
					log.Println("mt != websocket.BinaryMessage")
					break
				}

				n, err := websocketSide.Write(message)
				if err != nil {
					log.Println("pipe.Write:", err)
					break
				}

				if len(message) != n {
					log.Printf("whooot! len(data) != n => %d != %d!\n", len(message), n)
					break
				}
			}
		}()

		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				n, err := websocketSide.Read(data)
				if err != nil {
					log.Println("pipe.Read:", err)
					break
				}

				err = c.WriteMessage(websocket.BinaryMessage, data[:n])
				if err != nil {
					log.Println("c.WriteMessage:", err)
					break
				}
			}
		}()

		wg.Wait()
	}
	
}

func main() {
	flag.Parse()
	log.SetFlags(0)

	grpcSide, websocketSide := net.Pipe()

	http.HandleFunc("/echo", echo(grpcSide, websocketSide))
	go func() {
		log.Fatal(http.ListenAndServe(*addr, nil))
	}()

	c, err := grpc.Dial("", []grpc.DialOption{
			grpc.WithInsecure(),
			grpc.WithDialer(func (s string, d time.Duration) (net.Conn, error) {
				return grpcSide, nil
			}),
		}...,
	)
	if err != nil {
		log.Fatal(err.Error())
	}
	client := api.NewApiClient(c)

	r := bufio.NewReader(os.Stdin)
	for {
		l, _, err := r.ReadLine()
		if err != nil {
			log.Fatal(err.Error())
		}		

		go func() {
			streamRequest := &api.HelloStreamRequest{Name:string(l)}
			streamResponse, err := client.HelloStream(context.Background(), streamRequest)
			if err != nil {
				log.Fatalf("%v.HelloStream(_) = _, %v", client, err)
			}

			for {
				streamResponseItem, err := streamResponse.Recv()
				if err == io.EOF {
					break
				}
				if err != nil {
					log.Fatalf("streamResponse.Recv() %v", err)
				}
				log.Println(streamResponseItem)
			}
		}()

		request := &api.HelloRequest{Name:string(l)}
		response, err := client.Hello(context.Background(), request)
		if err != nil {
			log.Println(err.Error())
			continue
		}

		println("response: " + response.GetMessage())
	}
}