package server

import (
	"container/list"
	"net/http"
	"os"

	"github.com/gin-gonic/gin"
	"github.com/iawia002/annie/downloader"
	"github.com/iawia002/annie/extractors"
	"github.com/iawia002/annie/extractors/types"
	"github.com/urfave/cli/v2"
)

type Task struct {
	Url          string  `json:"url"`
	Cookie       string  `json:"cookie"`
	StreamFormat string  `json:"stream-format"`
	Status       string  `json:"status"`
	Errors       []error `json:"errors"`
}

type Server struct {
	outputPath string
	host       string
	port       string
	token      string

	tasks   *list.List // *Task
	history *list.List // Task
}

func New(c *cli.Context) *Server {
	host := c.String("host")
	if host == "" {
		host = os.Getenv("ANNIE_HOST")
	}
	port := c.String("port")
	if port == "" {
		port = os.Getenv("ANNIE_PORT")
	}
	if port == "" {
		port = "8080"
	}
	token := c.String("token")
	if token == "" {
		token = os.Getenv("TOKEN")
	}
	server := &Server{
		outputPath: c.String("output-path"),
		host:       host,
		port:       port,
		token:      token,
		tasks:      list.New(),
		history:    list.New(),
	}
	return server
}

func (s *Server) Run() {
	router := gin.Default()
	router.POST("/download", s.postDownload)
	router.GET("/tasks", s.getTasks)
	router.GET("/history", s.getHistory)
	router.Run(s.host + ":" + s.port)
}

func (s *Server) postDownload(c *gin.Context) {
	if c.Query("token") != s.token {
		c.Status(http.StatusForbidden)
	}

	var task Task
	if c.BindJSON(&task) != nil || task.Url == "" {
		c.Status(http.StatusBadRequest)
		return
	}
	task.Status = "Created"
	go s.download(task)
	c.IndentedJSON(http.StatusCreated, task)
}

func (s *Server) getTasks(c *gin.Context) {
	if c.Query("token") != s.token {
		c.Status(http.StatusForbidden)
	}

	tasks := []Task{}
	for e := s.tasks.Front(); e != nil; e = e.Next() {
		tasks = append(tasks, *e.Value.(*Task))
	}
	c.IndentedJSON(http.StatusOK, tasks)
}

func (s *Server) getHistory(c *gin.Context) {
	if c.Query("token") != s.token {
		c.Status(http.StatusForbidden)
	}

	tasks := []Task{}
	for e := s.history.Front(); e != nil; e = e.Next() {
		tasks = append(tasks, e.Value.(Task))
	}
	c.IndentedJSON(http.StatusOK, tasks)
}

func (s *Server) download(t Task) {
	element := s.tasks.PushBack(&t)
	defer s.finish(element)
	t.Status = "Extracting"
	data, err := extractors.Extract(t.Url, types.Options{
		Cookie: t.Cookie,
	})
	if err != nil {
		t.Errors = append(t.Errors, err)
		t.Status = "Failed"
		return
	}
	t.Status = "Downloading"
	d := downloader.New(downloader.Options{
		OutputPath: s.outputPath,
		Stream:     t.StreamFormat,
	})
	for _, item := range data {
		if item.Err != nil {
			t.Errors = append(t.Errors, err)
			continue
		}
		d.Download(item)
	}
	t.Status = "Done"
}

func (s *Server) finish(e *list.Element) {
	s.tasks.Remove(e)
	if s.history.Len() == 10 {
		s.history.Remove(s.history.Front())
	}
	s.history.PushBack(*e.Value.(*Task))
}
