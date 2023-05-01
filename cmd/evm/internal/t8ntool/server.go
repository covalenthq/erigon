package t8ntool

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"io/ioutil"
	"net/http"
	"os"
	"strconv"
	"syscall"
	"time"

	"github.com/ledgerwatch/log/v3"

	"github.com/urfave/cli/v2"
)

const (
	OK      = "OK"
	BAD     = "BAD"
	TIMEOUT = "TIMEOUT"
)

type EvmServer struct {
	mux *http.ServeMux
}

type State struct {
	status string
}

func NewState() *State {
	return &State{status: OK}
}

func (eserv *EvmServer) StartServer(ctx *cli.Context, port int64) error {
	httpState := NewState()
	eserv.mux = http.NewServeMux()
	eserv.mux.Handle("/process", eserv.recoveryWrapper(eserv.processHttpHandler(ctx)))
	eserv.mux.Handle("/health", eserv.recoveryWrapper(eserv.healthHttpHandler(ctx, httpState)))
	eserv.mux.Handle("/sabotage", eserv.recoveryWrapper(eserv.sabotageHttpHandler(ctx, httpState)))
	eserv.mux.Handle("/recover", eserv.recoveryWrapper(eserv.recoverHttpHandler(ctx, httpState)))
	eserv.mux.Handle("/timeout", eserv.recoveryWrapper(eserv.timeoutHttpHandler(ctx, httpState)))
	log.Info("Listening", "port", port)
	err := http.ListenAndServe(":"+strconv.Itoa(int(port)), eserv.mux)
	if err != nil {
		log.Info("error listening and serving on TCP network", "error", err)
		return err
	}

	return nil
}

func newFlagSet(app *cli.App) *flag.FlagSet {
	flagSet := flag.FlagSet{}
	for _, command := range app.Commands {
		if command.Name != "transition" { // hardcoded
			continue
		}
		for _, flag := range command.Flags {
			flag.Apply(&flagSet)
		}
	}
	return &flagSet
}

func (eserv *EvmServer) processHttpHandler(ctx *cli.Context) http.Handler {
	fn := func(w http.ResponseWriter, r *http.Request) {
		contents, err := eserv.readContentFromRequest(r)
		if err != nil {
			eserv.respondError(w, err)
			return
		}
		tmpFile, err := os.CreateTemp(os.TempDir(), "*")
		if err != nil {
			eserv.respondError(w, err)
			return
		}
		log.Info("input file at", "loc", tmpFile.Name())

		defer syscall.Unlink(tmpFile.Name())
		defer tmpFile.Close()

		if err := ioutil.WriteFile(tmpFile.Name(), contents, fs.ModePerm|fs.ModeExclusive|fs.ModeTemporary); err != nil {
			eserv.respondError(w, err)
			return
		}

		ctxcp := cli.NewContext(ctx.App, newFlagSet(ctx.App), nil)
		if err = ctxcp.Set(InputReplicaFlag.Name, tmpFile.Name()); err != nil {
			eserv.respondError(w, err)
			return
		}

		tmpFile, err = os.CreateTemp(os.TempDir(), "*")
		if err != nil {
			eserv.respondError(w, err)
			return
		}
		log.Info("output file at: ", "loc", tmpFile.Name())

		defer syscall.Unlink(tmpFile.Name())
		defer tmpFile.Close()

		ctxcp.Set(OutputBlockResultFlag.Name, tmpFile.Name())
		ctxcp.Set(OutputAllocFlag.Name, "")
		ctxcp.Set(OutputResultFlag.Name, "")
		if err := execute(ctxcp); err != nil {
			eserv.respondError(w, err)
			return
		}

		contents, err = ioutil.ReadFile(tmpFile.Name())
		if err != nil {
			eserv.respondError(w, err)
			return
		}
		w.Write(contents)
	}

	return http.HandlerFunc(fn)
}

func (eserv *EvmServer) recoveryWrapper(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			r := recover()
			if r != nil {
				var err error
				switch t := r.(type) {
				case string:
					err = errors.New(t)
				case error:
					err = t
				default:
					err = errors.New("unknown error")
				}
				http.Error(w, err.Error(), http.StatusInternalServerError)
			}
		}()
		h.ServeHTTP(w, r)
	})
}

func (eserv *EvmServer) readContentFromRequest(r *http.Request) ([]byte, error) {
	mreader, err := r.MultipartReader()
	if err != nil {
		return []byte{}, err
	}

	var contents []byte
	//var contents string = ""

	for {
		part, err := mreader.NextPart()
		if err == io.EOF {
			break
		}

		pcontents, err := ioutil.ReadAll(part)
		if err != nil {
			return []byte{}, err
		}

		contents = append(contents, pcontents...)
	}

	return contents, nil
}

func (eserv *EvmServer) respondError(w http.ResponseWriter, err error) {
	err_str := fmt.Sprintf("{\"error\": \"%s\"}", err)
	w.WriteHeader(http.StatusInternalServerError)
	_, err = w.Write([]byte(err_str))
	if err != nil {
		log.Error("error writing data to connection", "error", err)
	}
}

func (eserv *EvmServer) healthHttpHandler(ctx *cli.Context, s *State) http.Handler {
	// Check the health of the server and return a status code accordingly
	fn := func(w http.ResponseWriter, r *http.Request) {
		log.Info("Received /health request:", "source=", r.RemoteAddr, "status=", s.status)
		switch s.status {
		case OK:
			io.WriteString(w, "I'm healthy")
			return
		case BAD:
			http.Error(w, "Internal Error", 500)
			return
		case TIMEOUT:
			time.Sleep(30 * time.Second)
			return
		default:
			io.WriteString(w, "UNKNOWN")
			return
		}
	}
	return http.HandlerFunc(fn)
}

func (eserv *EvmServer) sabotageHttpHandler(ctx *cli.Context, s *State) http.Handler {
	fn := func(w http.ResponseWriter, r *http.Request) {
		s.status = BAD
		io.WriteString(w, "Sabotage ON")
	}
	return http.HandlerFunc(fn)
}

func (eserv *EvmServer) recoverHttpHandler(ctx *cli.Context, s *State) http.Handler {
	fn := func(w http.ResponseWriter, r *http.Request) {
		s.status = OK
		io.WriteString(w, "Recovered.")
	}
	return http.HandlerFunc(fn)
}

func (eserv *EvmServer) timeoutHttpHandler(ctx *cli.Context, s *State) http.Handler {
	fn := func(w http.ResponseWriter, r *http.Request) {
		s.status = TIMEOUT
		io.WriteString(w, "Configured to timeout.")
	}
	return http.HandlerFunc(fn)
}
