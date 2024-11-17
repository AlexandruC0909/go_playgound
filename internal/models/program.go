package models

type ProgramOutput struct {
	Output          string `json:"output,omitempty"`
	Error           string `json:"error,omitempty"`
	WaitingForInput bool   `json:"waitingForInput"`
	Done            bool   `json:"done"`
}

type InputRequest struct {
	Input string `json:"input"`
}

type InputOperation struct {
	Line    int
	Type    string
	Package string
}

type CodeRequest struct {
	Code string `json:"code"`
}

type SessionResponse struct {
	SessionID uint64 `json:"sessionId"`
}
