package main

import (
    "io"
    "os"
    "fmt"
    "bufio"
    "net/http"
    "strings"
    "text/template"
    "code.google.com/p/go.net/websocket"
    "code.google.com/p/go.crypto/ssh"
)

var wsChannel chan string
var wscon *websocket.Conn = nil

func wsHandler(ws *websocket.Conn) {
    wscon = ws

    for {
        var message string
        err := websocket.Message.Receive(ws, &message)
        if err != nil {
            break
        }
        wsChannel <-message
    }
    ws.Close()
}

var t = template.Must(template.ParseFiles("t.html"))

func tHandler(res http.ResponseWriter, req *http.Request) {
    t.Execute(res, req.Host)
}

func cssHandler(res http.ResponseWriter, req *http.Request) {
    res.Header().Set("Content-Type", "text/css")
    fmt.Fprintf(res, "html { overflow: hidden; } body { overflow: hidden; padding: 0; margin: 0; width: 100%; height: 100%; } #output { position: absolute; top: 0.5em; left: 0.5em; right: 0.5em; bottom: 3em; overflow: auto; } #input { position: absolute; bottom: 0.5em; }");
}

func jsHandler(res http.ResponseWriter, req *http.Request) {
    fmt.Fprintf(res, "var wsc; function output(data) { var div = document.getElementById(\"output\"); div.innerText += data; div.scrollTop = div.scrollHeight - div.clientHeight; } function send() { if (wsc) { input = document.getElementById(\"input_text\"); output(\"Sending \" + input.value); wsc.send(input.value); input.value = \"\";} } function sendOnEnter(event) { if (event.charCode == 13 || event.which == 13) send(); } (function() { if (window.WebSocket != undefined) { wsc = new WebSocket(\"ws://" + req.Host + "/ws\"); wsc.onopen = function() { output(\"Connected\"); }; wsc.onmessage = function(ev) { output(ev.data); }; wsc.onclose = function() { output(\"Disconnected\"); }; } })();");
}

func httpServer() {
    http.HandleFunc("/", tHandler)
    http.HandleFunc("/givemesomecss", cssHandler)
    http.HandleFunc("/givemesomejs", jsHandler)
    http.Handle("/ws", websocket.Handler(wsHandler))
    http.ListenAndServe(":8080", nil)
}

type password string

func (p password) Password(user string) (string, error) {
    return string(p), nil
}

var ccout chan *io.Reader
var ccerr chan *io.Reader
var ccin chan *io.WriteCloser

func doSSH(c chan *ssh.Session, sship string, username string, pw password) {
    config := &ssh.ClientConfig{
        User: username,
        Auth: []ssh.ClientAuth{
            ssh.ClientAuthPassword(pw),
        },
    }
    client, err := ssh.Dial("tcp", sship, config)
    if err != nil {
        panic("Failed to dial: " + err.Error())
    }

    session, err := client.NewSession()
    if err != nil {
        panic("Failed to create session: " + err.Error())
    }
    c <- session

    stdout, err := session.StdoutPipe()
    if err != nil {
        fmt.Println(err)
    }
    ccout <- &stdout

    stderr, err := session.StderrPipe()
    if err != nil {
        fmt.Println(err)
    }
    ccerr <- &stderr

    stdin, err := session.StdinPipe()
    if err != nil {
        fmt.Println(err)
    }
    ccin <- &stdin

    if err := session.Shell(); err != nil {
        fmt.Println(err)
    }
}

func readToChannel(rh *io.Reader, rc chan []byte) {
    for {
        var f []byte
        f = make([]byte, 1024)
        bytes, err := (*rh).Read(f);
        if err == nil {
            fmt.Printf("\nRead %d bytes (sending to channel now)\n", bytes)
            rc <- f[:bytes]
        } else {
            fmt.Printf("Err Reading - Literacy Fail")
        }
    }
}

func main() {
    wsChannel = make(chan string, 10)
    go httpServer()

    c := make(chan *ssh.Session, 10)
    ccout = make(chan *io.Reader, 10)
    ccerr = make(chan *io.Reader, 10)
    ccin = make(chan *io.WriteCloser, 10)

    if len(os.Args) < 4 {
        fmt.Printf("Usage: t1 <ip:port> <user> <password>")
        return
    }

    var ip = os.Args[1];
    var user = os.Args[2];
    var pass = os.Args[3];
    go doSSH(c, ip, user, password(pass))
    session := <- c
    fmt.Printf("Session: %s\n\n", session)
    cout := *(<- ccout)
    cerr := *(<- ccerr)
    cin := *(<- ccin)

    coutReadChannel := make(chan []byte, 10)
    cerrReadChannel := make(chan []byte, 10)
    go readToChannel(&cout, coutReadChannel)
    go readToChannel(&cerr, cerrReadChannel)

    go func() {
        for {
            select {
                case data := <-coutReadChannel:
                    fmt.Printf("%s", strings.TrimSpace(string(data)))
                    if wscon != nil {
                        websocket.Message.Send(wscon, string(data) + "\n")
                    }
                case data := <-cerrReadChannel:
                    fmt.Printf("%s", strings.TrimSpace(string(data)))
                    if wscon != nil {
                        websocket.Message.Send(wscon, strings.TrimSpace(string(data)) + "\n")
                    }
            }
        }
    }()

    go func() {
        for {
            sendThis := <-wsChannel
            cin.Write([]byte(sendThis + "\n"))
        }
    }()

    reader := bufio.NewReader(os.Stdin)
    for {
        str, err := reader.ReadString('\n')
        if err != nil {
            fmt.Printf("Got some error dawg: %s", err)
        } else {
            str = strings.TrimSpace(str)
            if str == "exit" {
                break
            } else {
                fmt.Printf("ReadLine: %s", str)
                cin.Write([]byte(str + "\n"))
            }
        }
    }
}
