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

func (t *AsyncTask) setStatus(s string) {
	t.mutex.Lock()
	t.task.Status = s
	t.mutex.Unlock()
}

func (t *AsyncTask) appendError(e string) {
	t.mutex.Lock()
	t.task.Errors = append(t.task.Errors, e)
	t.mutex.Unlock()
}
