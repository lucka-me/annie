package server

import (
	"container/list"
	"net/http"
	"os"
	"strconv"
	"sync"

	"github.com/gin-gonic/gin"
	"github.com/iawia002/annie/downloader"
	"github.com/iawia002/annie/extractors"
	"github.com/iawia002/annie/extractors/types"
	"github.com/iawia002/annie/request"
	"github.com/urfave/cli/v2"
)

type Server struct {
	chunkSizeMB uint
	debug       bool
	multiThread bool
	outputPath  string
	retryTimes  uint

	host  string
	port  string
	token string

	tasksMutext   sync.RWMutex
	tasks         list.List // *AyncTask
	historyMutext sync.RWMutex
	history       list.List // Task
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
	s.tasksMutext.RLock()
	for e := s.tasks.Front(); e != nil; e = e.Next() {
		asyncTask := e.Value.(*AsyncTask)
		asyncTask.mutex.RLock()
		tasks = append(tasks, asyncTask.task)
		asyncTask.mutex.RUnlock()
	}
	s.tasksMutext.RUnlock()
	c.IndentedJSON(http.StatusOK, tasks)
}

func (s *Server) getHistory(c *gin.Context) {
	if c.Query("token") != s.token {
		c.Status(http.StatusForbidden)
		return
	}

	tasks := []Task{}
	s.historyMutext.RLock()
	for e := s.history.Front(); e != nil; e = e.Next() {
		tasks = append(tasks, e.Value.(Task))
	}
	s.historyMutext.RUnlock()
	c.IndentedJSON(http.StatusOK, tasks)
}

func (s *Server) download(t Task) {
	s.tasksMutext.Lock()
	asyncTask := AsyncTask{
		task: t,
	}
	element := s.tasks.PushBack(&asyncTask)
	s.tasksMutext.Unlock()
	defer s.finish(element)
	asyncTask.setStatus("Extracting")
	data, err := extractors.Extract(t.Url, types.Options{
		Cookie: t.Cookie,
	})
	if err != nil {
		asyncTask.appendError(err.Error())
		asyncTask.setStatus("Failed")
		return
	}
	asyncTask.setStatus("Downloading")
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
			asyncTask.appendError(item.Err.Error())
			failureCount += 1
			continue
		}
		if err := d.Download(item); err != nil {
			asyncTask.appendError(err.Error())
			failureCount += 1
		} else {
			successCount += 1
		}
	}
	if failureCount == 0 {
		asyncTask.setStatus("Done")
	} else if successCount == 0 {
		asyncTask.setStatus("Failed")
	} else {
		asyncTask.setStatus("PartlyDone")
	}
}

func (s *Server) finish(e *list.Element) {
	s.tasksMutext.Lock()
	s.tasks.Remove(e)
	s.tasksMutext.Unlock()
	s.historyMutext.Lock()
	if s.history.Len() == 10 {
		s.history.Remove(s.history.Front())
	}
	s.history.PushBack(e.Value.(*AsyncTask).task)
	s.historyMutext.Unlock()
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
