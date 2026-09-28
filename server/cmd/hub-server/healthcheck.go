package main

import (
	"fmt"
	"net"
	"net/http"
	"time"
)

const healthcheckTimeout = 3 * time.Second

// runHealthcheck asks the running server for its health over loopback. The
// container image has no shell or curl, so the binary probes itself.
func runHealthcheck(address string) error {
	host, port, err := net.SplitHostPort(address)
	if err != nil {
		return fmt.Errorf("parse HUB_ADDR %q: %w", address, err)
	}
	if host == "" || host == "0.0.0.0" {
		host = "127.0.0.1"
	}

	client := http.Client{Timeout: healthcheckTimeout}
	response, err := client.Get("http://" + net.JoinHostPort(host, port) + "/v1/health")
	if err != nil {
		return err
	}
	defer response.Body.Close()

	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("health returned %d", response.StatusCode)
	}
	return nil
}
