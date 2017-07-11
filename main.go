package main

import (
	"bufio"
	"flag"
	"io"
	"log"
	"net"
	"strconv"
	"strings"

	"sync"

	"encoding/gob"

	"github.com/pkg/errors"
)

func init() {
	log.SetFlags(log.Lshortfile)
}

func main() {
	connect := flag.String("connect", "", "IP address of process to join. If empty, go into listen mode.")
	flag.Parse()
	if *connect != "" {
		err := client(*connect)
		if err != nil {
			log.Println("Error:", errors.WithStack(err))
		}
		log.Println("Client done.")
		return
	}
	err := server()
	if err != nil {
		log.Println("Error:", errors.WithStack(err))
	}
	log.Println("Server done.")
}

type dataObject struct {
	N int
	S string
	M map[string]int
	P []byte
	C *dataObject
}

const (
	Port = ":61000"
)

func Open(address string) (*bufio.ReadWriter, error) {
	log.Println("Dial", address)
	conn, err := net.Dial("tcp", address)
	if err != nil {
		return nil, errors.Wrap(err, "Dialing "+address+" failed")
	}
	return bufio.NewReadWriter(bufio.NewReader(conn), bufio.NewWriter(conn)), nil
}

type HandleFunc func(*bufio.ReadWriter)

type Endpoint struct {
	listener net.Listener
	handler  map[string]HandleFunc
	m        sync.RWMutex
}

func NewEndpoint() *Endpoint {
	return &Endpoint{
		handler: map[string]HandleFunc{},
	}
}

func (e *Endpoint) AddHandleFunc(name string, f HandleFunc) {
	e.m.Lock()
	e.handler[name] = f
	e.m.Unlock()
}

func (e *Endpoint) Listen() error {
	var err error
	e.listener, err = net.Listen("tcp", Port)
	if err != nil {
		return errors.Wrap(err, "Unable to listen on "+e.listener.Addr().String()+"\n")
	}
	log.Println("Listen on", e.listener.Addr().String())
	for {
		log.Println("Accept a connection request.")
		conn, err := e.listener.Accept()
		if err != nil {
			log.Println("Failed accepting a connection request:", err)
			continue
		}
		log.Println("Handle incoming message,")
		go e.handleMessages(conn)
	}
}

func (e *Endpoint) handleMessages(conn net.Conn) {
	rw := bufio.NewReadWriter(bufio.NewReader(conn), bufio.NewWriter(conn))
	defer conn.Close()
	for {
		log.Print("Receive command '")
		cmd, err := rw.ReadString('\n')
		switch {
		case err == io.EOF:
			log.Println("Reached EOF - close this connection. \n  ---")
			return
		case err != nil:
			log.Println("\nError reading command. Got: '"+cmd+"'\n", err)
			return
		}
		cmd = strings.Trim(cmd, "\n ")
		log.Println(cmd + "'")
		e.m.RLock()
		handleCommand, ok := e.handler[cmd]
		e.m.RUnlock()
		if !ok {
			log.Println("Command '" + cmd + "' is not registered.")
			return
		}
		handleCommand(rw)
	}
}

func handleStrings(rw *bufio.ReadWriter) {
	log.Print("Receive STRING message:")
	s, err := rw.ReadString('\n')
	if err != nil {
		log.Println("Cannot read from connection. \n", err)
	}
	s = strings.Trim(s, "\n ")
	log.Println(s)
	_, err = rw.WriteString("Thank you. \n")
	if err != nil {
		log.Println("Cannot write to connection. \n", err)
	}
	err = rw.Flush()
	if err != nil {
		log.Println("flush failed.", err)
	}
}

func handleGob(rw *bufio.ReadWriter) {
	log.Print("Receive GOB data:")
	var data dataObject
	decoder := gob.NewDecoder(rw)
	err := decoder.Decode(&data)
	if err != nil {
		log.Println("Error decoding GOB data:", err)
		return
	}
	log.Printf("Outer dataObject struct: \n%#v\n", data)
	log.Printf("inner dataObject sruct: \n%#v\n", data.C)
}

func client(ip string) error {
	testStruct := dataObject{
		N: 23,
		S: "String Data",
		M: map[string]int{"One": 1, "Two": 2, "Three": 3},
		P: []byte("abc"),
		C: &dataObject{
			N: 245,
			S: "Recursive Structs No Problem",
			M: map[string]int{"01": 1, "02": 2, "03": 3},
		},
	}
	rw, err := Open(ip + Port)
	if err != nil {
		return errors.Wrap(err, "Client: Failed to open connection to "+ip+Port)
	}
	log.Println("Send the string request.")
	n, err := rw.WriteString("STRING\n")
	if err != nil {
		return errors.Wrap(err, "could not send the STRING Request ("+strconv.Itoa(n)+" bytes written)")
	}
	n, err = rw.WriteString("Additional data. \n")
	if err != nil {
		return errors.Wrap(err, "Could not send additional STRING data ("+strconv.Itoa(n)+" bytes written)")
	}
	log.Println("Flush the buffer.")
	err = rw.Flush()
	if err != nil {
		return errors.Wrap(err, "Flush Failed.")
	}
	log.Println("Read the Reply.")
	response, err := rw.ReadString('\n')
	if err != nil {
		return errors.Wrap(err, "Client: Failed to read the reply: '"+response+"'")
	}
	log.Println("STRING request: got a response:", response)
	log.Println("Send a struct as GOB:")
	log.Printf("Outer dataObject struct: \n%#v\n", testStruct)
	log.Printf("Inner dataObject struct: \n%#v\n", testStruct.C)
	encode := gob.NewEncoder(rw)
	n, err = rw.WriteString("GOB\n")
	if err != nil {
		return errors.Wrap(err, "Could not write GOB data ("+strconv.Itoa(n)+" bytes written)")
	}
	err = encode.Encode(testStruct)
	if err != nil {
		return errors.Wrapf(err, "Encode failed for struct: %#v", testStruct)
	}
	err = rw.Flush()
	if err != nil {
		return errors.Wrap(err, "Flush failed.")
	}
	return nil
}

func server() error {
	endpoint := NewEndpoint()
	endpoint.AddHandleFunc("STRING", handleStrings)
	endpoint.AddHandleFunc("GOB", handleGob)
	return endpoint.Listen()
}
