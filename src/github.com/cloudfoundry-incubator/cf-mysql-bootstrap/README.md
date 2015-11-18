# cf-mysql-bootstrap
Auto-bootstrap errand for cf-mysql-release

#### Regenerate http.Handler fake
```
GOPATH=~/go-src counterfeiter -o fakes/fake_handler.go ~/go-src/src/net/http/server.go Handler
```
