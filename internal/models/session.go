package models

import "sync"

type ProgramSession struct {
	InputChan        chan string
	OutputChan       chan ProgramOutput
	Done             chan struct{}
	Cleanup          sync.Once
	DetectedInputOps []InputOperation
}

func NewSession() *ProgramSession {
	return &ProgramSession{
		InputChan:  make(chan string),
		OutputChan: make(chan ProgramOutput),
		Done:       make(chan struct{}),
	}
}

func (s *ProgramSession) Close() {
	s.Cleanup.Do(func() {
		close(s.Done)
		close(s.InputChan)
	})
}
