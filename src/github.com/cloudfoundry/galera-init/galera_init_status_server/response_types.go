package galera_init_status_server

// ReadinessResponse is the JSON body for GET / and GET /status.
type ReadinessResponse struct {
	Ready bool         `json:"ready"`
	Phase string       `json:"phase"`
	Mode  string       `json:"mode,omitempty"`
	Error *ErrorObject `json:"error,omitempty"`
}

// ErrorObject is included when the service has failed a bootstrap/start step.
type ErrorObject struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

// AckResponse is returned for POST /start and POST /stop. Error is set when ok is false.
type AckResponse struct {
	OK    bool         `json:"ok"`
	Error *ErrorObject `json:"error,omitempty"`
}
