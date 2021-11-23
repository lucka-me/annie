package server

import (
	"container/list"
	"net/http"
	"os"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/iawia002/annie/downloader"
	"github.com/iawia002/annie/extractors"
	"github.com/iawia002/annie/extractors/types"
	"github.com/iawia002/annie/request"
	"github.com/urfave/cli/v2"
)

type Task struct {
	Url string `json:"url"`

	Caption      bool   `json:"caption"`
	Cookie       string `json:"cookie"`
	Refer        string `json:"refer"`
	StreamFormat string `json:"stream-format"`

	Status string   `json:"status"`
	Errors []string `json:"errors"`
}

type Server struct {
	chunkSizeMB uint
	debug       bool
	multiThread bool
	outputPath  string
	retryTimes  uint

	host  string
	port  string
	token string

	tasks   *list.List // *Task
	history *list.List // Task
}

func New(c *cli.Context) *Server {
	enableDebug := boolFrom(c, "debug", "ANNIE_DEBUG")
	request.SetOptions(request.Options{
		Debug:  enableDebug,
		Silent: boolFrom(c, "silent", "ANNIE_SILENT"),
	})
	server := &Server{
		chunkSizeMB: uintFrom(c, "chunk-size", "ANNIE_CHUNK_SIZE"),
		debug:       enableDebug,
		multiThread: boolFrom(c, "multi-thread", "ANNIE_MULTI_THREAD"),
		outputPath:  stringFrom(c, "output-path", "ANNIE_OUTPUT_PATH", ""),
		retryTimes:  uintFrom(c, "retry", "ANNIE_RETRY"),

		host:  stringFrom(c, "host", "ANNIE_HOST", ""),
		port:  stringFrom(c, "port", "ANNIE_PORT", "8080"),
		token: stringFrom(c, "token", "ANNIE_TOKEN", ""),

		tasks:   list.New(),
		history: list.New(),
	}
	return server
}

func (s *Server) Run() {
	if s.debug {
		gin.SetMode(gin.DebugMode)
	} else {
		gin.SetMode(gin.ReleaseMode)
	}
	router := gin.Default()
	router.POST("/download", s.postDownload)
	router.GET("/tasks", s.getTasks)
	router.GET("/history", s.getHistory)
	router.Run(s.host + ":" + s.port)
}

func (s *Server) postDownload(c *gin.Context) {
	if c.Query("token") != s.token {
		c.Status(http.StatusForbidden)
		return
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
		return
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
		return
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
		t.Errors = append(t.Errors, err.Error())
		t.Status = "Failed"
		return
	}
	t.Status = "Downloading"
	d := downloader.New(downloader.Options{
		Caption:     t.Caption,
		ChunkSizeMB: int(s.chunkSizeMB),
		MultiThread: s.multiThread,
		OutputPath:  s.outputPath,
		Refer:       t.Refer,
		RetryTimes:  int(s.retryTimes),
		Stream:      t.StreamFormat,
	})
	failureCount := 0
	successCount := 0
	for _, item := range data {
		if item.Err != nil {
			t.Errors = append(t.Errors, item.Err.Error())
			failureCount += 1
			continue
		}
		if err := d.Download(item); err != nil {
			t.Errors = append(t.Errors, err.Error())
			failureCount += 1
		} else {
			successCount += 1
		}
	}
	if failureCount == 0 {
		t.Status = "Done"
	} else if successCount == 0 {
		t.Status = "Failed"
	} else {
		t.Status = "PartlyDone"
	}
}

func (s *Server) finish(e *list.Element) {
	s.tasks.Remove(e)
	if s.history.Len() == 10 {
		s.history.Remove(s.history.Front())
	}
	s.history.PushBack(*e.Value.(*Task))
}

func boolFrom(c *cli.Context, flag string, env string) bool {
	value := c.Bool(flag)
	if !value {
		_, envSet := os.LookupEnv(env)
		value = envSet
	}
	return value
}

func stringFrom(c *cli.Context, flag string, env string, def string) string {
	value := c.String(flag)
	if value == "" {
		value = os.Getenv(env)
	}
	if value == "" {
		value = def
	}
	return value
}

func uintFrom(c *cli.Context, flag string, env string) uint {
	value := c.Uint(flag)
	if value == 0 {
		if envValue, envSet := os.LookupEnv(env); envSet {
			if intValue, err := strconv.Atoi(envValue); err != nil {
				value = uint(intValue)
			}
		}
	}
	return value
}
