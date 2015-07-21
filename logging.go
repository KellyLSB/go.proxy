package proxy

import (
	"os"

	"github.com/op/go-logging"
)

var log = logging.MustGetLogger("proxy")

var logFormat = logging.MustStringFormatter(
	"%{color}%{time:15:04:05.000} %{shortfunc:10s} ▶ " +
		"%{level:.4s} %{id:03x}%{color:reset} %{message}",
)

func init() {
	backendStdout := logging.NewBackendFormatter(
		logging.NewLogBackend(os.Stdout, "", 0),
		logFormat,
	)

	backendStderr := logging.AddModuleLevel(
		logging.NewBackendFormatter(
			logging.NewLogBackend(os.Stderr, "", 0),
			logFormat,
		),
	)

	backendStderr.SetLevel(logging.ERROR, "")

	logging.SetBackend(backendStdout, backendStderr)
}
