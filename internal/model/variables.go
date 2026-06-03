package model

// Variable is a single {{template}} value. Key is the name used as {{Key}} in
// any text field (URL, params, headers, body, auth). A disabled Variable is kept
// but not applied. When Secret is true the value is NOT persisted in the
// committed environment/collection file; it is stored separately in a gitignored
// .env file (see the store package) so credentials stay out of version control.
type Variable struct {
	Key     string `json:"key"`
	Value   string `json:"value,omitempty"`
	Enabled bool   `json:"enabled"`
	Secret  bool   `json:"secret,omitempty"`
}

// Environment is a named set of Variables (e.g. "Local", "Staging", "Prod").
// Exactly one environment is active per collection at a time; its variables take
// precedence over collection-level variables. Environments are persisted in
// sibling files, one per environment, not inside the .yon file.
type Environment struct {
	Name      string     `json:"name"`
	Variables []Variable `json:"variables,omitempty"`
}
