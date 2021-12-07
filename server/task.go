package server

import "sync"

type Task struct {
	Url string `json:"url"`

	Caption      bool   `json:"caption"`
	Cookie       string `json:"cookie"`
	Refer        string `json:"refer"`
	StreamFormat string `json:"stream-format"`

	Status string   `json:"status"`
	Errors []string `json:"errors"`
}

type AsyncTask struct {
	task  Task
	mutex sync.RWMutex
}

func (at *AsyncTask) setStatus(s string) {
	at.mutex.Lock()
	at.task.Status = s
	at.mutex.Unlock()
}

func (at *AsyncTask) appendError(e string) {
	at.mutex.Lock()
	at.task.Errors = append(at.task.Errors, e)
	at.mutex.Unlock()
}
