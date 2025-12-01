package main

import "golang.org/x/crypto/acme"
import "golang.org/x/crypto/acme/autocert"
import "context"
import "crypto/tls"
import "fmt"
import "log"
import "net"
import "net/http"
import "net/http/httputil"
import "os"
import "os/signal"
import "sync"
import "syscall"
import "time"

func die(status int, format string, args ...interface {}) {
  fmt.Fprintf(os.Stderr, format + "\n", args...)
  os.Exit(status)
}

func main() {
  if len(os.Args) < 2 {
    die(64, "Usage: %s HOSTNAME...", os.Args[0])
  }

  manager := &autocert.Manager {
    Cache: autocert.DirCache("."),
    HostPolicy: autocert.HostWhitelist(os.Args[1:]...),
    Prompt: autocert.AcceptTOS,
  }

  server := &http.Server {
    ErrorLog: log.New(os.Stdout, "", 0),
    Handler: &httputil.ReverseProxy {
      Director: func(request *http.Request) {
        if host, _, err := net.SplitHostPort(request.Host); err == nil {
          request.Host = host
        }
        request.URL.Scheme, request.URL.Host = "http", request.Host
        if _, ok := request.Header["User-Agent"]; ok == false {
          request.Header.Set("User-Agent", "")
        }
      },
      ErrorLog: log.New(os.Stdout, "", 0),
    },
    TLSConfig: manager.TLSConfig(),
  }

  if url := os.Getenv("ACMEURL"); url != "" {
    manager.Client = &acme.Client {
      DirectoryURL: url,
    }
  }

  terminate := make(chan os.Signal, 1)
  signal.Notify(terminate, syscall.SIGHUP, syscall.SIGINT, syscall.SIGTERM)

  hosts := make(map[string]string)
  for _, name := range os.Args[1:] {
    addresses, err := net.LookupHost(name)
    if err != nil {
      die(1, "Failed to resolve hostname %s", name)
    }
    for _, host := range addresses {
      hosts[host] = host
    }
  }

  wait := new(sync.WaitGroup)
  for host := range hosts {
    listener, err := net.Listen("tcp", net.JoinHostPort(host, "https"))
    if err != nil {
      server.Shutdown(context.Background())
      wait.Wait()
      die(1, "Failed to listen for https on %s", host)
    }
    wait.Add(1)
    go func() {
      server.ServeTLS(listener, "", "")
      wait.Done()
    }()
  }

  go func() {
    for {
      for _, name := range os.Args[1:] {
        conn, err := tls.Dial("tcp", fmt.Sprintf("%s:443", name), nil)
        if err == nil {
          conn.Close()
        }
      }
      time.Sleep(24 * time.Hour)
    }
  }()

  <-terminate
  server.Shutdown(context.Background())
  wait.Wait()
}
