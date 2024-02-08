package stashserver

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"

	logf "sigs.k8s.io/controller-runtime/pkg/log"
)

// The Stasher interface provides stashes for StashServer to manage
type Stasher interface {
	Stash(io.Writer) error
}

var log = logf.Log.WithName("stash-server")

var tsFormat = "20060102T150405"

// StashStatus represents the status of a buffer Stash
type StashStatus string

const (
	// StashStarted status is when the stash has been started but not complete
	StashStarted StashStatus = "Started"
	// StashComplete status is when the stash has succesfully completed
	StashComplete StashStatus = "Complete"
	// StashFailed status is when the stash has failed
	StashFailed StashStatus = "Failed"
)

// StashCompletionFunc takes a certain action based on the event that triggered it
type StashCompletionFunc func(d *Stash)

// Stash is a dump of the event buffer
type Stash struct {
	Status StashStatus
	Name   string
}

// StashServer serves an API to trigger and fetch stashes
type StashServer struct {
	mux          *http.ServeMux
	stasher      Stasher
	stashes      map[string]*Stash
	stashesMutex sync.RWMutex

	// These are callbacks used to trigger notifications when stashes are complete
	stashCompleteCallbacks []StashCompletionFunc

	stashDir    string
	stashPrefix string

	maxStashes int
}

const stashDir = "/tmp/"
const stashFileExtension = ".json"
const stashPrefix = "event-log-"

// NewStashServer creates a new StashServer
func NewStashServer(stasher Stasher, maxStashes int) *StashServer {
	dm := StashServer{
		mux:         http.NewServeMux(),
		stasher:     stasher,
		stashes:     make(map[string]*Stash),
		stashDir:    stashDir,
		stashPrefix: stashPrefix,
		maxStashes:  maxStashes,
	}

	dm.loadExistingFileStashes()

	dm.mux.HandleFunc("/stashes", dm.handleStashes)
	dm.mux.HandleFunc("/stashes/", dm.handleGetStash)
	dm.mux.HandleFunc("/buffer", dm.handleGetBuffer)
	return &dm
}

// AddCompletionCallback adds a completion callback
func (dm *StashServer) AddCompletionCallback(callback StashCompletionFunc) {
	dm.stashCompleteCallbacks = append(dm.stashCompleteCallbacks, callback)
}

func (dm *StashServer) loadExistingFileStashes() {
	dm.stashesMutex.Lock()
	defer dm.stashesMutex.Unlock()
	dirs, err := os.ReadDir(dm.stashDir)

	if err != nil {
		log.Error(err, "Couldn't read stash directory, no existing stashes loaded")
	}

	for _, d := range dirs {
		if !d.IsDir() && strings.HasPrefix(d.Name(), dm.stashPrefix) {
			stashName, _ := strings.CutSuffix(d.Name(), stashFileExtension)
			dm.stashes[stashName] = &Stash{Status: StashComplete, Name: stashName}
		}
	}
}

func (dm *StashServer) handleStashes(rw http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		dm.handleGetStashes(rw, r)
		return
	case http.MethodPost:
		dm.handlePostStashes(rw, r)
		return
	}
}

func (dm *StashServer) handleGetStashes(rw http.ResponseWriter, r *http.Request) {
	dm.stashesMutex.RLock()
	defer dm.stashesMutex.RUnlock()
	json.NewEncoder(rw).Encode(dm.stashes)
}

func (dm *StashServer) handlePostStashes(rw http.ResponseWriter, r *http.Request) {
	stashName := dm.stashPrefix + time.Now().Format(tsFormat)

	dm.purgeOldStashes(dm.maxStashes - 1)

	if err := dm.createFileStash(stashName); err != nil {
		rw.WriteHeader(http.StatusInternalServerError)
		return
	}
	rw.WriteHeader(http.StatusCreated)

	rw.Write([]byte(stashName))
}

func (dm *StashServer) purgeOldStashes(maxStashes int) {
	dm.stashesMutex.Lock()
	defer dm.stashesMutex.Unlock()
	if len(dm.stashes) <= maxStashes {
		return
	}

	stashNames := make([]string, len(dm.stashes))

	i := 0
	for stash := range dm.stashes {
		stashNames[i] = stash
		i++
	}

	slices.Sort(stashNames)

	stashesToDelete := len(dm.stashes) - maxStashes

	for i = 0; i < stashesToDelete; i++ {
		stashName := stashNames[i]
		log.Info("Removing old stash", "stashName", stashName)
		stashLocation := dm.getStashLocation(stashName)
		os.Remove(stashLocation)
		delete(dm.stashes, stashName)
	}
}

func (dm *StashServer) handleGetStash(rw http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		rw.WriteHeader(http.StatusBadRequest)
		return
	}

	stashName := strings.TrimPrefix(r.URL.Path, "/stashes/")

	dm.stashesMutex.RLock()
	defer dm.stashesMutex.RUnlock()
	if stash, exists := dm.stashes[stashName]; !exists || stash.Status == StashFailed {
		rw.WriteHeader(http.StatusNotFound)
		return
	} else if stash.Status == StashStarted {
		rw.WriteHeader(http.StatusMethodNotAllowed)
		return
	} else if stash.Status != StashComplete {
		rw.WriteHeader(http.StatusInternalServerError)
		return
	}

	filePath := dm.getStashLocation(stashName)
	rw.Header().Set("Content-Type", "application/json")
	http.ServeFile(rw, r, filePath)
}

func (dm *StashServer) createFileStash(stashName string) error {
	dm.stashesMutex.Lock()
	defer dm.stashesMutex.Unlock()
	log.Info("Creating event stash", "stash-name", stashName)

	d := &Stash{
		Status: StashStarted,
		Name:   stashName,
	}

	if _, exists := dm.stashes[stashName]; exists {
		err := fmt.Errorf("Stash %s already exists", stashName)
		log.Error(err, "Stash creation failed")
		return err
	}

	dm.stashes[stashName] = d

	stashStatus := StashFailed

	// After we're finished whether succesful or not, update the stash status
	defer func() {
		dm.stashes[stashName].Status = stashStatus
	}()

	filePath := filepath.Join(dm.stashDir, stashName) + stashFileExtension
	f, err := os.Create(filePath)

	if err != nil {
		log.Error(err, "Failed to create stash file")
		return err
	}

	defer f.Close()

	err = dm.stasher.Stash(f)

	if err != nil {
		log.Error(err, "Error writing stash to file")
		return err
	}

	stashStatus = StashComplete

	log.Info("Executing Complete Functions", "stash-name", stashName)
	go dm.execStashCompleteFuncs(d)
	return nil
}

func (dm *StashServer) handleGetBuffer(rw http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		rw.WriteHeader(http.StatusBadRequest)
		return
	}

	rw.Header().Set("Content-Type", "application/json")
	err := dm.stasher.Stash(rw)

	if err != nil {
		rw.WriteHeader(http.StatusInternalServerError)
	}
}

// CreateBufferStash creates a stash of the buffer
func (dm *StashServer) CreateBufferStash() {
	stashName := dm.stashPrefix + time.Now().Format(tsFormat)

	dm.createFileStash(stashName)
}

func (dm *StashServer) execStashCompleteFuncs(d *Stash) {
	for _, callback := range dm.stashCompleteCallbacks {
		callback(d)
	}
}

func (dm *StashServer) getStashLocation(stashName string) string {
	return filepath.Join(dm.stashDir, stashName) + stashFileExtension
}

// Run starts the server
func (dm *StashServer) Run(port string) {
	log.Info("Starting Stash Server", "port", port)
	http.ListenAndServe(":"+port, dm.mux)
}
