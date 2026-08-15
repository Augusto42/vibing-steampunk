// Command vsp-mock-sap starts a local, synthetic SAP ADT/ZADT_VSP protocol
// simulator. It is intended for VSP development and integration testing only.
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/oisee/vibing-steampunk/internal/mocksap"
)

func main() {
	listen := flag.String("listen", "127.0.0.1:50080", "listen address (localhost by default)")
	username := flag.String("username", mocksap.Username, "synthetic Basic Auth username")
	password := flag.String("password", mocksap.Password, "synthetic Basic Auth password")
	syntaxError := flag.Bool("syntax-error", false, "inject a syntax error before include writes")
	activationError := flag.Bool("activation-error", false, "inject an activation error after include writes")
	dynproSubrc := flag.Int("dynpro-subrc", 0, "return code for synthetic RPY_DYNPRO_READ")
	flag.Parse()

	handler := mocksap.New(mocksap.Options{
		Username:        *username,
		Password:        *password,
		SyntaxError:     *syntaxError,
		ActivationError: *activationError,
		DynproSubrc:     *dynproSubrc,
	})
	listener, err := net.Listen("tcp", *listen)
	if err != nil {
		log.Fatalf("listen: %v", err)
	}

	server := &http.Server{
		Handler:           handler,
		ReadHeaderTimeout: 5 * time.Second,
		IdleTimeout:       30 * time.Second,
	}

	shutdown := make(chan os.Signal, 1)
	signal.Notify(shutdown, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-shutdown
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = server.Shutdown(ctx)
	}()

	fmt.Printf("VSP synthetic SAP protocol simulator listening at http://%s\n", listener.Addr())
	fmt.Printf("client=%s user=%s fixtures=%s,%s,%s/%s\n", mocksap.Client, *username, mocksap.EnhancementName, mocksap.IncludeName, mocksap.ProgramName, mocksap.ScreenNumber)
	if err := server.Serve(listener); err != nil && err != http.ErrServerClosed {
		log.Fatalf("serve: %v", err)
	}
}
