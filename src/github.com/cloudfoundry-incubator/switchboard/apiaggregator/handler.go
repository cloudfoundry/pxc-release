package apiaggregator

import (
	"bytes"
	"fmt"
	"html/template"
	"log/slog"
	"net/http"
	"os"

	"github.com/cloudfoundry-incubator/switchboard/api/middleware"
	"github.com/cloudfoundry-incubator/switchboard/config"
)

func NewHandler(
	logger *slog.Logger,
	apiConfig config.API,
) http.Handler {
	mux := http.NewServeMux()

	mux.Handle("/", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t, err := template.New("proxySpringboard").Parse(
			`
<html><head><title>Proxy Springboard</title></head><body>
{{ range . }}
<p><a href="http://{{ . }}">{{ . }}</a></p>
{{ end }}
</body></html>
`,
		)
		if err != nil {
			fmt.Fprintf(os.Stderr, "%v", err)
		}

		buf := new(bytes.Buffer)
		err = t.Execute(buf, apiConfig.ProxyURIs)
		if err != nil {
			panic(err)
		}

		fmt.Fprint(w, buf)
	}))

	return middleware.Chain{
		middleware.NewPanicRecovery(logger),
		middleware.NewLogger(logger, "/v0"),
		middleware.NewHttpsEnforcer(apiConfig.ForceHttps),
		middleware.NewBasicAuth(apiConfig.Username, apiConfig.Password),
	}.Wrap(mux)
}
