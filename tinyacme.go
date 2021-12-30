package main

import "golang.org/x/crypto/acme/autocert"
import "fmt"
import "log"
import "net"
import "net/http"
import "os"

func die(status int, format string, args ...interface {}) {
  fmt.Fprintf(os.Stderr, format + "\n", args...)
  os.Exit(status)
}

func main() {
  if len(os.Args) < 2 {
    die(64, "Usage: %s HOSTNAME...\n", os.Args[0])
  }

  manager := &autocert.Manager {
    Cache: autocert.DirCache("."),
    HostPolicy: autocert.HostWhitelist(os.Args[1:]...),
    Prompt: autocert.AcceptTOS,
  }

  server := &http.Server {
    ErrorLog: log.New(os.Stderr, "", 0),
    TLSConfig: manager.TLSConfig(),
  }

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

  for host := range hosts {
    listener, err := net.Listen("tcp", net.JoinHostPort(host, "https"))
    if err != nil {
      die(1, "Failed to listen for https on %s", host)
    }
    go server.ServeTLS(listener, "", "")
  }

  for _, name := range os.Args[1:] {
    url := fmt.Sprintf("https://%s/", name)
    if _, err := http.Get(url); err != nil {
      die(1, "Failed sanity check for %s", url)
    }
  }
}
