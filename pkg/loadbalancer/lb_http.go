package loadbalancer

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"time"

	"github.com/plunder-app/kube-vip/pkg/kubevip"
	log "github.com/sirupsen/logrus"
)

func (lb *LBInstance) startHTTP(bindAddress string) error {
	frontEnd := fmt.Sprintf("%s:%d", bindAddress, lb.instance.Port)
	log.Infof("Starting HTTP Load Balancer for service [%s]", frontEnd)

	// Validate the back end URLS
	err := kubevip.ValidateBackEndURLS(&lb.instance.Backends)
	if err != nil {
		return err
	}

	handler := func(w http.ResponseWriter, req *http.Request) {
		// parse the url
		url, _ := url.Parse(lb.instance.ReturnEndpointURL().String())

		// create the reverse proxy
		proxy := httputil.NewSingleHostReverseProxy(url)

		// Update the headers to allow for SSL redirection
		req.URL.Host = url.Host
		req.URL.Scheme = url.Scheme
		// Get remote ip
		remoteIP, _, _ := net.SplitHostPort(req.RemoteAddr)
		req.Header.Set("X-Real-IP", remoteIP)
		req.Header.Set("X-Forwarded-Host", req.Host)
		req.Host = url.Host

		// Print out the response (if debug logging)
		if log.GetLevel() >= log.DebugLevel {
			log.Debugf("Host: %s", req.Host)
			log.Debugf("Request: %s", req.Method)
			log.Debugf("URI: %s", req.RequestURI)

			for key, value := range req.Header {
				log.Debugf("Header: %s, Value: %s", key, value)
			}
		}

		// Note that ServeHttp is non blocking and uses a go routine under the hood
		proxy.ServeHTTP(w, req)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/", handler)
	log.Infof("Starting server listening [%s]", lb.instance.Name)

	server := &http.Server{Addr: frontEnd, Handler: mux}

	go func() error {
		if err := server.ListenAndServe(); err != nil {
			return err
		}
		return nil
	}()

	// If the stop channel is closed then the server will be gracefully shut down
	<-lb.stop
	log.Infof("Stopping the load balancer [%s] bound to [%s] with 5sec timeout", lb.instance.Name, frontEnd)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := server.Shutdown(ctx); err != nil {
		return err
	}
	close(lb.stopped)
	return nil
}

//StartHTTP - begins the HTTP load balancer
func StartHTTP(lb *kubevip.LoadBalancer, address string) error {
	frontEnd := fmt.Sprintf("%s:%d", address, lb.Port)
	log.Infof("Starting HTTP Load Balancer for service [%s]", frontEnd)

	// Validate the back end URLS
	err := kubevip.ValidateBackEndURLS(&lb.Backends)
	if err != nil {
		return err
	}

	handler := func(w http.ResponseWriter, req *http.Request) {
		// parse the url
		url, _ := url.Parse(lb.ReturnEndpointURL().String())

		// create the reverse proxy
		proxy := httputil.NewSingleHostReverseProxy(url)

		// Update the headers to allow for SSL redirection
		req.URL.Host = url.Host
		req.URL.Scheme = url.Scheme
		// Get remote ip
		remoteIP, _, _ := net.SplitHostPort(req.RemoteAddr)
		req.Header.Set("X-Real-IP", remoteIP)
		req.Header.Set("X-Forwarded-Host", req.Host)
		req.Host = url.Host

		// Print out the response (if debug logging)
		if log.GetLevel() >= log.DebugLevel {
			log.Debugf("Host: %s", req.Host)
			log.Debugf("Request: %s", req.Method)
			log.Debugf("URI: %s", req.RequestURI)

			for key, value := range req.Header {
				log.Debugf("Header: %s, Value: %s", key, value)
			}
		}

		// Note that ServeHttp is non blocking and uses a go routine under the hood
		proxy.ServeHTTP(w, req)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/", handler)
	log.Infof("Starting server listening [%s]", lb.Name)
	http.ListenAndServe(frontEnd, mux)
	// Should never get here
	return nil
}
