package main

import "golang.org/x/crypto/acme"
import "golang.org/x/crypto/acme/autocert"
import "context"
import "fmt"
import "log"
import "net"
import "net/http"
import "net/http/httputil"
import "os"
import "os/signal"
import "strings"
import "sync"
import "syscall"

func die(status int, format string, args ...interface {}) {
  fmt.Fprintf(os.Stderr, format + "\n", args...)
  os.Exit(status)
}

func redirect(writer http.ResponseWriter, request *http.Request) {
  if host, _, err := net.SplitHostPort(request.Host); err == nil {
    request.Host = host
  }
  request.URL.Scheme, request.URL.Host = "https", request.Host
  http.Redirect(writer, request, request.URL.String(), 301)
}

func main() {
  if len(os.Args) < 3 {
    die(64, "Usage: %s SOCKET HOSTNAME...", os.Args[0])
  }

  manager := &autocert.Manager {
    Cache: autocert.DirCache("."),
    HostPolicy: autocert.HostWhitelist(os.Args[1:]...),
    Prompt: autocert.AcceptTOS,
  }

  proxy := &httputil.ReverseProxy {
    Director: func(request *http.Request) {
      request.URL.Scheme, request.URL.Host = "http", request.Host
      if _, ok := request.Header["User-Agent"]; ok == false {
        request.Header.Set("User-Agent", "")
      }
    },
    ErrorLog: log.New(os.Stdout, "", 0),
  }

  server := &http.Server {
    ErrorLog: log.New(os.Stdout, "", 0),
    Handler: proxy,
    TLSConfig: manager.TLSConfig(),
  }

  if url := os.Getenv("ACMEURL"); url != "" {
    manager.Client = &acme.Client {
      DirectoryURL: url,
    }
  }

  if strings.ContainsRune(os.Args[1], '/') {
    proxy.Transport = &http.Transport {
      DialContext: func(_ context.Context, _, _ string) (net.Conn, error) {
        return net.Dial("unix", os.Args[1])
      },
    }
  } else if _, _, err := net.SplitHostPort(os.Args[1]); err == nil {
    proxy.Transport = &http.Transport {
      DialContext: func(_ context.Context, _, _ string) (net.Conn, error) {
        return net.Dial("tcp", os.Args[1])
      },
    }
  } else {
    die(1, "Invalid socket address: %s", os.Args[1])
  }

  terminate := make(chan os.Signal, 1)
  signal.Notify(terminate, syscall.SIGHUP, syscall.SIGINT, syscall.SIGTERM)

  hosts := make(map[string]string)
  for _, name := range os.Args[2:] {
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

  for host := range hosts {
    listener, err := net.Listen("tcp", net.JoinHostPort(host, "http"))
    if err != nil {
      server.Shutdown(context.Background())
      wait.Wait()
      die(1, "Failed to listen for http on %s", host)
    }
    go http.Serve(listener, http.HandlerFunc(redirect))
  }

  <-terminate
  server.Shutdown(context.Background())
  wait.Wait()
}
